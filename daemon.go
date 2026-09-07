package main

import (
	"strings"
	"time"
)

// daemon runs forever, polling login() at a fixed interval. login() already
// short-circuits fast when already connected, so this is cheap while
// connected and will transparently re-authenticate whenever the session
// drops (AP switch, sleep/wake, portal timeout) without needing to detect
// network-change events at the OS level.
func daemon(interval time.Duration) {
	logf("\033[36m[ INIT ]\033[0m   Daemon started (interval=%s)", interval)
	for {
		login(true, false)
		time.Sleep(interval)
	}
}

func runAutoMode() {
	c, ok := loadCredentials()
	if !ok {
		if notifyCredentialsNeeded() {
			logf("\033[31m[ FAIL ]\033[0m   No creds. Run interactively.")
		}
		return
	}
	username = c.Username
	password = c.Password

	if c.SSID != "" {
		current, err := getCurrentSSID()
		if err != nil {
			if strings.Contains(err.Error(), "Location Privacy") {
				// macOS blocked SSID detection. Fall back to checking the portal directly.
				// We don't exit here so the script still functions perfectly.
			} else {
				// We are actually disconnected from Wi-Fi. Safe to exit.
				return
			}
		} else {
			// We successfully read the SSID. Strictly enforce it.
			matched := false
			for _, s := range strings.Split(c.SSID, ",") {
				if strings.Contains(strings.ToUpper(current), strings.ToUpper(strings.TrimSpace(s))) {
					matched = true
					break
				}
			}
			if !matched {
				return
			}
		}
	}
	login(true, false)
}
