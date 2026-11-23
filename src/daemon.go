package main

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	keepaliveRefreshMargin = 2 * time.Minute
	maxAdaptiveInterval    = 5 * time.Minute
	wakeJumpThreshold      = 15 * time.Second
)

// localWatchInterval controls how often the daemon checks local OS interface
// state and wall-clock jumps (zero network I/O). Overrideable in tests.
var localWatchInterval = 2 * time.Second

// networkFingerprintFunc returns a string representing active non-loopback
// interface addresses. Overrideable in tests.
var networkFingerprintFunc = networkFingerprint

// networkFingerprint snapshots all UP non-loopback interface IP addresses.
// It performs zero network I/O (<0.1ms) and changes immediately on Wi-Fi roam,
// DHCP renewal, interface toggle, or VPN/WARP state change.
func networkFingerprint() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var entries []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			entries = append(entries, iface.Name+"="+addr.String())
		}
	}
	sort.Strings(entries)
	return strings.Join(entries, ";")
}

// adaptiveSchedule backs off network polling while the session is healthy and
// resets immediately on any network event, wake-from-sleep, or auth drop.
type adaptiveSchedule struct {
	base    time.Duration
	current time.Duration
	max     time.Duration
}

func newAdaptiveSchedule(base, max time.Duration) *adaptiveSchedule {
	if base <= 0 {
		base = DefaultDaemonIntervalSecs * time.Second
	}
	if max < base {
		max = base
	}
	return &adaptiveSchedule{base: base, current: base, max: max}
}

func (a *adaptiveSchedule) onHealthy() {
	next := a.current * 2
	if next > a.max {
		next = a.max
	}
	a.current = next
}

func (a *adaptiveSchedule) onEvent() {
	a.current = a.base
}

// nextDelay returns how long to wait before the next outbound probe, clamping
// the delay so a saved session's proactive keepalive is never missed.
func (a *adaptiveSchedule) nextDelay(s session, now time.Time, margin time.Duration) time.Duration {
	delay := a.current
	if s.RefreshedAt > 0 && s.Countdown > 0 {
		lifetime := time.Duration(s.Countdown) * time.Second
		m := margin
		if half := lifetime / 2; m > half {
			m = half
		}
		refreshAt := time.Unix(s.RefreshedAt, 0).Add(lifetime - m)
		untilRefresh := refreshAt.Sub(now)
		if untilRefresh > 0 && untilRefresh < delay {
			if untilRefresh < time.Second {
				untilRefresh = time.Second
			}
			delay = untilRefresh
		}
	}
	return delay
}

// daemon runs an adaptive, event-aware monitoring loop:
//   - Local interface/IP changes and system sleep/wake are detected in <=2s
//     with zero network packets and trigger an immediate connectivity check.
//   - Outbound probes use a single fast-path HTTP check (~25-55ms, 0-byte body)
//     with a reused keep-alive client, skipping gateway TLS handshakes while
//     logged in.
//   - Poll intervals back off exponentially while healthy (base -> 2x -> 4x up
//     to 5m), clamped to the session's keepalive deadline.
func daemon(ctx context.Context, interval time.Duration) {
	sched := newAdaptiveSchedule(interval, maxAdaptiveInterval)
	logf("\033[36m[DAEMON]\033[0m Starting adaptive monitoring daemon (base=%s, max=%s)", sched.base, sched.max)
	logf("\033[36m[INIT]\033[0m   Daemon started (interval=%s)", sched.base)

	rotateLogIfNeeded()

	client := makeClient()
	var wasFortinet bool
	lastFP := networkFingerprintFunc()
	lastWall := time.Now().Round(0)

	runCheck := func() time.Time {
		healthy := checkDaemonState(client, &wasFortinet)
		if healthy {
			sched.onHealthy()
		} else {
			sched.onEvent()
		}
		s, _ := loadSession()
		now := time.Now()
		return now.Add(sched.nextDelay(s, now, keepaliveRefreshMargin))
	}

	nextProbeAt := runCheck()

	watchInterval := localWatchInterval
	if sched.base < watchInterval {
		watchInterval = sched.base
	}
	watcher := time.NewTicker(watchInterval)
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			logf("\033[36m[DAEMON]\033[0m Gracefully stopped by signal.")
			return
		case <-watcher.C:
			nowWall := time.Now().Round(0)
			wallJump := nowWall.Sub(lastWall) > watchInterval+wakeJumpThreshold
			lastWall = nowWall

			fp := networkFingerprintFunc()
			netChanged := fp != lastFP
			lastFP = fp

			if netChanged || wallJump {
				reason := "network interface change"
				if wallJump {
					reason = "system wake"
				}
				logf("\033[36m[NET]\033[0m    Detected %s; verifying session immediately...", reason)
				InvalidateDetectionCache()
				sched.onEvent()
				nextProbeAt = runCheck()
				continue
			}

			if !time.Now().Before(nextProbeAt) {
				rotateLogIfNeeded()
				nextProbeAt = runCheck()
			}
		}
	}
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

// checkDaemonState performs a fast-path check first and escalates to full
// Fortinet detection / login only when logged out or offline. Returns true
// when the connection is healthy (safe to back off polling).
func checkDaemonState(client *http.Client, wasFortinet *bool) bool {
	if allowed := loadExistingSSID(); allowed != "" && !isAllowedSSID(allowed) {
		return true
	}

	state, portalURL := fastCheckAuthState(client)
	switch state {
	case authStateLoggedIn:
		// Already logged in: skip DetectFortinetNetwork() (no 600ms gateway TLS probes)
		maybeRefreshSession(keepaliveRefreshMargin)
		return true

	case authStateLoggedOut:
		// FortiGate intercepted port 80 and already gave us the portal URL + magic token
		if !*wasFortinet {
			logf("\033[32m[NET]\033[0m    Fortinet captive portal intercepted; authenticating...")
			*wasFortinet = true
		}
		authLock, err := acquireProcessLock("auth")
		if err != nil {
			return false
		}
		defer authLock.Release()
		loginWithPortal(client, portalURL, true, false)
		return false

	default:
		// Offline or DNS blackholed: fall back to full Fortinet detection + direct gateway probe
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
			if det.InternetConnected {
				maybeRefreshSession(keepaliveRefreshMargin)
				return true
			}
			authLock, err := acquireProcessLock("auth")
			if err != nil {
				return false
			}
			defer authLock.Release()
			login(true, false)
			return false
		}
		if *wasFortinet {
			logf("\033[33m[WARN]\033[0m   Left Fortinet network. Monitoring...")
			*wasFortinet = false
		}
		return false
	}
}

func checkDaemonTick(wasFortinet *bool) {
	checkDaemonState(makeClient(), wasFortinet)
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

	// Fast path: 1 lightweight HTTP probe (~25-55ms), zero gateway TLS probes if logged in
	client := makeClient()
	state, portalURL := fastCheckAuthState(client)
	switch state {
	case authStateLoggedIn:
		maybeRefreshSession(keepaliveRefreshMargin)
		return

	case authStateLoggedOut:
		authLock, err := acquireProcessLock("auth")
		if err != nil {
			return
		}
		defer authLock.Release()
		loginWithPortal(client, portalURL, true, false)
		return
	}

	// Fallback when DNS is blackholed or external probe failed
	det := DetectFortinetNetwork()
	if !det.IsFortinetNetwork || det.InternetConnected {
		if det.InternetConnected {
			maybeRefreshSession(keepaliveRefreshMargin)
		}
		return
	}

	authLock, err := acquireProcessLock("auth")
	if err != nil {
		return
	}
	defer authLock.Release()
	login(true, false)
}
