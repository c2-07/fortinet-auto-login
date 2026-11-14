package main

import (
	"os"
	"testing"
)

func TestConfigDefaultsAndPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := loadConfig()
	if cfg.PortalHost != DefaultPortalHost {
		t.Errorf("expected portal host %s, got %s", DefaultPortalHost, cfg.PortalHost)
	}
	if len(cfg.CheckURLs) != 1 || cfg.CheckURLs[0] != DefaultCheckURL {
		t.Errorf("expected check URL %s, got %v", DefaultCheckURL, cfg.CheckURLs)
	}

	// Update host
	err := setPortalHost("172.16.1.1:1003")
	if err != nil {
		t.Fatalf("setPortalHost failed: %v", err)
	}

	loaded := loadConfig()
	if loaded.PortalHost != "172.16.1.1:1003" {
		t.Errorf("expected updated host 172.16.1.1:1003, got %s", loaded.PortalHost)
	}

	// Reset
	err = resetConfig()
	if err != nil {
		t.Fatalf("resetConfig failed: %v", err)
	}

	resetLoaded := loadConfig()
	if resetLoaded.PortalHost != DefaultPortalHost {
		t.Errorf("expected reset host %s, got %s", DefaultPortalHost, resetLoaded.PortalHost)
	}
}

func TestClearCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	saveCredentials(credentials{Username: "testuser", Password: "testpass"})
	saveSession(session{Host: "test-host", Magic: "12345"})

	if _, ok := loadCredentials(); !ok {
		t.Fatalf("credentials should be present")
	}
	if _, ok := loadSession(); !ok {
		t.Fatalf("session should be present")
	}

	clearCredentials()

	if _, ok := loadCredentials(); ok {
		t.Errorf("credentials should be deleted after clearCredentials")
	}
	if _, ok := loadSession(); ok {
		t.Errorf("session should be deleted after clearCredentials")
	}
}

func TestProcessLock(t *testing.T) {
	lock1, err := acquireProcessLock("testlock")
	if err != nil {
		t.Fatalf("lock1 acquisition failed: %v", err)
	}
	defer lock1.Release()

	// Second acquisition should fail
	_, err = acquireProcessLock("testlock")
	if err == nil {
		t.Errorf("expected lock2 acquisition to fail while lock1 is held")
	}

	// Release lock1
	lock1.Release()

	// Now third should succeed
	lock3, err := acquireProcessLock("testlock")
	if err != nil {
		t.Fatalf("lock3 acquisition failed after release: %v", err)
	}
	lock3.Release()
}
