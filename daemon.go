package main

import (
	"context"
	"strings"
	"time"
)

// daemon runs polling DetectFortinetNetwork() and login() at a fixed interval.
// login() short-circuits fast when already connected, so this is cheap while
// connected and transparently re-authenticates whenever the session drops.
func daemon(ctx context.Context, interval time.Duration) {
	daemonWithTickHandler(ctx, interval, checkDaemonTick)
}

func daemonWithTickHandler(ctx context.Context, interval time.Duration, tickFn func(*bool)) {
	logf("\033[36m[INIT]\033[0m   Daemon started (interval=%s)", interval)

	// Rotate log if it exceeded cap
	rotateLogIfNeeded()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var wasFortinet bool
	for {
		tickFn(&wasFortinet)

		select {
		case <-ctx.Done():
			logf("\033[36m[DAEMON]\033[0m Gracefully stopped by signal.")
			return
		case <-ticker.C:
		}
	}
}

func checkDaemonTick(wasFortinet *bool) {
	det := DetectFortinetNetwork()
	if det.IsFortinetNetwork {
		if !*wasFortinet {
			if det.DeviceSerialNumber != "" {
				logf("\033[32m[NET]\033[0m    Fortinet network detected (device=%s, gateway=%s)", det.DeviceSerialNumber, det.GatewayIP)
			} else {
				logf("\033[32m[NET]\033[0m    Fortinet network detected (portal=%s)", det.CaptivePortalHost)
			}
			*wasFortinet = true
		}
		login(true, false)
	} else {
		if *wasFortinet {
			logf("\033[33m[WARN]\033[0m   Left Fortinet network. Monitoring...")
			*wasFortinet = false
		}
	}
}

func runAutoMode() {
	// Flush detection cache so we always inspect fresh state upon an OS event trigger
	InvalidateDetectionCache()

	c, ok := loadCredentials()
	if !ok {
		if notifyCredentialsNeeded() {
			logf("\033[31m[FAIL]\033[0m   No creds. Run interactively.")
		}
		return
	}
	username = c.Username
	password = c.Password

	// Check if we are on a Fortinet network using our primary detection engine
	det := DetectFortinetNetwork()
	if !det.IsFortinetNetwork {
		// Not a Fortinet network, skip authentication attempt with 0 CPU cost
		return
	}

	// If internet is already reachable, no captive portal interception is occurring
	if det.InternetConnected {
		return
	}

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
