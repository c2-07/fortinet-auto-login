package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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
	} else if runtime.GOOS == "windows" {
		script := `$wshell = New-Object -ComObject Wscript.Shell; $wshell.Popup("Invalid or missing credentials. Please run autologin in your terminal to update them.", 0, "Fortinet Auto Login", 0x30)`
		exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script).Run()
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
		fmt.Println("\033[31m[FAIL]\033[0m   Failed to locate executable:", err)
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
    <integer>30</integer>
    <key>WatchPaths</key>
    <array>
        <string>/Library/Preferences/SystemConfiguration/com.apple.airport.preferences.plist</string>
        <string>/Library/Preferences/SystemConfiguration/NetworkInterfaces.plist</string>
        <string>/Library/Preferences/SystemConfiguration/preferences.plist</string>
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
	fmt.Println("\033[36m[INFO]\033[0m   Hybrid service active (event-driven Wi-Fi triggers + 30s session watchdog)")
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

func clearLogs() {
	logPath := getLogFilePath()
	err := os.WriteFile(logPath, []byte(""), 0644)
	if err != nil {
		fmt.Printf("\033[31m[FAIL]\033[0m   Failed to clear logs: %v\n", err)
		return
	}
	fmt.Printf("\033[32m[OK]\033[0m     Logs cleared (%s)\n", logPath)
}

func viewLogs(follow bool, linesCount int) {
	logPath := getLogFilePath()

	if !follow {
		content, err := os.ReadFile(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("\033[33m[INFO]\033[0m   No logs found yet. The background task might not have started or hasn't printed anything.\n")
			} else {
				fmt.Printf("\033[31m[FAIL]\033[0m   Could not read logs: %v\n", err)
			}
			return
		}
		if len(content) == 0 {
			fmt.Printf("\033[33m[INFO]\033[0m   Log file is empty (%s).\n", logPath)
			return
		}

		lines := strings.Split(strings.TrimRight(string(content), "\r\n"), "\n")
		start := 0
		if linesCount > 0 && len(lines) > linesCount {
			start = len(lines) - linesCount
			fmt.Printf("\033[36m[LOGS]\033[0m   Showing last %d lines of %s:\n", linesCount, logPath)
		} else {
			fmt.Printf("\033[36m[LOGS]\033[0m   Showing all %d lines of %s:\n", len(lines), logPath)
		}
		for _, l := range lines[start:] {
			fmt.Println(l)
		}
		return
	}

	// Follow / streaming mode (tail -f)
	fmt.Printf("\033[36m[LOGS]\033[0m   Streaming %s (Press Ctrl+C to exit)...\n", logPath)

	file, err := os.Open(logPath)
	if err != nil {
		fmt.Printf("\033[31m[FAIL]\033[0m   Could not open log file: %v\n", err)
		return
	}
	defer file.Close()

	// Print last 20 lines first
	content, _ := os.ReadFile(logPath)
	if len(content) > 0 {
		lines := strings.Split(strings.TrimRight(string(content), "\r\n"), "\n")
		start := 0
		if len(lines) > 20 {
			start = len(lines) - 20
		}
		for _, l := range lines[start:] {
			fmt.Println(l)
		}
	}

	// Seek to end of file
	_, _ = file.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadString('\n')
		if err == nil {
			fmt.Print(line)
			continue
		}
		if err == io.EOF {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		break
	}
}

func cleanShellPathUnix() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	rcFiles := []string{
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".profile"),
	}
	for _, rc := range rcFiles {
		contentBytes, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		content := string(contentBytes)
		if strings.Contains(content, "fortinet-auto-login") {
			var filtered []string
			for _, line := range strings.Split(content, "\n") {
				if !strings.Contains(line, "fortinet-auto-login") &&
					!strings.Contains(line, "# Added by fortinet-auto-login installer") {
					filtered = append(filtered, line)
				}
			}
			newContent := strings.Join(filtered, "\n") + "\n"
			_ = os.WriteFile(rc, []byte(newContent), 0o644)
		}
	}
}

func fullUninstall() {
	fmt.Println("\033[33m[UNINSTALL]\033[0m Completely uninstalling autologin...")

	// 1. Remove background service
	uninstallAgent()

	// 2. Disconnect WARP
	_ = exec.Command("warp-cli", "disconnect").Run()

	// 3. Remove data directory
	appDir := getAppDir()
	if err := os.RemoveAll(appDir); err != nil {
		fmt.Printf("\033[33m[WARN]\033[0m   Could not remove data dir: %v\n", err)
	} else {
		fmt.Printf("\033[32m[OK]\033[0m     Removed configuration and credentials (%s)\n", appDir)
	}

	// 4. Remove log files
	logPath := getLogFilePath()
	_ = os.Remove(logPath)
	_ = os.Remove(logPath + ".old")
	_ = os.Remove("/tmp/autologin.err.log")
	fmt.Println("\033[32m[OK]\033[0m     Removed daemon log files")

	// 5. Clean shell PATH modifications
	if runtime.GOOS != "windows" {
		cleanShellPathUnix()
	}
	fmt.Println("\033[32m[OK]\033[0m     Cleaned shell PATH configuration")

	// 6. Remove executable itself
	if exe, err := os.Executable(); err == nil {
		fmt.Printf("\033[32m[OK]\033[0m     Removing executable binary at %s\n", exe)
		_ = os.Remove(exe)
	}

	fmt.Println("\033[32m[OK]\033[0m     autologin has been completely uninstalled from this system.")
}
