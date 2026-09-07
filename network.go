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
	checkURL = "http://detectportal.firefox.com/"

	// Fallback gateway used for logout when no saved session exists — e.g.
	// the script never ran login in this environment, or the cache was
	// cleared. The portal doesn't validate the magic against the actual
	// session, so a random one of the right shape works.
	fallbackScheme = "https"
	fallbackHost   = "192.168.55.253:1003"
)

var portalRe = regexp.MustCompile(`window\.location="([^"]+)"`)

func makeClient() *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		// Don't follow redirects automatically — mirrors allow_redirects=False.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// doWithRetry retries a request-building function up to `retries` times,
// mirroring the Python urllib3 Retry(total=5, backoff_factor=1).
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
			backoff := time.Duration(attempt+1) * time.Second
			time.Sleep(backoff)
		}
	}
	return nil, lastErr
}

func getPortal(client *http.Client) (string, bool, error) {
	resp, err := doWithRetry(client, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, checkURL, nil)
		return req, err
	}, 5)
	if err != nil {
		return "", false, nil // couldn't reach it
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}
	text := strings.TrimSpace(string(body))
	if text == "success" {
		return "", true, nil
	}

	matches := portalRe.FindStringSubmatch(text)
	if matches == nil {
		return "", false, nil
	}
	return matches[1], false, nil
}

func login(quiet bool, isRetry bool) bool {
	client := makeClient()

	if !quiet {
		logf("\033[34m[NET]\033[0m    Checking internet (Mozilla method)...")
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

	if !quiet {
		logf("\033[33m[PORTAL]\033[0m %s", portal)
		logf("\033[34m[AUTH]\033[0m  User: %s", username)
	}

	// Visit the portal first to establish cookies/session.
	pageResp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, portal, nil)
	}, 5)
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

	form := url.Values{}
	form.Set("4Tredir", "http://google.com/")
	form.Set("magic", parsedPortal.RawQuery)
	form.Set("username", username)
	form.Set("password", password)

	postURL := strings.SplitN(portal, "?", 2)[0]
	origin := fmt.Sprintf("%s://%s", parsedPortal.Scheme, parsedPortal.Host)

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
	}, 5)
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
				deleteCredentials()
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
		Scheme:    parsedPortal.Scheme,
		Host:      parsedPortal.Host,
		Magic:     parsedPortal.RawQuery,
		Countdown: extractCountdown(text),
	}
	saveSession(s)

	logf("\033[32m[OK]\033[0m     Auth success (host=%s magic=%s)", s.Host, s.Magic)
	return true
}

func logout() bool {
	s, ok := loadSession()
	if !ok {
		logf("\033[33m[WARN]\033[0m   No session. Using random magic.")
		s = session{Scheme: fallbackScheme, Host: fallbackHost, Magic: randomMagic()}
	}

	logoutURL := fmt.Sprintf("%s://%s/logout?%s", s.Scheme, s.Host, s.Magic)

	client := makeClient()
	resp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, logoutURL, nil)
	}, 5)
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

// keepalive periodically hits the keepalive URL before the session's
// countdown expires, mirroring what the browser's background tab does with
// its timer + redirect. Runs until the process is stopped or an attempt
// exhausts its retries.
func keepalive() {
	s, ok := loadSession()
	if !ok {
		logf("\033[31m[FAIL]\033[0m   No session for keepalive")
		return
	}
	logf("\033[36m[INIT]\033[0m   Keepalive (host=%s magic=%s timeout=%ds)", s.Host, s.Magic, s.Countdown)

	client := makeClient()

	for {
		wait := time.Duration(s.Countdown-30) * time.Second
		if wait <= 0 {
			wait = 5 * time.Second
		}
		time.Sleep(wait)

		keepaliveURL := fmt.Sprintf("%s://%s/keepalive?%s", s.Scheme, s.Host, s.Magic)
		resp, err := doWithRetry(client, func() (*http.Request, error) {
			return http.NewRequest(http.MethodGet, keepaliveURL, nil)
		}, 5)
		if err != nil {
			logf("\033[31m[FAIL]\033[0m   Keepalive err: %v", err)
			return
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logf("\033[31m[FAIL]\033[0m   Keepalive read err")
			return
		}

		s.Countdown = extractCountdown(string(body))
		saveSession(s)
		logf("\033[32m[OK]\033[0m     Keepalive (reset=%ds)", s.Countdown)
	}
}
