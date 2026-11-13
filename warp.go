package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Comprehensive Cloudflare WARP Anycast IP pools
var warpAnycastIPs = []string{
	"162.159.192.1",
	"162.159.192.2",
	"162.159.192.3",
	"162.159.193.1",
	"162.159.193.2",
	"162.159.195.1",
	"188.114.96.1",
	"188.114.96.2",
	"188.114.97.1",
	"188.114.97.2",
	"188.114.98.1",
	"188.114.99.1",
}

// Ports accepted by Cloudflare WARP that evade firewall blocks.
// 500 & 4500 are IPsec VPN standard ports (whitelisted for enterprise VPNs).
// 1701 is L2TP, and 854/880/890 are alternate WireGuard ports.
var warpPorts = []string{
	"500",
	"4500",
	"1701",
	"854",
	"880",
	"890",
}

func generateCandidateEndpoints() []string {
	var candidates []string
	highPriority := []string{
		"162.159.192.1:500",
		"162.159.192.1:4500",
		"162.159.193.1:500",
		"162.159.193.1:4500",
		"188.114.96.1:500",
		"188.114.97.1:4500",
	}
	candidates = append(candidates, highPriority...)

	var pool []string
	for _, ip := range warpAnycastIPs {
		for _, port := range warpPorts {
			ep := ip + ":" + port
			isDup := false
			for _, hp := range highPriority {
				if hp == ep {
					isDup = true
					break
				}
			}
			if !isDup {
				pool = append(pool, ep)
			}
		}
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})

	candidates = append(candidates, pool...)
	return candidates
}

func preflightProbe(endpoint string) bool {
	conn, err := net.DialTimeout("udp", endpoint, 400*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// testInternetConnectivity verifies live outbound HTTP reachability through the tunnel.
func testInternetConnectivity(timeout time.Duration) bool {
	client := &http.Client{
		Timeout: timeout,
	}

	// Probe 1: Direct IP query to Cloudflare trace
	resp, err := client.Get("https://1.1.1.1/cdn-cgi/trace")
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}

	// Probe 2: Standard HTTP connectivity check
	resp2, err2 := client.Get("http://detectportal.firefox.com/success.txt")
	if err2 == nil {
		_ = resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK {
			return true
		}
	}

	// Probe 3: Cloudflare generate 204
	resp3, err3 := client.Get("https://cp.cloudflare.com/generate_204")
	if err3 == nil {
		_ = resp3.Body.Close()
		if resp3.StatusCode == http.StatusNoContent || resp3.StatusCode == http.StatusOK {
			return true
		}
	}

	return false
}

// CycleWiFi turns Wi-Fi off and back on across macOS, Linux, and Windows to renew DHCP and reset firewall state.
func CycleWiFi() bool {
	switch runtime.GOOS {
	case "darwin":
		return cycleWiFiDarwin()
	case "linux":
		return cycleWiFiLinux()
	case "windows":
		return cycleWiFiWindows()
	default:
		return false
	}
}

func cycleWiFiDarwin() bool {
	wifiDevice := "en0"
	out, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for i, line := range lines {
			if strings.Contains(line, "Wi-Fi") || strings.Contains(line, "AirPort") {
				if i+1 < len(lines) && strings.Contains(lines[i+1], "Device:") {
					parts := strings.Fields(lines[i+1])
					if len(parts) >= 2 {
						wifiDevice = parts[1]
					}
					break
				}
			}
		}
	}

	if err := exec.Command("networksetup", "-setairportpower", wifiDevice, "off").Run(); err != nil {
		return false
	}
	time.Sleep(2 * time.Second)
	return exec.Command("networksetup", "-setairportpower", wifiDevice, "on").Run() == nil
}

func cycleWiFiLinux() bool {
	if _, err := exec.LookPath("nmcli"); err == nil {
		if exec.Command("nmcli", "radio", "wifi", "off").Run() == nil {
			time.Sleep(2 * time.Second)
			return exec.Command("nmcli", "radio", "wifi", "on").Run() == nil
		}
	}
	if _, err := exec.LookPath("rfkill"); err == nil {
		if exec.Command("rfkill", "block", "wifi").Run() == nil {
			time.Sleep(2 * time.Second)
			return exec.Command("rfkill", "unblock", "wifi").Run() == nil
		}
	}
	return false
}

