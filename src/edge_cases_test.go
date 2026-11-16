package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// 1. LOCKING & CONCURRENCY EDGE CASES
// =============================================================================

// TestLockConcurrencyDaemonAndAuth verifies that the background daemon lock
// and CLI auth lock can be held concurrently without blocking each other.
func TestLockConcurrencyDaemonAndAuth(t *testing.T) {
	daemonLock, err := acquireProcessLock("daemon")
	if err != nil {
		t.Fatalf("failed to acquire daemon lock: %v", err)
	}
	defer daemonLock.Release()

	// While daemon is running, auth lock must be independently acquirable
	authLock, err := acquireProcessLock("auth")
	if err != nil {
		t.Fatalf("auth lock failed while daemon lock was active: %v", err)
	}
	authLock.Release()

	// Warp lock must also be independently acquirable
	warpLock, err := acquireProcessLock("warp")
	if err != nil {
		t.Fatalf("warp lock failed while daemon lock was active: %v", err)
	}
	warpLock.Release()
}

// TestLockMutualExclusionSameName ensures two processes cannot hold the same lock.
func TestLockMutualExclusionSameName(t *testing.T) {
	lockName := fmt.Sprintf("test-mutex-%d", time.Now().UnixNano())
	l1, err := acquireProcessLock(lockName)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer l1.Release()

	l2, err := acquireProcessLock(lockName)
	if err == nil {
		l2.Release()
		t.Fatalf("expected second acquire to fail for %s, but it succeeded", lockName)
	}
}

// TestLockReleaseIdempotency verifies that releasing a lock multiple times does not panic.
func TestLockReleaseIdempotency(t *testing.T) {
	lockName := fmt.Sprintf("test-release-%d", time.Now().UnixNano())
	l, err := acquireProcessLock(lockName)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}

	// Double release must be safe
	l.Release()
	l.Release()

	// Nil receiver must be safe
	var nilLock *ProcessLock
	nilLock.Release()
}

// TestLockFallbackToAppDir verifies that if the temp directory cannot be used,
// acquireProcessLock falls back to the user application locks directory.
func TestLockFallbackToAppDir(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	fallbackDir := filepath.Join(getAppDir(), "locks")
	_ = os.MkdirAll(fallbackDir, 0700)

	lockName := fmt.Sprintf("test-fallback-%d", time.Now().UnixNano())
	l, err := acquireProcessLock(lockName)
	if err != nil {
		t.Fatalf("acquireProcessLock failed with fallback: %v", err)
	}
	defer l.Release()
}

// =============================================================================
// 2. CONFIGURATION EDGE CASES
// =============================================================================

// TestConfigCorruptedJSON verifies loadConfig falls back to defaults when config.json is corrupt.
func TestConfigCorruptedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfgPath := configFilePath()
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0700)
	_ = os.WriteFile(cfgPath, []byte(`{INVALID_JSON:;;`), 0644)

	cfg := loadConfig()
	if cfg.PortalHost != DefaultPortalHost {
		t.Errorf("expected default portal host %s on corrupt JSON, got %s", DefaultPortalHost, cfg.PortalHost)
	}
	if len(cfg.CheckURLs) == 0 || cfg.CheckURLs[0] != DefaultCheckURL {
		t.Errorf("expected default check URL on corrupt JSON, got %v", cfg.CheckURLs)
	}
}

// TestConfigMissingFields verifies loadConfig populates default values when JSON has missing fields.
func TestConfigMissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfgPath := configFilePath()
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0700)
	// Only provide portal_host, omitted check_urls
	_ = os.WriteFile(cfgPath, []byte(`{"portal_host":"10.0.0.1:1003"}`), 0644)

	cfg := loadConfig()
	if cfg.PortalHost != "10.0.0.1:1003" {
		t.Errorf("expected portal host 10.0.0.1:1003, got %s", cfg.PortalHost)
	}
	if len(cfg.CheckURLs) == 0 {
		t.Errorf("expected check URLs to be populated with defaults when omitted")
	}
}

