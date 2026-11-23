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
	logf("\033[36m[DAEMON]\033[0m Starting background monitoring daemon (interval=%s)", interval)
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

const keepaliveRefreshMargin = 2 * time.Minute

func isAllowedSSID(allowedRaw string) bool {
	if len(parseSSIDList(allowedRaw)) == 0 {
		return true
	}
	current, err := getCurrentSSID()
	if err != nil {
		if strings.Contains(err.Error(), "Location Privacy") {
			// macOS blocked SSID detection; fall back to checking the portal directly.
			return true
		}
		// Disconnected from Wi-Fi.
		return false
	}
	return ssidMatches(allowedRaw, current)
}

func checkDaemonTick(wasFortinet *bool) {
	if allowed := loadExistingSSID(); allowed != "" && !isAllowedSSID(allowed) {
		return
	}

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
		// If internet is already connected, refresh the session proactively if it's nearing expiry
		if det.InternetConnected {
			maybeRefreshSession(keepaliveRefreshMargin)
			return
		}
		authLock, err := acquireProcessLock("auth")
		if err != nil {
			// Another process (e.g. manual CLI) is currently authenticating, yield this tick
			return
		}
		defer authLock.Release()
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

	// Enforce SSID allow-list before spending time on network probes
	if !isAllowedSSID(c.SSID) {
		return
	}

	// Check if we are on a Fortinet network using our primary detection engine
	det := DetectFortinetNetwork()
	if !det.IsFortinetNetwork {
		// Not a Fortinet network, skip authentication attempt with 0 CPU cost
		return
	}

	// If internet is already reachable, refresh the session if it's close to expiring
	if det.InternetConnected {
		maybeRefreshSession(keepaliveRefreshMargin)
		return
	}

	authLock, err := acquireProcessLock("auth")
	if err != nil {
		return
	}
	defer authLock.Release()
	login(true, false)
}
