package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// Fallback gateway used for logout when no saved session exists — e.g.
	// the script never ran login in this environment, or the cache was
	// cleared. The portal doesn't validate the magic against the actual
	// session, so a random one of the right shape works.
	fallbackScheme = "https"
	fallbackHost   = "192.168.55.253:1003"
)

var portalRe = regexp.MustCompile(`window\.location="([^"]+)"`)

// makeClient builds the captive-portal HTTP client using the configured
// request timeout (config.json: request_timeout_secs).
func makeClient() *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: 30 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   effectiveConfig().requestTimeout(),
		// Don't follow redirects automatically — mirrors allow_redirects=False.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// doWithRetry retries a request-building function up to `retries` times,
// with fast backoff suitable for local network captive portal interactions.
func doWithRetry(client *http.Client, buildReq func() (*http.Request, error), retries int) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt < retries {
			backoff := time.Duration(250*(attempt+1)) * time.Millisecond
			time.Sleep(backoff)
		}
	}
	return nil, lastErr
}

// probeDirectGateway queries the local gateway IP directly when external DNS is
// blackholed or unreachable, returning the captive portal redirect URL in <20ms.
func probeDirectGateway(client *http.Client) (string, error) {
	gwHost := getEffectivePortalHost()
	gwIP := strings.Split(gwHost, ":")[0]
	if gwIP == "" {
		gwIP = "192.168.55.253"
	}

	candidates := []string{gwIP}
	if defaultGW := getDefaultGatewayHint(); defaultGW != "" && defaultGW != gwIP {
		candidates = append(candidates, defaultGW)
	}

	for _, ip := range candidates {
		// 1. Direct HTTP probe to FortiGate captive portal redirect port 1000
		directURL := fmt.Sprintf("http://%s:1000/", ip)
		req, err := http.NewRequest(http.MethodGet, directURL, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if matches := portalRe.FindStringSubmatch(string(body)); len(matches) > 1 {
					return matches[1], nil
				}
			}
		}
	}

	// 2. Direct HTTPS probe to FortiGate auth port
	portalURL := fmt.Sprintf("https://%s/fgtauth", gwHost)
	reqTLS, err := http.NewRequest(http.MethodGet, portalURL, nil)
	if err == nil {
		respTLS, errTLS := client.Do(reqTLS)
		if errTLS == nil {
			io.Copy(io.Discard, respTLS.Body)
			respTLS.Body.Close()
			if respTLS.StatusCode == http.StatusOK || respTLS.StatusCode == http.StatusFound {
				return portalURL, nil
			}
		}
	}

	return "", fmt.Errorf("direct gateway probe failed")
}

// probeResult is the outcome of a single connectivity check URL.
type probeResult int

const (
	probeUnknown   probeResult = iota // reachable, but response not recognised
	probeConnected                    // internet is reachable
	probePortal                       // captive portal intercepted the request
)

// classifyCheckResponse interprets a connectivity check response. It understands
// the common check services (Cloudflare/Google 204, Firefox "success", Apple
// "<TITLE>Success</TITLE>", Microsoft "Microsoft Connect Test") and FortiGate
// interception via a JS redirect body or a Location header pointing at /fgtauth.
func classifyCheckResponse(status int, location, body string) (probeResult, string) {
	if status == http.StatusNoContent {
		return probeConnected, ""
	}
	if m := portalRe.FindStringSubmatch(body); len(m) > 1 {
		return probePortal, m[1]
	}
	if status >= 300 && status < 400 && strings.Contains(location, "/fgtauth") {
		return probePortal, location
	}
	if status == http.StatusOK {
		text := strings.TrimSpace(body)
		if strings.EqualFold(text, "success") ||
			strings.Contains(text, "<TITLE>Success</TITLE>") ||
			strings.HasPrefix(text, "Microsoft Connect Test") {
			return probeConnected, ""
		}
	}
	return probeUnknown, ""
}

func getPortal(client *http.Client) (string, bool, error) {
	cfg := effectiveConfig()

	// Walk the configured check URLs in priority order (config.json:
	// check_urls). The first conclusive answer wins; unreachable or
	// unrecognised endpoints fall through to the next one.
	var lastErr error
	for _, checkURL := range cfg.CheckURLs {
		resp, err := doWithRetry(client, func() (*http.Request, error) {
			return http.NewRequest(http.MethodGet, checkURL, nil)
		}, cfg.MaxRetries)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		result, portal := classifyCheckResponse(resp.StatusCode, resp.Header.Get("Location"), string(body))
		switch result {
		case probeConnected:
			return "", true, nil
		case probePortal:
			return portal, false, nil
		}
	}

	// Fallback to direct gateway IP probe if every external probe failed or
	// timed out (e.g. when logged out and external DNS is blocked).
	directPortal, errDirect := probeDirectGateway(client)
	if errDirect == nil && directPortal != "" {
		return directPortal, false, nil
	}

	return "", false, lastErr
}