func cycleWiFiWindows() bool {
	psScript := `Get-NetAdapter -Name *Wi-Fi* | Disable-NetAdapter -Confirm:$false; Start-Sleep -Seconds 2; Get-NetAdapter -Name *Wi-Fi* | Enable-NetAdapter -Confirm:$false`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	if cmd.Run() == nil {
		return true
	}
	_ = exec.Command("netsh", "interface", "set", "interface", "Wi-Fi", "disable").Run()
	time.Sleep(2 * time.Second)
	return exec.Command("netsh", "interface", "set", "interface", "Wi-Fi", "enable").Run() == nil
}

func recoverViaWiFiCycle() bool {
	logf("\033[33m[WARN]\033[0m   WARP tunnel connected but internet traffic is blackholed by FortiGate.")
	logf("\033[36m[ACTION]\033[0m Automatically restarting Wi-Fi to renew DHCP lease and reset firewall session table...")

	_ = exec.Command("warp-cli", "disconnect").Run()
	time.Sleep(500 * time.Millisecond)

	if !CycleWiFi() {
		logf("\033[33m[INFO]\033[0m   Could not toggle Wi-Fi automatically (permission or OS restriction).")
		logf("         Please toggle Wi-Fi OFF and ON in system settings to refresh your DHCP IP,")
		logf("         then re-run './autologin' and './autologin -warp'.")
		return false
	}

	logf("\033[32m[OK]\033[0m     Wi-Fi restarted. Waiting 6s for network association & DHCP lease...")
	time.Sleep(6 * time.Second)

	// Re-authenticate with Fortinet captive portal if on campus
	det := DetectFortinetNetwork()
	if det.IsFortinetNetwork && !det.InternetConnected {
		logf("\033[34m[AUTH]\033[0m   Re-authenticating with Fortinet captive portal...")
		initCredentials("", "")
		if !login(false, false) {
			logf("\033[33m[WARN]\033[0m   Portal login pending. Retrying in 2s...")
			time.Sleep(2 * time.Second)
			_ = login(false, false)
		}
	}

	recoveryEndpoints := []string{"162.159.192.1:500", "162.159.192.1:4500", "162.159.193.1:500"}
	logf("\033[36m[WARP]\033[0m   Retrying Cloudflare WARP with fresh IP and clean session table...")
	for _, ep := range recoveryEndpoints {
		_ = exec.Command("warp-cli", "tunnel", "endpoint", "set", ep).Run()
		_ = exec.Command("warp-cli", "connect").Run()

		for attempt := 0; attempt < 5; attempt++ {
			time.Sleep(800 * time.Millisecond)
			chk, _ := exec.Command("warp-cli", "status").CombinedOutput()
			if strings.Contains(string(chk), "Connected") {
				if testInternetConnectivity(3 * time.Second) {
					logf("\033[32m[OK]\033[0m     Success! Reconnected to Cloudflare WARP via %s (Internet verified active)", ep)
					return true
				}
			}
		}
		_ = exec.Command("warp-cli", "disconnect").Run()
	}

	logf("\033[31m[FAIL]\033[0m   Unable to restore WARP connection. Tunnel safely disconnected.")
	return false
}

