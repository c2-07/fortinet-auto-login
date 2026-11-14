package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubRepo = "c2-07/fortinet-auto-login"
	releaseURL = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// upgradeCLI checks GitHub releases for a newer version, downloads the appropriate
// binary for the host OS/Arch, and atomically replaces the current executable.
func upgradeCLI() bool {
	logf("\033[34m[UPGRADE]\033[0m Checking for updates from %s...", githubRepo)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		logf("\033[31m[FAIL]\033[0m    Failed to create request: %v", err)
		return false
	}
	req.Header.Set("User-Agent", "fortinet-auto-login-updater")

	resp, err := client.Do(req)
	if err != nil {
		logf("\033[31m[FAIL]\033[0m    Network error fetching release: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logf("\033[31m[FAIL]\033[0m    GitHub API returned status %d", resp.StatusCode)
		return false
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		logf("\033[31m[FAIL]\033[0m    Failed to parse release response: %v", err)
		return false
	}

	latestTag := strings.TrimPrefix(rel.TagName, "v")
	currentVer := strings.TrimPrefix(version, "v")

	logf("  Current version: %s", version)
	logf("  Latest release:  %s", rel.TagName)

	if currentVer != "dev" && currentVer == latestTag {
		logf("\033[32m[OK]\033[0m      Already up to date (%s)", version)
		return true
	}

	// Determine matching asset name for runtime.GOOS and runtime.GOARCH
	// Pattern: fortinet-auto-login-<os>-<arch>[.exe]
	osName := runtime.GOOS
	archName := runtime.GOARCH
	expectedPrefix := fmt.Sprintf("fortinet-auto-login-%s-%s", osName, archName)

	var downloadURL string
	var assetName string
	for _, asset := range rel.Assets {
		if strings.HasPrefix(asset.Name, expectedPrefix) {
			downloadURL = asset.BrowserDownloadURL
			assetName = asset.Name
			break
		}
	}

	if downloadURL == "" {
		logf("\033[31m[FAIL]\033[0m    No binary found matching OS=%s ARCH=%s in release %s", osName, archName, rel.TagName)
		return false
	}

	logf("\033[34m[UPGRADE]\033[0m Downloading %s...", assetName)

	execPath, err := os.Executable()
	if err != nil {
		logf("\033[31m[FAIL]\033[0m    Failed to locate current executable: %v", err)
		return false
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		logf("\033[31m[FAIL]\033[0m    Failed to resolve symlinks for executable: %v", err)
		return false
	}

	tmpFile := execPath + ".tmp"

	dlResp, err := client.Get(downloadURL)
	if err != nil {
		logf("\033[31m[FAIL]\033[0m    Failed to download binary: %v", err)
		return false
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		logf("\033[31m[FAIL]\033[0m    Download failed with HTTP %d", dlResp.StatusCode)
		return false
	}

	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		logf("\033[31m[FAIL]\033[0m    Failed to create temporary file: %v (try running with sudo)", err)
		return false
	}

	_, err = io.Copy(out, dlResp.Body)
	out.Close()
	if err != nil {
		os.Remove(tmpFile)
		logf("\033[31m[FAIL]\033[0m    Failed to write update binary: %v", err)
		return false
	}

	// Atomic replace
	if runtime.GOOS == "windows" {
		oldFile := execPath + ".old"
		_ = os.Remove(oldFile)
		_ = os.Rename(execPath, oldFile)
	}

	if err := os.Rename(tmpFile, execPath); err != nil {
		os.Remove(tmpFile)
		logf("\033[31m[FAIL]\033[0m    Failed to install update: %v (try running with sudo)", err)
		return false
	}

	logf("\033[32m[OK]\033[0m      Successfully upgraded to %s at %s", rel.TagName, execPath)
	return true
}
