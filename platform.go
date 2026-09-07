package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func getCurrentSSID() (string, error) {
	if runtime.GOOS == "windows" {
		out, _ := exec.Command("cmd", "/c", "netsh wlan show interfaces | findstr SSID").Output()
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "SSID") && !strings.Contains(line, "BSSID") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1]), nil
				}
			}
		}
		return "", fmt.Errorf("disconnected or not found")
	} else if runtime.GOOS == "darwin" {
		out, err := exec.Command("networksetup", "-listallhardwareports").Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(out), "\n")
		var iface string
		for i, line := range lines {
			if strings.Contains(line, "Wi-Fi") && i+1 < len(lines) {
				parts := strings.Fields(lines[i+1])
				if len(parts) >= 2 && parts[0] == "Device:" {
					iface = parts[1]
					break
				}
			}
		}
		if iface == "" {
			return "", fmt.Errorf("no wifi interface found")
		}
		out, err = exec.Command("networksetup", "-getairportnetwork", iface).Output()
		if err != nil {
			return "", err
		}
		outStr := string(out)
		if strings.Contains(outStr, "Current Wi-Fi Network: ") {
			parts := strings.SplitN(outStr, ": ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
		if strings.Contains(outStr, "not associated") {
			statusOut, _ := exec.Command("ifconfig", iface).Output()
			if strings.Contains(string(statusOut), "status: active") {
				return "", fmt.Errorf("Location Privacy")
			}
			return "", fmt.Errorf("disconnected")
		}
		return "", fmt.Errorf("unknown macOS error")
	} else {
		out, err := exec.Command("iwgetid", "-r").Output()
		if err != nil {
			return "", err
		}
		ssid := strings.TrimSpace(string(out))
		if ssid == "" {
			return "", fmt.Errorf("disconnected")
		}
		return ssid, nil
	}
}

func notifyCredentialsNeeded() bool {
	notifiedFile := credentialsFilePath() + ".notified"
	if _, err := os.Stat(notifiedFile); err == nil {
		return false
	}

	if runtime.GOOS == "darwin" {
		script := `display notification "Invalid or missing credentials. Please run autologin in your terminal to update them." with title "Fortinet Auto Login"`
		exec.Command("osascript", "-e", script).Run()
		os.WriteFile(notifiedFile, []byte("1"), 0644)
		return true
	} else if runtime.GOOS == "linux" {
		exec.Command("notify-send", "Fortinet Auto Login", "Invalid or missing credentials. Please run autologin in your terminal to update them.", "-u", "critical").Run()
		os.WriteFile(notifiedFile, []byte("1"), 0644)
		return true
	}
	return false
}

func installAgent() {
	_, ok := loadCredentials()
	if !ok {
		promptCredentials()
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   Executable path error:", err)
		return
	}

	if runtime.GOOS == "darwin" {
		installDarwin(exe)
	} else if runtime.GOOS == "linux" {
		installLinux(exe)
	} else {
		fmt.Println("\033[31m[FAIL]\033[0m   Auto-install is only supported on macOS and Linux")
	}
}

func uninstallAgent() {
	if runtime.GOOS == "darwin" {
		uninstallDarwin()
	} else if runtime.GOOS == "linux" {
		uninstallLinux()
	} else {
		fmt.Println("\033[31m[FAIL]\033[0m   Auto-uninstall is only supported on macOS and Linux")
	}
}

func installDarwin(exe string) {
	plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.fortinet.autologin.plist")
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.fortinet.autologin</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>-auto</string>
    </array>
    <key>StartInterval</key>
    <integer>5</integer>
    <key>WatchPaths</key>
    <array>
        <string>/Library/Preferences/SystemConfiguration/com.apple.wifi.message-tracer.plist</string>
        <string>/var/run/resolv.conf</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/autologin.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/autologin.err.log</string>
</dict>
</plist>`, exe)

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   LaunchAgent write error:", err)
		return
	}

	exec.Command("launchctl", "unload", plistPath).Run()
	err = exec.Command("launchctl", "load", plistPath).Run()
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   LaunchAgent load error:", err)
		return
	}

	fmt.Println("\033[32m[OK]\033[0m     Installed successfully")
	fmt.Println("\033[36m[INFO]\033[0m   Background service active (event-driven + 5s fallback)")
	fmt.Println("\033[36m[INFO]\033[0m   Logs: /tmp/autologin.log")
}

func uninstallDarwin() {
	plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.fortinet.autologin.plist")

	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("\033[33m[WARN]\033[0m   Agent is not installed")
		return
	}

	exec.Command("launchctl", "unload", plistPath).Run()
	err := os.Remove(plistPath)
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   Failed to remove LaunchAgent:", err)
		return
	}

	fmt.Println("\033[32m[OK]\033[0m     Uninstalled successfully")
}

func installLinux(exe string) {
	home, _ := os.UserHomeDir()
	systemdDir := filepath.Join(home, ".config", "systemd", "user")
	err := os.MkdirAll(systemdDir, 0755)
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   Failed to create systemd user directory:", err)
		return
	}

	servicePath := filepath.Join(systemdDir, "fortinet-autologin.service")
	serviceContent := fmt.Sprintf(`[Unit]
Description=Fortinet Auto Login Daemon
After=network-online.target

[Service]
ExecStart=%s -daemon
Restart=always
RestartSec=5
StandardOutput=append:/tmp/autologin.log
StandardError=append:/tmp/autologin.err.log

[Install]
WantedBy=default.target
`, exe)

	err = os.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   Systemd service write error:", err)
		return
	}

	exec.Command("systemctl", "--user", "daemon-reload").Run()
	err = exec.Command("systemctl", "--user", "enable", "--now", "fortinet-autologin.service").Run()
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   Systemd service enable error:", err)
		return
	}

	fmt.Println("\033[32m[OK]\033[0m     Installed successfully")
	fmt.Println("\033[36m[INFO]\033[0m   Background service active (systemd)")
	fmt.Println("\033[36m[INFO]\033[0m   Logs: /tmp/autologin.log")
}

func uninstallLinux() {
	home, _ := os.UserHomeDir()
	servicePath := filepath.Join(home, ".config", "systemd", "user", "fortinet-autologin.service")

	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Println("\033[33m[WARN]\033[0m   Service is not installed")
		return
	}

	exec.Command("systemctl", "--user", "disable", "--now", "fortinet-autologin.service").Run()
	err := os.Remove(servicePath)
	if err != nil {
		fmt.Println("\033[31m[FAIL]\033[0m   Failed to remove systemd service:", err)
		return
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()

	fmt.Println("\033[32m[OK]\033[0m     Uninstalled successfully")
}
