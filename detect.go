package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// FortinetDetectionResult contains findings about Fortinet presence on the network.
type FortinetDetectionResult struct {
	IsFortinetNetwork  bool   `json:"is_fortinet_network"`
	CaptivePortalHost  string `json:"captive_portal_host,omitempty"`
	MagicToken         string `json:"magic_token,omitempty"`
	GatewayIP          string `json:"gateway_ip,omitempty"`
	DeviceSerialNumber string `json:"device_serial_number,omitempty"`
	DetectionMethod    string `json:"detection_method,omitempty"`
	InternetConnected  bool   `json:"internet_connected"`
}

var (
	cachedDetection     FortinetDetectionResult
	cachedDetectionTime time.Time
	cacheMutex          sync.Mutex
	detectionCacheTTL   = 20 * time.Second
)

// InvalidateDetectionCache clears the cached network detection result.
func InvalidateDetectionCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	cachedDetectionTime = time.Time{}
}

// DetectFortinetNetwork inspects the current network environment using multiple probes.
func DetectFortinetNetwork() FortinetDetectionResult {
	cacheMutex.Lock()
	if time.Since(cachedDetectionTime) < detectionCacheTTL && cachedDetection.IsFortinetNetwork {
		res := cachedDetection
		cacheMutex.Unlock()
		return res
	}
	cacheMutex.Unlock()

	result := FortinetDetectionResult{}

	// Probe 1: Check captive portal redirect and general connectivity
	client := &http.Client{
		Timeout: 2500 * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	portalURL, alreadyConnected, _ := getPortal(client)
	result.InternetConnected = alreadyConnected

	if portalURL != "" {
		result.IsFortinetNetwork = true
		result.DetectionMethod = "Captive Portal Redirect"
		if parsed, err := url.Parse(portalURL); err == nil {
			result.CaptivePortalHost = parsed.Host
			result.MagicToken = parsed.RawQuery
		}
		updateCache(result)
		return result
	}

	// Probe 2: Gateway TLS inspection
	gatewayCandidates := []string{
		getEffectivePortalHost(),
	}

	if defaultGW := getDefaultGatewayHint(); defaultGW != "" {
		gatewayCandidates = append([]string{defaultGW + ":1003"}, gatewayCandidates...)
	}

	for _, target := range gatewayCandidates {
		conn, err := net.DialTimeout("tcp", target, 600*time.Millisecond)
		if err != nil {
			continue
		}

		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true,
		})
		_ = tlsConn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
		err = tlsConn.Handshake()
		if err == nil {
			state := tlsConn.ConnectionState()
			for _, cert := range state.PeerCertificates {
				issuer := strings.ToLower(cert.Issuer.String())
				subject := strings.ToLower(cert.Subject.String())
				if strings.Contains(issuer, "fortinet") || strings.Contains(subject, "fortinet") ||
					strings.Contains(issuer, "fortigate") || strings.Contains(subject, "fortigate") {
					result.IsFortinetNetwork = true
					result.GatewayIP = target
					result.DetectionMethod = "Gateway TLS Certificate Signature"
					if cert.Subject.CommonName != "" {
						result.DeviceSerialNumber = cert.Subject.CommonName
					}
					tlsConn.Close()
					updateCache(result)
					return result
				}
			}
		}
		tlsConn.Close()
	}

	// Probe 3: Check active authenticated session cache
	if alreadyConnected {
		if s, ok := loadSession(); ok {
			result.IsFortinetNetwork = true
			result.CaptivePortalHost = s.Host
			result.DetectionMethod = "Active Authenticated Session Cache"
			updateCache(result)
			return result
		}
	}

	updateCache(result)
	return result
}

// PrintStatus prints a comprehensive summary of internet connectivity, Fortinet network presence, and active session.
func PrintStatus() {
	res := DetectFortinetNetwork()

	fmt.Println("\033[36m--- Network & Fortinet Status ---\033[0m")
	if res.InternetConnected {
		fmt.Printf("Internet:       \033[32mConnected\033[0m\n")
	} else if res.CaptivePortalHost != "" {
		fmt.Printf("Internet:       \033[33mCaptive Portal Intercepted\033[0m\n")
	} else {
		fmt.Printf("Internet:       \033[31mDisconnected / Offline\033[0m\n")
	}

	if res.IsFortinetNetwork {
		fmt.Printf("Fortinet Network: \033[32mYes\033[0m (Method: %s)\n", res.DetectionMethod)
		if res.GatewayIP != "" {
			fmt.Printf("Gateway IP:     %s\n", res.GatewayIP)
		}
		if res.DeviceSerialNumber != "" {
			fmt.Printf("Device ID:      %s\n", res.DeviceSerialNumber)
		}
		if res.CaptivePortalHost != "" {
			fmt.Printf("Portal Host:    %s\n", res.CaptivePortalHost)
		}
	} else {
		fmt.Printf("Fortinet Network: \033[33mNo Fortinet gateway signature detected\033[0m\n")
	}

	// Session details
	if s, ok := loadSession(); ok {
		fmt.Printf("\n\033[36m--- Active Session Cache ---\033[0m\n")
		fmt.Printf("Host:           %s://%s\n", s.Scheme, s.Host)
		fmt.Printf("Magic:          %s\n", s.Magic)
		if s.Countdown > 0 {
			fmt.Printf("Timeout:        %ds\n", s.Countdown)
		}
	}

	// Check credentials presence
	if c, ok := loadCredentials(); ok {
		fmt.Printf("\nSaved User:     %s\n", c.Username)
		if c.SSID != "" {
			fmt.Printf("Allowed SSIDs:  %s\n", c.SSID)
		}
	} else {
		fmt.Printf("\nSaved User:     \033[33mNone (Run autologin to set)\033[0m\n")
	}
}

func updateCache(r FortinetDetectionResult) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	cachedDetection = r
	cachedDetectionTime = time.Now()
}

// getDefaultGatewayHint checks network interfaces for local IP addresses to infer gateway subnets.
func getDefaultGatewayHint() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ip := ipnet.IP.To4()
				return fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
			}
		}
	}
	return ""
}
