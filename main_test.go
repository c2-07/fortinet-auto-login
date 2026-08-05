package main

import (
	"os"
	"testing"
)

func TestExtractCountdown(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected int
	}{
		{
			name:     "Valid countdown",
			body:     `<html><body><b id="countdown">3600</b></body></html>`,
			expected: 3600,
		},
		{
			name:     "No countdown in body",
			body:     `<html><body><b id="timer">3600</b></body></html>`,
			expected: defaultCountdown,
		},
		{
			name:     "Empty body",
			body:     "",
			expected: defaultCountdown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCountdown(tt.body)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestRandomMagic(t *testing.T) {
	magic1 := randomMagic()
	magic2 := randomMagic()

	if len(magic1) != 16 {
		t.Errorf("expected magic length 16, got %d", len(magic1))
	}
	if magic1 == magic2 {
		t.Errorf("expected random magic to be different, but got same: %s", magic1)
	}
}

func TestCredentialsSaveAndLoad(t *testing.T) {
	// Create a temporary directory to act as XDG_CACHE_HOME
	tempDir, err := os.MkdirTemp("", "fortinet-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set XDG_CACHE_HOME so credentialsFilePath() uses it
	os.Setenv("XDG_CACHE_HOME", tempDir)
	defer os.Unsetenv("XDG_CACHE_HOME")

	// 1. Initially, no credentials should exist
	_, ok := loadCredentials()
	if ok {
		t.Errorf("expected no credentials to be loaded initially")
	}

	// 2. Save credentials
	expectedCreds := credentials{
		Username: "testuser",
		Password: "testpassword",
	}
	saveCredentials(expectedCreds)

	// Verify file was created and has correct permissions
	path := credentialsFilePath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("credentials file was not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %v", info.Mode().Perm())
	}

	// 3. Load credentials
	loadedCreds, ok := loadCredentials()
	if !ok {
		t.Fatalf("expected credentials to be loaded")
	}
	if loadedCreds.Username != expectedCreds.Username || loadedCreds.Password != expectedCreds.Password {
		t.Errorf("loaded credentials mismatch. expected %v, got %v", expectedCreds, loadedCreds)
	}

	// 4. Delete credentials
	deleteCredentials()
	_, ok = loadCredentials()
	if ok {
		t.Errorf("expected credentials to be deleted")
	}
}
