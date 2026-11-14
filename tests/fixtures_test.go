package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var portalRe = regexp.MustCompile(`window\.location="([^"]+)"`)
var countdownRe = regexp.MustCompile(`id="countdown">(\d+)<`)

func TestCaptivePortalFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("assets", "captive_portal.html"))
	if err != nil {
		t.Fatalf("failed to read captive_portal.html: %v", err)
	}

	matches := portalRe.FindStringSubmatch(string(data))
	if len(matches) < 2 {
		t.Fatalf("failed to extract portal URL from fixture")
	}

	expectedPrefix := "https://192.168.55.253:1003/fgtauth?"
	if !strings.HasPrefix(matches[1], expectedPrefix) {
		t.Errorf("expected URL starting with %s, got %s", expectedPrefix, matches[1])
	}
}

func TestKeepaliveFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("assets", "keepalive.html"))
	if err != nil {
		t.Fatalf("failed to read keepalive.html: %v", err)
	}

	matches := countdownRe.FindStringSubmatch(string(data))
	if len(matches) < 2 {
		t.Fatalf("failed to extract countdown from fixture")
	}

	countdown, err := strconv.Atoi(matches[1])
	if err != nil || countdown != 14400 {
		t.Errorf("expected countdown 14400, got %d (err: %v)", countdown, err)
	}
}

func TestLoginResultFixtures(t *testing.T) {
	successData, err := os.ReadFile(filepath.Join("assets", "login_success.html"))
	if err != nil {
		t.Fatalf("failed to read login_success.html: %v", err)
	}
	successText := strings.ToLower(string(successData))
	if strings.Contains(successText, "failed") || strings.Contains(successText, "invalid") {
		t.Errorf("login_success.html should not contain failure signatures")
	}

	failedData, err := os.ReadFile(filepath.Join("assets", "login_failed.html"))
	if err != nil {
		t.Fatalf("failed to read login_failed.html: %v", err)
	}
	failedText := strings.ToLower(string(failedData))
	if !strings.Contains(failedText, "failed") && !strings.Contains(failedText, "invalid") {
		t.Errorf("login_failed.html should contain failure signatures")
	}
}