// FixAndConnectWarp systematically finds a responsive Cloudflare endpoint across Anycast pools:
// 1. Detects Fortinet captive portal / gateway presence
// 2. Verifies warp-cli presence and configures WireGuard mode
// 3. Automatically probes and rotates across Anycast IPs and VPN ports
// 4. Verifies real outbound internet connectivity (not just tunnel status)
// 5. Automatically toggles Wi-Fi to refresh DHCP and clear firewall blackholes if traffic stalls
func FixAndConnectWarp() bool {
	// Step 1: Detect if user is connected to Fortinet network
	logf("\033[36m[DETECT]\033[0m Checking network environment and Fortinet presence...")
	det := DetectFortinetNetwork()
	if det.IsFortinetNetwork {
		info := ""
		if det.DeviceSerialNumber != "" {
			info = fmt.Sprintf(" (Device ID: %s, Gateway: %s)", det.DeviceSerialNumber, det.GatewayIP)
		} else if det.GatewayIP != "" {
			info = fmt.Sprintf(" (Gateway: %s)", det.GatewayIP)
		}
		logf("\033[32m[OK]\033[0m     Fortinet network detected%s", info)

		// Check if captive portal requires authentication
		if !det.InternetConnected {
			logf("\033[33m[AUTH]\033[0m   Captive portal is not authenticated yet. Logging in...")
			initCredentials("", "")
			if !login(false, false) {
				logf("\033[31m[FAIL]\033[0m   Could not authenticate with Fortinet captive portal. Run './autologin' first.")
				return false
			}
			logf("\033[32m[OK]\033[0m     Captive portal authenticated successfully.")
		}
	} else {
		logf("\033[33m[WARN]\033[0m   No Fortinet gateway signature detected on this network.")
		logf("         (WARP auto-rotation is specifically optimized for evading Fortinet firewall restrictions).")
	}

	// Step 2: Verify warp-cli presence
	_, err := exec.LookPath("warp-cli")
	if err != nil {
		logf("\033[31m[FAIL]\033[0m   warp-cli not found in system PATH. Please install Cloudflare WARP.")
		return false
	}

	_ = exec.Command("warp-cli", "debug", "connectivity-check", "disable").Run()
	_ = exec.Command("warp-cli", "tunnel", "protocol", "set", "WireGuard").Run()

	// Step 3: Check if already connected and verify live internet
	statusOut, _ := exec.Command("warp-cli", "status").CombinedOutput()
	if strings.Contains(string(statusOut), "Connected") {
		logf("  Checking existing WARP connection health...")
		if testInternetConnectivity(3 * time.Second) {
			logf("\033[32m[OK]\033[0m     WARP is already connected and internet is fully reachable!")
			return true
		}
		logf("\033[33m[WARN]\033[0m   WARP is connected but internet traffic is stalled. Disconnecting to rotate...")
		_ = exec.Command("warp-cli", "disconnect").Run()
		time.Sleep(200 * time.Millisecond)
	}

	logf("\033[36m[WARP]\033[0m   Initializing intelligent Cloudflare WARP auto-rotation...")
	endpoints := generateCandidateEndpoints()
	logf("  Generated %d candidate endpoint combinations across %d Anycast IP pools", len(endpoints), len(warpAnycastIPs))

	// Step 4: Rotate candidates
	for i, ep := range endpoints {
		if i >= 8 {
			break
		}

		if !preflightProbe(ep) {
			continue
		}

		logf("\033[34m[WARP]\033[0m   Testing endpoint [%d/8]: %s...", i+1, ep)
		_ = exec.Command("warp-cli", "disconnect").Run()
		time.Sleep(100 * time.Millisecond)

		_ = exec.Command("warp-cli", "tunnel", "endpoint", "set", ep).Run()
		_ = exec.Command("warp-cli", "connect").Run()

		connected := false
		for attempt := 0; attempt < 6; attempt++ {
			time.Sleep(500 * time.Millisecond)
			chk, _ := exec.Command("warp-cli", "status").CombinedOutput()
			chkStr := string(chk)

			if strings.Contains(chkStr, "Connected") {
				statsOut, _ := exec.Command("warp-cli", "tunnel", "stats").CombinedOutput()
				statsStr := string(statsOut)
				if !strings.Contains(statsStr, "Received: 0B") || attempt >= 3 {
					connected = true
					break
				}
			}
		}

		if connected {
			logf("  Verifying internet reachability through %s...", ep)
			if testInternetConnectivity(3 * time.Second) {
				logf("\033[32m[OK]\033[0m     Success! Connected to Cloudflare WARP via %s (Internet verified active)", ep)
				return true
			}
			logf("\033[33m[WARN]\033[0m   Endpoint %s connected to tunnel but internet is unreachable (UDP blackholed).", ep)
		} else {
			logf("\033[33m[WARN]\033[0m   Endpoint %s timed out. Trying next candidate...", ep)
		}
	}

	// Step 5: Tunnel key rotation attempt
	logf("\033[34m[WARP]\033[0m   Rotating tunnel cryptographic keys...")
	_ = exec.Command("warp-cli", "tunnel", "rotate-keys").Run()
	_ = exec.Command("warp-cli", "tunnel", "endpoint", "set", "162.159.192.1:500").Run()
	_ = exec.Command("warp-cli", "connect").Run()

	time.Sleep(2 * time.Second)
	finalChk, _ := exec.Command("warp-cli", "status").CombinedOutput()
	if strings.Contains(string(finalChk), "Connected") {
		if testInternetConnectivity(3 * time.Second) {
			logf("\033[32m[OK]\033[0m     Connected after tunnel key rotation (Internet verified active)!")
			return true
		}
	}

	// Step 6: If all endpoints fail or internet is unreachable, trigger Wi-Fi toggle recovery
	return recoverViaWiFiCycle()
}

// DisconnectWarp disconnects WARP cleanly.
func DisconnectWarp() bool {
	logf("\033[34m[WARP]\033[0m   Disconnecting Cloudflare WARP...")
	out, err := exec.Command("warp-cli", "disconnect").CombinedOutput()
	if err != nil {
		logf("\033[31m[FAIL]\033[0m   Failed to disconnect: %s", string(out))
		return false
	}
	logf("\033[32m[OK]\033[0m     Cloudflare WARP disconnected. Normal routing restored.")
	return true
}