// authState is the outcome of a fast-path login check.
type authState int

const (
	authStateUnknown   authState = iota // offline, DNS blocked, or inconclusive
	authStateLoggedIn                   // internet is reachable (no login needed)
	authStateLoggedOut                  // captive portal intercepted (portal URL captured)
)

// fastCheckAuthState performs a single lightweight HTTP GET against the primary
// check URL (no retries, no gateway TLS handshakes) to determine whether the
// user is still logged in. When logged in, cp.cloudflare.com/generate_204
// returns 204 (0-byte body) in ~25-55ms. When logged out, FortiGate intercepts
// port 80 and returns the portal URL + magic token in a single RTT.
func fastCheckAuthState(client *http.Client) (authState, string) {
	cfg := effectiveConfig()
	if len(cfg.CheckURLs) == 0 {
		return authStateUnknown, ""
	}

	req, err := http.NewRequest(http.MethodGet, cfg.CheckURLs[0], nil)
	if err != nil {
		return authStateUnknown, ""
	}

	resp, err := client.Do(req)
	if err != nil {
		return authStateUnknown, ""
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if readErr != nil {
		return authStateUnknown, ""
	}

	result, portal := classifyCheckResponse(resp.StatusCode, resp.Header.Get("Location"), string(body))
	switch result {
	case probeConnected:
		return authStateLoggedIn, ""
	case probePortal:
		return authStateLoggedOut, portal
	default:
		return authStateUnknown, ""
	}
}

func login(quiet bool, isRetry bool) bool {
	client := makeClient()

	if !quiet {
		logf("\033[34m[NET]\033[0m    Checking internet...")
	}
	portal, alreadyConnected, err := getPortal(client)
	if alreadyConnected {
		if !quiet {
			logf("\033[32m[OK]\033[0m     Internet connected")
		}
		return true
	}
	if err != nil || portal == "" {
		if !quiet {
			logf("\033[31m[FAIL]\033[0m   Network unreachable")
		}
		return false
	}

	return loginWithPortal(client, portal, quiet, isRetry)
}

// loginWithPortal completes the FortiGate login handshake using an already
// discovered portal URL, avoiding a duplicate connectivity probe round trip.
func loginWithPortal(client *http.Client, portal string, quiet bool, isRetry bool) bool {
	if client == nil {
		client = makeClient()
	}

	if !quiet {
		logf("\033[33m[PORTAL]\033[0m %s", portal)
		logf("\033[34m[AUTH]\033[0m  User: %s", username)
	}

	// Visit the portal first to establish cookies/session.
	pageResp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, portal, nil)
	}, effectiveConfig().MaxRetries)
	if err != nil {
		logf("Failed to load login page.")
		return false
	}
	defer pageResp.Body.Close()
	io.Copy(io.Discard, pageResp.Body)

	if pageResp.StatusCode != http.StatusOK {
		logf("Failed to load login page (status %d).", pageResp.StatusCode)
		return false
	}

	time.Sleep(200 * time.Millisecond)

	parsedPortal, err := url.Parse(portal)
	if err != nil {
		logf("Couldn't parse portal URL.")
		return false
	}

	magicToken := parsedPortal.RawQuery
	if magicToken == "" {
		magicToken = randomMagic()
	}

	form := url.Values{}
	form.Set("4Tredir", "http://google.com/")
	form.Set("magic", magicToken)
	form.Set("username", username)
	form.Set("password", password)

	postURL := strings.SplitN(portal, "?", 2)[0]
	scheme := parsedPortal.Scheme
	if scheme == "" {
		scheme = fallbackScheme
	}
	host := parsedPortal.Host
	if host == "" {
		host = getEffectivePortalHost()
	}
	origin := fmt.Sprintf("%s://%s", scheme, host)

	resp, err := doWithRetry(client, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, postURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", portal)
		req.Header.Set("Origin", origin)
		req.Header.Set("User-Agent",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "+
				"AppleWebKit/537.36 (KHTML, like Gecko) "+
				"Chrome/138.0 Safari/537.36")
		return req, nil
	}, effectiveConfig().MaxRetries)
	if err != nil {
		logf("Login request failed.")
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logf("Failed to read login response.")
		return false
	}
	text := string(body)

	textLower := strings.ToLower(text)
	if strings.Contains(textLower, "failed") || strings.Contains(textLower, "invalid") {
		if !quiet {
			logf("\033[31m[FAIL]\033[0m   Login rejected by Fortinet")
		}

		isBadCreds := strings.Contains(textLower, "invalid user") ||
			strings.Contains(textLower, "invalid pass") ||
			strings.Contains(textLower, "authentication failed") ||
			strings.Contains(textLower, "login failed") ||
			strings.Contains(textLower, "wrong")

		if !isRetry && !quiet {
			// For manual interactive runs, we can prompt again if it seems like a typo.
			// But to be safe, only prompt if it's explicitly a bad password.
			if isBadCreds {
				// promptCredentials overwrites the saved file (keeping the SSID
				// allow-list), so a Ctrl-C at the prompt doesn't lose anything.
				c := promptCredentials()
				username = c.Username
				password = c.Password
				return login(quiet, true)
			}
		}

		if quiet {
			if isBadCreds {
				deleteCredentials()
				if notifyCredentialsNeeded() {
					logf("\033[33m[WARN]\033[0m   Bad creds deleted. Run manually to update.")
				}
			} else {
				logf("\033[33m[WARN]\033[0m   Session error (Not deleting creds).")
			}
		}
		return false
	}

	// The magic used for logout/keepalive is the same one used for login,
	// carried in the portal URL's query string.
	s := session{
		Scheme:      scheme,
		Host:        host,
		Magic:       magicToken,
		Countdown:   extractCountdown(text),
		RefreshedAt: time.Now().Unix(),
	}
	saveSession(s)

	logf("\033[32m[OK]\033[0m     Auth success (host=%s magic=%s)", s.Host, s.Magic)
	return true
}

