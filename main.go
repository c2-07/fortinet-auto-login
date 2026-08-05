package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	username = "bt25cse138"
	password = "********"
	checkURL = "http://detectportal.firefox.com/"

	fallbackScheme = "https"
	fallbackHost   = "192.168.55.253:1003"
)

// Magic is the token issued per session in the portal redirect — the same
// value is reused for login, logout, and keepalive. It changes each new
// session, so it's captured at login time and stashed here for later
// -logout / -keepalive calls rather than hardcoded.
type session struct {
	Scheme    string `json:"scheme"`
	Host      string `json:"host"`
	Magic     string `json:"magic"`
	Countdown int    `json:"countdown"` // seconds, from the keepalive page
}

var countdownRe = regexp.MustCompile(`id="countdown">(\d+)<`)

const defaultCountdown = 14400 // seconds; fallback if the page doesn't have one

// extractCountdown pulls the refresh interval out of the keepalive page's
// <b id="countdown">N</b>, falling back to the default if not found.
func extractCountdown(body string) int {
	if m := countdownRe.FindStringSubmatch(body); m != nil {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > 0 {
			return n
		}
	}
	return defaultCountdown
}

func sessionFilePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "captive-portal-session.json")
}

func saveSession(s session) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(sessionFilePath(), data, 0o600)
}

func loadSession() (session, bool) {
	var s session
	data, err := os.ReadFile(sessionFilePath())
	if err != nil {
		return s, false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, false
	}
	return s, s.Host != "" && s.Magic != ""
}

// randomMagic generates a random hex string shaped like the real magic
// tokens (16 hex chars), for use when we have no saved session to logout
// with.
func randomMagic() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// Extremely unlikely, but fall back to something well-formed anyway.
		return "0000000000000000"
	}
	return hex.EncodeToString(buf)
}

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
		return "", false, nil // couldn't reach it, treat like Python's "return None"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}
	text := strings.TrimSpace(string(body))
	if text == "success" {
		fmt.Println("Already connected.")
		return "", true, nil
	}

	matches := portalRe.FindStringSubmatch(text)
	if matches == nil {
		return "", false, nil
	}
	return matches[1], false, nil
}

func login() bool {
	client := makeClient()

	portal, alreadyConnected, err := getPortal(client)
	if alreadyConnected {
		return true
	}
	if err != nil || portal == "" {
		fmt.Println("Couldn't detect captive portal.")
		return false
	}

	// Visit the portal first to establish cookies/session.
	pageResp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, portal, nil)
	}, 5)
	if err != nil {
		fmt.Println("Failed to load login page.")
		return false
	}
	defer pageResp.Body.Close()
	io.Copy(io.Discard, pageResp.Body)

	if pageResp.StatusCode != http.StatusOK {
		fmt.Println("Failed to load login page.")
		return false
	}

	time.Sleep(200 * time.Millisecond)

	parsedPortal, err := url.Parse(portal)
	if err != nil {
		fmt.Println("Couldn't parse portal URL.")
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
		fmt.Println("Login request failed.")
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to read login response.")
		return false
	}
	text := string(body)

	if strings.Contains(text, "Failed") || strings.Contains(text, "Invalid") {
		fmt.Println("Login failed.")
		return false
	}

	// The magic used for logout/keepalive is the same one used for login,
	// carried in the portal URL's query string.
	saveSession(session{
		Scheme:    parsedPortal.Scheme,
		Host:      parsedPortal.Host,
		Magic:     parsedPortal.RawQuery,
		Countdown: extractCountdown(text),
	})

	fmt.Println("Login successful.")
	return true
}

func logout() bool {
	s, ok := loadSession()
	if !ok {
		fmt.Println("No saved session found — using a random magic against the default gateway.")
		s = session{Scheme: fallbackScheme, Host: fallbackHost, Magic: randomMagic()}
	}

	logoutURL := fmt.Sprintf("%s://%s/logout?%s", s.Scheme, s.Host, s.Magic)

	client := makeClient()
	resp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, logoutURL, nil)
	}, 5)
	if err != nil {
		fmt.Println("Logout request failed.")
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Logout returned status %d.\n", resp.StatusCode)
		return false
	}
	fmt.Println("Logged out.")
	return true
}

// keepalive periodically hits the keepalive URL before the session's
// countdown expires, mirroring what the browser's background tab does with
// its timer + redirect. Runs until the process is stopped or an attempt
// exhausts its retries.
func keepalive() {
	s, ok := loadSession()
	if !ok {
		fmt.Println("No saved session found — log in at least once before using -keepalive.")
		return
	}

	client := makeClient()

	for {
		wait := time.Duration(s.Countdown-30) * time.Second
		if wait <= 0 {
			wait = 5 * time.Second
		}
		fmt.Printf("Sleeping %s before next keepalive hit.\n", wait)
		time.Sleep(wait)

		keepaliveURL := fmt.Sprintf("%s://%s/keepalive?%s", s.Scheme, s.Host, s.Magic)
		resp, err := doWithRetry(client, func() (*http.Request, error) {
			return http.NewRequest(http.MethodGet, keepaliveURL, nil)
		}, 5)
		if err != nil {
			fmt.Println("Keepalive request failed, stopping:", err)
			return
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Println("Failed to read keepalive response, stopping.")
			return
		}

		s.Countdown = extractCountdown(string(body))
		saveSession(s)
		fmt.Println("Keepalive refreshed.")
	}
}

func main() {
	logoutFlag := flag.Bool("logout", false, "log out of the captive portal instead of logging in")
	keepaliveFlag := flag.Bool("keepalive", false, "keep the current session alive instead of logging in")
	flag.Parse()

	if *logoutFlag {
		if !logout() {
			fmt.Println("Unable to log out.")
		}
		return
	}

	if *keepaliveFlag {
		keepalive()
		return
	}

	success := false
	for attempt := 1; attempt <= 5; attempt++ {
		fmt.Printf("Attempt %d\n", attempt)
		if login() {
			success = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !success {
		fmt.Println("Unable to authenticate.")
	}
}