// TestConfigInvalidHostValidation tests validation of invalid host input strings.
func TestConfigInvalidHostValidation(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Empty host must fail
	err := setPortalHost("")
	if err == nil {
		t.Errorf("expected error when setting empty portal host")
	}

	// Valid host with port
	err = setPortalHost("10.100.1.254:8090")
	if err != nil {
		t.Fatalf("expected valid host to succeed, got %v", err)
	}

	// Host without port stores as configured
	err = setPortalHost("10.100.1.254")
	if err != nil {
		t.Fatalf("expected host without port to be accepted, got %v", err)
	}
	cfg := loadConfig()
	if cfg.PortalHost != "10.100.1.254" {
		t.Errorf("expected host 10.100.1.254, got %s", cfg.PortalHost)
	}
}

// TestConfigEnvVarOverride verifies FORTINET_PORTAL_HOST override.
func TestConfigEnvVarOverride(t *testing.T) {
	origEnv := os.Getenv("FORTINET_PORTAL_HOST")
	os.Setenv("FORTINET_PORTAL_HOST", "172.20.0.1:1003")
	defer func() {
		if origEnv != "" {
			os.Setenv("FORTINET_PORTAL_HOST", origEnv)
		} else {
			os.Unsetenv("FORTINET_PORTAL_HOST")
		}
	}()

	host := getEffectivePortalHost()
	if host != "172.20.0.1:1003" {
		t.Errorf("expected env var override 172.20.0.1:1003, got %s", host)
	}
}

// =============================================================================
// 3. CREDENTIALS & SECURITY EDGE CASES
// =============================================================================

// TestCredentialsSpecialCharacters verifies that passwords with quotes, spaces,
// and special characters are preserved accurately through JSON serialization.
func TestCredentialsSpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProf := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProf)
	}()

	specialCreds := credentials{
		Username: "user+name@campus.edu",
		Password: `P@ss"w0rd!#%^&*() _+-=[]{}|;':,.<>/?~`,
		SSID:     "Campus-5G, Campus-Secure",
	}

	if err := saveCredentials(specialCreds); err != nil {
		t.Fatalf("failed to save special credentials: %v", err)
	}

	loaded, ok := loadCredentials()
	if !ok {
		t.Fatalf("failed to load saved special credentials")
	}

	if loaded.Username != specialCreds.Username {
		t.Errorf("username mismatch: got %q, want %q", loaded.Username, specialCreds.Username)
	}
	if loaded.Password != specialCreds.Password {
		t.Errorf("password mismatch: got %q, want %q", loaded.Password, specialCreds.Password)
	}
	if loaded.SSID != specialCreds.SSID {
		t.Errorf("SSID mismatch: got %q, want %q", loaded.SSID, specialCreds.SSID)
	}
}

// TestCredentialsEmptyUserOrPass verifies loadCredentials returns false if username or password is blank.
func TestCredentialsEmptyUserOrPass(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProf := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProf)
	}()

	// 1. Empty username
	saveCredentials(credentials{Username: "", Password: "secretpassword"})
	if _, ok := loadCredentials(); ok {
		t.Errorf("expected loadCredentials to return false for empty username")
	}

	// 2. Empty password
	saveCredentials(credentials{Username: "validuser", Password: ""})
	if _, ok := loadCredentials(); ok {
		t.Errorf("expected loadCredentials to return false for empty password")
	}
}

// TestCredentialsCorruptFile verifies loadCredentials handles corrupt files gracefully.
func TestCredentialsCorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProf := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProf)
	}()

	credPath := credentialsFilePath()
	_ = os.MkdirAll(filepath.Dir(credPath), 0700)
	_ = os.WriteFile(credPath, []byte(`not json at all`), 0600)

	_, ok := loadCredentials()
	if ok {
		t.Errorf("expected loadCredentials to return false for malformed JSON")
	}
}

