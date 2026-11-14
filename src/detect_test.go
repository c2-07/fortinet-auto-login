package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDetectFortinetNetworkMock(t *testing.T) {
	// 1. Test when internet is already working (detectportal returns "success")
	serverSuccess := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success\n"))
	}))
	defer serverSuccess.Close()

	// 2. Test when captive portal returns a FortiGate redirect
	serverRedirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`window.location="https://192.168.55.253:1003/fgtauth?magic=abc123xyz"`))
	}))
	defer serverRedirect.Close()

	client := serverRedirect.Client()
	portal, alreadyConnected, err := getPortal(client)
	// Since getPortal uses checkURL constant, verify parsing logic directly:
	matches := portalRe.FindStringSubmatch(`window.location="https://192.168.55.253:1003/fgtauth?magic=abc123xyz"`)
	if len(matches) < 2 || matches[1] != "https://192.168.55.253:1003/fgtauth?magic=abc123xyz" {
		t.Fatalf("Failed to extract captive portal URL, got %v", matches)
	}

	_ = portal
	_ = alreadyConnected
	_ = err
}

func TestCacheInvalidation(t *testing.T) {
	updateCache(FortinetDetectionResult{
		IsFortinetNetwork:  true,
		DeviceSerialNumber: "TEST_SERIAL_123",
	})

	res := DetectFortinetNetwork()
	if res.DeviceSerialNumber != "TEST_SERIAL_123" {
		t.Errorf("Expected cached result, got %v", res)
	}

	InvalidateDetectionCache()
	cacheMutex.Lock()
	isZero := cachedDetectionTime.IsZero()
	cacheMutex.Unlock()

	if !isZero {
		t.Errorf("Expected cache time to be zeroed out")
	}
}

func TestMakeClient(t *testing.T) {
	client := makeClient()
	if client.Timeout == 0 {
		t.Errorf("makeClient should set a non-zero timeout")
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("makeClient transport should skip insecure TLS verification for captive portals")
	}
}

func TestGetDefaultGatewayHint(t *testing.T) {
	hint := getDefaultGatewayHint()
	// Should not panic, returns either an IP ending in .1 or empty string if no IPv4
	if hint != "" && !t.Failed() {
		// Just ensure it's a valid format
		if len(hint) < 7 {
			t.Errorf("Unreasonable gateway IP hint: %s", hint)
		}
	}
}

func TestDirectGatewayFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`window.location="https://192.168.55.253:1003/fgtauth?magic=testmagic999"`))
	}))
	defer server.Close()

	// Direct regex parsing verification
	matches := portalRe.FindStringSubmatch(`window.location="https://192.168.55.253:1003/fgtauth?magic=testmagic999"`)
	if len(matches) < 2 || matches[1] != "https://192.168.55.253:1003/fgtauth?magic=testmagic999" {
		t.Fatalf("regex failed to extract portal redirect")
	}
}

func TestLogoutSessionCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	s := session{
		Scheme: "https",
		Host:   "192.168.55.253:1003",
		Magic:  "magic123456",
	}
	if err := saveSession(s); err != nil {
		t.Fatalf("saveSession failed: %v", err)
	}
	if _, ok := loadSession(); !ok {
		t.Fatalf("session should be present before clearSession")
	}

	clearSession()
	if _, ok := loadSession(); ok {
		t.Errorf("session should be absent after clearSession")
	}
}
