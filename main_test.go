package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// binPath holds the path to the compiled binary for integration testing.
var binPath string

func TestMain(m *testing.M) {
	// Compile the binary for testing
	dir, err := os.MkdirTemp("", "fortinet-test")
	if err != nil {
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "fortinet-auto-login")
	cmd := exec.Command("go", "build", "-o", binPath)
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}

	// Run the tests
	os.Exit(m.Run())
}

func TestFlags(t *testing.T) {
	fakeHome := t.TempDir()
	fakeCache := t.TempDir()

	tests := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
		wantExit   int
	}{
		{
			name:       "version flag",
			args:       []string{"-version"},
			wantStdout: "fortinet-auto-login dev, commit none, built at unknown",
			wantExit:   0,
		},
		{
			name:       "missing credentials requires both",
			args:       []string{"-u", "user"},
			wantStdout: "-username and -password require each other",
			wantExit:   1,
		},
		{
			name:       "help flag",
			args:       []string{"-h"},
			wantStderr: "Usage of", // flag package prints help to stderr
			wantExit:   0,          // flag.Parse() exits with 0 on -h
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binPath, tt.args...)
			cmd.Env = append(os.Environ(),
				"HOME="+fakeHome,
				"XDG_CACHE_HOME="+fakeCache,
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}

			if exitCode != tt.wantExit {
				t.Errorf("got exit code %d, want %d\nStderr: %s", exitCode, tt.wantExit, stderr.String())
			}

			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout %q does not contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr %q does not contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestNetworkAndDaemonFlags(t *testing.T) {
	fakeHome := t.TempDir()

	// Provide fake credentials so it doesn't block on promptCredentials()
	appDir := filepath.Join(fakeHome, ".fortinet-autologin")
	os.MkdirAll(appDir, 0700)
	credPath := filepath.Join(appDir, "credentials.json")
	os.WriteFile(credPath, []byte(`{"username":"testuser","password":"testpass"}`), 0600)

	tests := []struct {
		name       string
		args       []string
		wantStdout string
		killAfter  time.Duration
	}{
		{
			name:       "logout mode",
			args:       []string{"-logout"},
			wantStdout: "No session. Using random magic.",
		},
		{
			name:       "keepalive mode",
			args:       []string{"-keepalive"},
			wantStdout: "No session for keepalive",
		},
		{
			name:       "daemon mode",
			args:       []string{"-daemon", "-interval", "1s"},
			wantStdout: "Daemon started",
			killAfter:  500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binPath, tt.args...)
			// Use an invalid proxy to fail real network requests instantly
			cmd.Env = append(os.Environ(),
				"HOME="+fakeHome,
				"http_proxy=http://127.0.0.1:1",
				"https_proxy=http://127.0.0.1:1",
			)

			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stdout

			if tt.killAfter > 0 {
				cmd.Start()
				time.Sleep(tt.killAfter)
				cmd.Process.Kill()
				cmd.Wait()
			} else {
				cmd.Run()
			}

			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("output %q does not contain %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}

func TestServiceMode(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS service install test on non-darwin platform")
	}

	fakeHome := t.TempDir()

	// The launch agent directory needs to exist in the fake home
	launchAgentsDir := filepath.Join(fakeHome, "Library", "LaunchAgents")
	err := os.MkdirAll(launchAgentsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create fake LaunchAgents dir: %v", err)
	}

	// Fake credentials to bypass prompt
	appDir := filepath.Join(fakeHome, ".fortinet-autologin")
	os.MkdirAll(appDir, 0700)
	credPath := filepath.Join(appDir, "credentials.json")
	os.WriteFile(credPath, []byte(`{"username":"testuser","password":"testpass"}`), 0600)

	// Test Installation
	cmd := exec.Command(binPath, "-install")
	cmd.Env = append(os.Environ(), "HOME="+fakeHome)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout

	// It's expected that launchctl load might fail since the paths are fake,
	// so we just run it and check if the file was created successfully.
	cmd.Run()

	plistPath := filepath.Join(launchAgentsDir, "com.fortinet.autologin.plist")
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		t.Fatalf("LaunchAgent plist was not created at %s\nOutput: %s", plistPath, stdout.String())
	}

	content, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("failed to read plist: %v", err)
	}

	if !strings.Contains(string(content), binPath) {
		t.Errorf("plist does not contain the binary path: %s", string(content))
	}
	if !strings.Contains(string(content), "-auto") {
		t.Errorf("plist does not contain the -auto flag: %s", string(content))
	}

	// Test Uninstallation
	cmdUninstall := exec.Command(binPath, "-uninstall")
	cmdUninstall.Env = append(os.Environ(), "HOME="+fakeHome)

	var stdoutUninstall bytes.Buffer
	cmdUninstall.Stdout = &stdoutUninstall
	cmdUninstall.Stderr = &stdoutUninstall

	cmdUninstall.Run()

	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Fatalf("LaunchAgent plist was not removed after -uninstall\nOutput: %s", stdoutUninstall.String())
	}

	if !strings.Contains(stdoutUninstall.String(), "Uninstalled successfully") {
		t.Errorf("Expected success message, got: %s", stdoutUninstall.String())
	}
}