// =============================================================================
// 4. IN-MEMORY RETRY ENGINE & CAPTIVE REDIRECT PATTERN EDGE CASES
// =============================================================================

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// TestDoWithRetry_SuccessOnSecondAttempt tests retry resilience when an initial request fails.
func TestDoWithRetry_SuccessOnSecondAttempt(t *testing.T) {
	var attempts int32

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			att := atomic.AddInt32(&attempts, 1)
			if att == 1 {
				// Simulate dropped connection / transient network error
				return nil, fmt.Errorf("connection reset by peer")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("success")),
			}, nil
		}),
	}

	resp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, "http://mock.endpoint/check", nil)
	}, 3)
	if err != nil {
		t.Fatalf("expected retry to recover, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK after retry, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

// TestDoWithRetry_AllExhausted verifies doWithRetry returns error after exhausting retries.
func TestDoWithRetry_AllExhausted(t *testing.T) {
	var attempts int32

	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&attempts, 1)
			return nil, fmt.Errorf("network unreachable")
		}),
	}

	resp, err := doWithRetry(client, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, "http://mock.endpoint/check", nil)
	}, 2)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatalf("expected error when all retries fail")
	}

	if atomic.LoadInt32(&attempts) < 2 {
		t.Errorf("expected at least 2 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

// TestPortalURLPatternMatching tests extraction of portal URLs from FortiOS responses.
func TestPortalURLPatternMatching(t *testing.T) {
	cases := []struct {
		name        string
		html        string
		expectURL   string
		expectFound bool
	}{
		{
			name:        "Standard script window.location",
			html:        `<html><body><script>window.location="https://192.168.55.253:1003/fgtauth?0b0c034a70659db0"</script></body></html>`,
			expectURL:   "https://192.168.55.253:1003/fgtauth?0b0c034a70659db0",
			expectFound: true,
		},
		{
			name:        "HTML with 4Tredir encoded parameter",
			html:        `<script>window.location="https://portal.campus.edu:1003/fgtauth?magic=abc&4Tredir=http%3A%2F%2Fgoogle.com"</script>`,
			expectURL:   "https://portal.campus.edu:1003/fgtauth?magic=abc&4Tredir=http%3A%2F%2Fgoogle.com",
			expectFound: true,
		},
		{
			name:        "HTML without quotes inside window.location",
			html:        `<html><body><h1>Normal Page</h1></body></html>`,
			expectURL:   "",
			expectFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := portalRe.FindStringSubmatch(tc.html)
			if tc.expectFound {
				if len(matches) < 2 {
					t.Fatalf("expected redirect URL, found none")
				}
				if matches[1] != tc.expectURL {
					t.Errorf("got %q, want %q", matches[1], tc.expectURL)
				}
			} else {
				if len(matches) > 1 {
					t.Errorf("unexpected match found: %v", matches[1])
				}
			}
		})
	}
}

// =============================================================================
// 5. SESSION & COUNTDOWN PARSING EDGE CASES
// =============================================================================

// TestExtractCountdownEdgeCases tests various HTML formats and corrupt inputs.
func TestExtractCountdownEdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "standard countdown ID tag",
			input:    `<div id="countdown">7200</div>`,
			expected: 7200,
		},
		{
			name:     "standard b countdown tag",
			input:    `<b id="countdown">14400</b>`,
			expected: 14400,
		},
		{
			name:     "countdown zero defaults",
			input:    `<span id="countdown">0</span>`,
			expected: defaultCountdown,
		},
		{
			name:     "negative countdown defaults",
			input:    `<span id="countdown">-500</span>`,
			expected: defaultCountdown,
		},
		{
			name:     "huge countdown",
			input:    `<span id="countdown">86400</span>`,
			expected: 86400,
		},
		{
			name:     "non-numeric content defaults",
			input:    `<span id="countdown">unlimited</span>`,
			expected: defaultCountdown,
		},
		{
			name:     "empty input defaults",
			input:    "",
			expected: defaultCountdown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := extractCountdown(tc.input)
			if actual != tc.expected {
				t.Errorf("extractCountdown(%q) = %d, want %d", tc.input, actual, tc.expected)
			}
		})
	}
}

// TestSessionCorruptedFile verifies loadSession handles corrupted JSON gracefully.
func TestSessionCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProf := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProf)
	}()

	sFile := sessionFilePath()
	_ = os.MkdirAll(filepath.Dir(sFile), 0700)
	_ = os.WriteFile(sFile, []byte("{CORRUPTED_SESSION}"), 0600)

	_, ok := loadSession()
	if ok {
		t.Errorf("expected loadSession to return false on corrupted file")
	}
}

// =============================================================================
// 6. LOGGING & LOG ROTATION EDGE CASES
// =============================================================================