func logout() bool {
	s, ok := loadSession()
	if !ok {
		logf("\033[33m[WARN]\033[0m   No session. Using random magic.")
		s = session{Scheme: fallbackScheme, Host: getEffectivePortalHost(), Magic: randomMagic()}
	}

	logoutURL := fmt.Sprintf("%s://%s/logout?%s", s.Scheme, s.Host, s.Magic)

	client := makeClient()
	resp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, logoutURL, nil)
	}, effectiveConfig().MaxRetries)

	// Clean up local session and invalidate detection cache immediately
	clearSession()
	InvalidateDetectionCache()

	if err != nil {
		logf("\033[31m[FAIL]\033[0m   Logout request failed")
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		logf("\033[31m[FAIL]\033[0m   Logout HTTP %d", resp.StatusCode)
		return false
	}
	logf("\033[32m[OK]\033[0m     Logged out (host=%s magic=%s)", s.Host, s.Magic)
	return true
}

// refreshSession calls the portal's keepalive URL, which resets the FortiGate
// authentication timer the same way the browser's keepalive tab does, and
// stores the new countdown in the session cache.
func refreshSession(s session) (session, error) {
	keepaliveURL := fmt.Sprintf("%s://%s/keepalive?%s", s.Scheme, s.Host, s.Magic)
	resp, err := doWithRetry(makeClient(), func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, keepaliveURL, nil)
	}, effectiveConfig().MaxRetries)
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return s, err
	}
	if resp.StatusCode != http.StatusOK {
		return s, fmt.Errorf("keepalive HTTP %d", resp.StatusCode)
	}

	s.Countdown = extractCountdown(string(body))
	s.RefreshedAt = time.Now().Unix()
	return s, saveSession(s)
}

// sessionNeedsRefresh reports whether the session's authentication timer will
// expire within margin. Sessions without a refresh timestamp (written by older
// versions) are left alone; they get one on the next login.
func sessionNeedsRefresh(s session, now time.Time, margin time.Duration) bool {
	if s.RefreshedAt == 0 || s.Countdown <= 0 {
		return false
	}
	lifetime := time.Duration(s.Countdown) * time.Second
	if half := lifetime / 2; margin > half {
		margin = half
	}
	expiresAt := time.Unix(s.RefreshedAt, 0).Add(lifetime)
	return expiresAt.Sub(now) <= margin
}

// maybeRefreshSession sends a keepalive when the saved session is close to
// expiring, so the user isn't dropped offline until the next re-login. On
// failure it stops refreshing that session; the normal re-login path takes
// over if the portal does log the user out.
func maybeRefreshSession(margin time.Duration) {
	s, ok := loadSession()
	if !ok || !sessionNeedsRefresh(s, time.Now(), margin) {
		return
	}

	updated, err := refreshSession(s)
	if err != nil {
		logf("\033[33m[WARN]\033[0m   Session keepalive failed: %v (will re-authenticate if the session drops)", err)
		s.RefreshedAt = 0
		_ = saveSession(s)
		return
	}
	logf("\033[32m[OK]\033[0m     Session keepalive sent (timer reset to %ds)", updated.Countdown)
}
