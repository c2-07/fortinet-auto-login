package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConfigSettingsHonoured(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	// 1. CheckURLs failover: primary returns 503, secondary returns 204
	var primaryHits, secondaryHits int
	srvPrimary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srvPrimary.Close()

	srvSecondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryHits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srvSecondary.Close()

	cfg := appConfig{
		PortalHost:         "127.0.0.1:1",
		CheckURLs:          []string{srvPrimary.URL, srvSecondary.URL},
		DaemonIntervalSecs: 17,
		RequestTimeoutSecs: 7,
		MaxRetries:         1,
	}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	// Verify makeClient uses RequestTimeoutSecs
	client := makeClient()
	if client.Timeout != 7*time.Second {
		t.Errorf("makeClient Timeout = %v, want 7s", client.Timeout)
	}

	// Verify getPortal fails over from primary to secondary
	portal, connected, err := getPortal(client)
	if err != nil || !connected || portal != "" {
		t.Fatalf("getPortal() = (%q, %v, %v), want (\"\", true, nil)", portal, connected, err)
	}
	if primaryHits == 0 || secondaryHits != 1 {
		t.Errorf("expected primaryHits>0 and secondaryHits=1, got primary=%d secondary=%d", primaryHits, secondaryHits)
	}

	// Verify FORTINET_CHECK_URL and FORTINET_INTERVAL env overrides
	t.Setenv("FORTINET_CHECK_URL", srvSecondary.URL)
	t.Setenv("FORTINET_INTERVAL", "23")
	eff := effectiveConfig()
	if len(eff.CheckURLs) != 1 || eff.CheckURLs[0] != srvSecondary.URL {
		t.Errorf("FORTINET_CHECK_URL not applied: %v", eff.CheckURLs)
	}
	if eff.daemonInterval() != 23*time.Second {
		t.Errorf("FORTINET_INTERVAL not applied: got %v, want 23s", eff.daemonInterval())
	}
}

func TestSSIDFilterAndPreservation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	if err := setAllowedSSIDs(" Campus-5G , Campus-Secure "); err != nil {
		t.Fatalf("setAllowedSSIDs failed: %v", err)
	}

	// Saving credentials via initCredentials must preserve existing SSID allow-list
	initCredentials("alice", "secret123")
	c, ok := loadCredentials()
	if !ok {
		t.Fatalf("loadCredentials returned false")
	}
	if c.SSID != "Campus-5G, Campus-Secure" {
		t.Errorf("SSID was not preserved: got %q", c.SSID)
	}

	// Exact case-insensitive matching (no substring false positives)
	if !ssidMatches(c.SSID, "campus-5g") {
		t.Errorf("expected exact case-insensitive match for campus-5g")
	}
	if ssidMatches(c.SSID, "Campus-5G-Guest") {
		t.Errorf("substring Campus-5G-Guest must not match Campus-5G")
	}

	// Clearing SSID filter
	if err := setAllowedSSIDs(""); err != nil {
		t.Fatalf("clearing SSID failed: %v", err)
	}
	c, ok = loadCredentials()
	if !ok || c.SSID != "" {
		t.Errorf("expected cleared SSID and intact creds, got ok=%v ssid=%q", ok, c.SSID)
	}
	if !ssidMatches(c.SSID, "AnyNetwork") {
		t.Errorf("empty allow-list should match any network")
	}
}

