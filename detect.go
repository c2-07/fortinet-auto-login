package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FortinetDetectionResult contains findings about Fortinet presence on the network.
type FortinetDetectionResult struct {
	IsFortinetNetwork bool   `json:"is_fortinet_network"`
	CaptivePortalHost string `json:"captive_portal_host,omitempty"`
	MagicToken        string `json:"magic_token,omitempty"`
	GatewayIP         string `json:"gateway_ip,omitempty"`
	DeviceSerialNumber string `json:"device_serial_number,omitempty"`
	DetectionMethod   string `json:"detection_method,omitempty"`
}

// DetectFortinetNetwork inspects the current network environment using multiple probes:
// 1. Captive portal redirect probe (via standard HTTP detection)
// 2. Gateway inspection probe on known FortiGate admin/portal ports (1000, 1003)
// 3. TLS Certificate inspection looking for Fortinet CA / FortiGate signatures
func DetectFortinetNetwork() FortinetDetectionResult {
	result := FortinetDetectionResult{}

	// Probe 1: Check if an unauthenticated captive portal redirect is active
	client := &http.Client{
		Timeout: 4 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	portalURL, alreadyConnected, _ := getPortal(client)
	if portalURL != "" {
		result.IsFortinetNetwork = true
		result.DetectionMethod = "Captive Portal Redirect"
		if parsed, err := url.Parse(portalURL); err == nil {
			result.CaptivePortalHost = parsed.Host
			result.MagicToken = parsed.RawQuery
		}
		return result
	}

	// Probe 2: Gateway TLS inspection (even when internet is already authenticated)
	// Query local gateway IP and fallbackHost (192.168.55.253:1003)
	gatewayCandidates := []string{
		fallbackHost,
	}

	// Also dynamically find default gateway or local interface subnets if reachable
	if defaultGW := getDefaultGatewayHint(); defaultGW != "" {
		gatewayCandidates = append([]string{defaultGW + ":1003", defaultGW + ":1000"}, gatewayCandidates...)
	}

	for _, target := range gatewayCandidates {
		conn, err := net.DialTimeout("tcp", target, 1500*time.Millisecond)
		if err != nil {
			continue
		}

		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true,
		})
		_ = tlsConn.SetDeadline(time.Now().Add(2 * time.Second))
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
					return result
				}
			}
		}
		tlsConn.Close()
	}

	// If already authenticated via captive portal, inspect last known session cache
	if alreadyConnected {
		if s, ok := loadSession(); ok {
			result.IsFortinetNetwork = true
			result.CaptivePortalHost = s.Host
			result.DetectionMethod = "Active Authenticated Session Cache"
			return result
		}
	}

	return result
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
				// Common gateway is .1 or .254
				return fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
			}
		}
	}
	return ""
}