// TestAnsiFilterWriter verifies that all ANSI color and control codes are stripped.
func TestAnsiFilterWriter(t *testing.T) {
	var buf bytes.Buffer
	filter := &ansiFilterWriter{w: &buf}

	input := "\033[31m[FAIL]\033[0m   \033[1mBold text\033[0m \033[36mCyan\033[0m \033[32m[OK]\033[0m"
	expected := "[FAIL]   Bold text Cyan [OK]"

	n, err := filter.Write([]byte(input))
	if err != nil {
		t.Fatalf("filter.Write error: %v", err)
	}
	if n != len(input) {
		t.Errorf("filter.Write returned %d, expected %d", n, len(input))
	}

	got := buf.String()
	if got != expected {
		t.Errorf("ANSI filtering mismatch:\ngot:  %q\nwant: %q", got, expected)
	}
}

// TestLogRotationSizeCap verifies that log file is truncated when exceeding maxLogSize.
func TestLogRotationSizeCap(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	logPath := getLogFilePath()
	_ = os.MkdirAll(filepath.Dir(logPath), 0700)

	// Create a dummy log file exceeding 2MB
	bigData := make([]byte, maxLogSize+1024)
	for i := range bigData {
		bigData[i] = 'A'
	}
	_ = os.WriteFile(logPath, bigData, 0644)

	// Call rotateLogIfNeeded
	rotateLogIfNeeded()

	fi, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("failed to stat rotated log file: %v", err)
	}

	if fi.Size() > maxLogSize {
		t.Errorf("log file was not rotated, size: %d, max: %d", fi.Size(), maxLogSize)
	}
}

// =============================================================================
// 7. WARP CANDIDATE GENERATION & OUTPUT PARSING EDGE CASES
// =============================================================================

// TestWarpCandidateMatrixTotalCount verifies exactly 72 candidates generated.
func TestWarpCandidateMatrixTotalCount(t *testing.T) {
	candidates := generateCandidateEndpoints()
	if len(candidates) != 72 {
		t.Errorf("expected exactly 72 candidate combinations (12 subnets * 6 ports), got %d", len(candidates))
	}

	// Verify each is a valid host:port format
	seen := make(map[string]bool)
	for _, ep := range candidates {
		if seen[ep] {
			t.Errorf("duplicate candidate endpoint: %s", ep)
		}
		seen[ep] = true

		parts := strings.Split(ep, ":")
		if len(parts) != 2 {
			t.Errorf("malformed candidate endpoint format %q", ep)
		}
	}
}

// TestWarpOutputParsingEdgeCases tests various output formats from warp-cli.
func TestWarpOutputParsingEdgeCases(t *testing.T) {
	cases := []struct {
		name         string
		output       string
		wantDisabled bool
		wantFound    bool
	}{
		{
			name:         "plain text disabled",
			output:       "Mode: Warp\nConnectivity Checks Disabled: true\nTunnel: WireGuard",
			wantDisabled: true,
			wantFound:    true,
		},
		{
			name:         "plain text enabled",
			output:       "Mode: Warp\nConnectivity Checks Disabled: false\nTunnel: WireGuard",
			wantDisabled: false,
			wantFound:    true,
		},
		{
			name:         "empty output",
			output:       "",
			wantDisabled: false,
			wantFound:    false,
		},
		{
			name:         "daemon connection error",
			output:       "Error: failed to connect to warp daemon",
			wantDisabled: false,
			wantFound:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			disabled, found := isConnectivityCheckDisabled(tc.output)
			if disabled != tc.wantDisabled || found != tc.wantFound {
				t.Errorf("isConnectivityCheckDisabled(%q) = (%v, %v), want (%v, %v)",
					tc.output, disabled, found, tc.wantDisabled, tc.wantFound)
			}
		})
	}
}

// =============================================================================
// 8. CONCURRENT LOGGING SAFETY
// =============================================================================

// TestConcurrentLoggingSafety verifies that logf can be called concurrently from
// multiple goroutines without data races or memory corruption.
func TestConcurrentLoggingSafety(t *testing.T) {
	var wg sync.WaitGroup
	iterations := 50
	goroutines := 10

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				logf("worker %d iteration %d message", id, i)
			}
		}(g)
	}
	wg.Wait()
}

// TestDaemonContextCancelImmediate verifies daemon terminates immediately when context is cancelled.
func TestDaemonContextCancelImmediate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	stopped := make(chan struct{})
	go func() {
		daemonWithTickHandler(ctx, 100*time.Millisecond, func(b *bool) {})
		close(stopped)
	}()

	select {
	case <-stopped:
		// Succeeded immediately
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("daemon did not terminate promptly with already-cancelled context")
	}
}