func TestKeepaliveRefresh(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	now := time.Now()
	// Legacy session without RefreshedAt should not trigger
	if sessionNeedsRefresh(session{Countdown: 300}, now, 2*time.Minute) {
		t.Errorf("session with RefreshedAt=0 should not trigger refresh")
	}
	// Fresh session should not trigger
	if sessionNeedsRefresh(session{Countdown: 3600, RefreshedAt: now.Unix()}, now, 2*time.Minute) {
		t.Errorf("fresh 1h session should not trigger refresh immediately")
	}
	// Short countdown caps margin at half lifetime (30s for 60s countdown)
	if sessionNeedsRefresh(session{Countdown: 60, RefreshedAt: now.Add(-10 * time.Second).Unix()}, now, 2*time.Minute) {
		t.Errorf("60s session at t=10s should not refresh before half-life")
	}
	if !sessionNeedsRefresh(session{Countdown: 60, RefreshedAt: now.Add(-35 * time.Second).Unix()}, now, 2*time.Minute) {
		t.Errorf("60s session at t=35s should trigger refresh")
	}

	// End-to-end maybeRefreshSession against a mock portal server
	var hitPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPath = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><b id="countdown">7200</b></html>`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	s := session{
		Scheme:      u.Scheme,
		Host:        u.Host,
		Magic:       "deadbeef01234567",
		Countdown:   300,
		RefreshedAt: now.Add(-250 * time.Second).Unix(),
	}
	if err := saveSession(s); err != nil {
		t.Fatalf("saveSession failed: %v", err)
	}

	maybeRefreshSession(2 * time.Minute)

	if hitPath != "/keepalive?deadbeef01234567" {
		t.Errorf("keepalive hit %q, want /keepalive?deadbeef01234567", hitPath)
	}
	updated, ok := loadSession()
	if !ok {
		t.Fatalf("loadSession failed after keepalive")
	}
	if updated.Countdown != 7200 {
		t.Errorf("updated Countdown = %d, want 7200", updated.Countdown)
	}
	if updated.RefreshedAt <= s.RefreshedAt {
		t.Errorf("expected RefreshedAt to advance, got %d <= %d", updated.RefreshedAt, s.RefreshedAt)
	}
}

func TestDisconnectWarpRestoresSettings(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	origRunner := cmdRunner
	defer func() { cmdRunner = origRunner }()

	var cmds []string
	cmdRunner = func(name string, args ...string) ([]byte, error) {
		full := name + " " + strings.Join(args, " ")
		cmds = append(cmds, full)
		if full == "warp-cli settings list" {
			return []byte("Connectivity Checks Disabled: false"), nil
		}
		return []byte("Success"), nil
	}

	// First disable connectivity check (records that it was previously enabled)
	if !ensureWarpConnectivityCheckDisabled() {
		t.Fatalf("ensureWarpConnectivityCheckDisabled failed")
	}
	cmds = nil

	// Disconnect must reset endpoint, protocol, and re-enable connectivity check
	if !DisconnectWarp() {
		t.Fatalf("DisconnectWarp failed")
	}

	want := []string{
		"warp-cli disconnect",
		"warp-cli tunnel endpoint reset",
		"warp-cli tunnel protocol reset",
		"warp-cli debug connectivity-check enable",
	}
	if len(cmds) != len(want) {
		t.Fatalf("commands = %v, want %v", cmds, want)
	}
	for i := range want {
		if cmds[i] != want[i] {
			t.Errorf("cmd[%d] = %q, want %q", i, cmds[i], want[i])
		}
	}

	// Second disconnect should NOT re-enable connectivity check since state was cleared
	cmds = nil
	if !DisconnectWarp() {
		t.Fatalf("second DisconnectWarp failed")
	}
	for _, c := range cmds {
		if strings.Contains(c, "connectivity-check enable") {
			t.Errorf("did not expect connectivity-check enable on second disconnect: %v", cmds)
		}
	}
}

func TestUninstallConfirmationAndPathCleanup(t *testing.T) {
	if confirmUninstall(strings.NewReader("")) {
		t.Errorf("empty input must cancel uninstall")
	}
	if confirmUninstall(strings.NewReader("n\n")) {
		t.Errorf("'n' must cancel uninstall")
	}
	if !confirmUninstall(strings.NewReader("y\n")) {
		t.Errorf("'y' must confirm uninstall")
	}
	if !confirmUninstall(strings.NewReader("YES\n")) {
		t.Errorf("'YES' must confirm uninstall")
	}

	input := strings.Join([]string{
		`alias repo="cd ~/dev/fortinet-auto-login"`,
		`export EDITOR=vim`,
		``,
		`# Added by fortinet-auto-login installer`,
		`export PATH="$HOME/.local/bin:$PATH"`,
		`export FOO=bar`,
		``,
	}, "\n")

	got, changed := stripInstallerPathBlock(input)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if strings.Contains(got, "# Added by fortinet-auto-login installer") {
		t.Errorf("installer comment was not removed:\n%s", got)
	}
	if strings.Contains(got, `export PATH="$HOME/.local/bin:$PATH"`) {
		t.Errorf("installer PATH export was not removed:\n%s", got)
	}
	if !strings.Contains(got, `alias repo="cd ~/dev/fortinet-auto-login"`) {
		t.Errorf("unrelated user line mentioning fortinet-auto-login was deleted:\n%s", got)
	}
	if !strings.Contains(got, `export FOO=bar`) {
		t.Errorf("subsequent user line was deleted:\n%s", got)
	}
}

func TestBuildWindowsTaskXML(t *testing.T) {
	xmlOut := buildWindowsTaskXML(`C:\Users\A&B\autologin.exe`)

	required := []string{
		`<DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>`,
		`<StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>`,
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>`,
		`<Hidden>true</Hidden>`,
		`<RestartOnFailure>`,
		`<Command>C:\Users\A&amp;B\autologin.exe</Command>`,
		`<Arguments>-daemon</Arguments>`,
	}
	for _, needle := range required {
		if !strings.Contains(xmlOut, needle) {
			t.Errorf("Windows task XML missing %q:\n%s", needle, xmlOut)
		}
	}
}

func TestStripInstallerPathBlock_NoOpWhenAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	orig := "export PATH=\"$HOME/.local/bin:$PATH\"\n# my own comment about fortinet-auto-login\n"
	got, changed := stripInstallerPathBlock(orig)
	if changed || got != orig {
		t.Errorf("expected no-op when installer block is absent; changed=%v got=%q", changed, got)
	}
	_ = os.Getenv("HOME")
}
