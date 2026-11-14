package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestCandidateEndpoints(t *testing.T) {
	endpoints := generateCandidateEndpoints()
	if len(endpoints) != 72 {
		t.Fatalf("expected 72 endpoints, got %d", len(endpoints))
	}

	seen := make(map[string]bool)
	for _, ep := range endpoints {
		if seen[ep] {
			t.Errorf("duplicate endpoint found: %s", ep)
		}
		seen[ep] = true
	}

	// Verify top candidates
	hasTopCandidate := false
	for _, ep := range endpoints[:6] {
		if ep == "162.159.192.1:500" || ep == "162.159.192.1:4500" {
			hasTopCandidate = true
			break
		}
	}
	if !hasTopCandidate {
		t.Errorf("expected high priority endpoints in first candidates")
	}
}

func TestWarpPoolCounts(t *testing.T) {
	if len(warpAnycastIPs) != 12 {
		t.Errorf("expected 12 anycast IPs, got %d", len(warpAnycastIPs))
	}
	if len(warpPorts) != 6 {
		t.Errorf("expected 6 ports, got %d", len(warpPorts))
	}
}

func TestIsConnectivityCheckDisabled(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantDisabled bool
		wantFound    bool
	}{
		{
			name:         "plain text already disabled",
			output:       "Mode: Warp\n(user set) Connectivity Checks Disabled: true\nTunnel Protocol: WireGuard",
			wantDisabled: true,
			wantFound:    true,
		},
		{
			name:         "plain text enabled",
			output:       "Mode: Warp\nConnectivity Checks Disabled: false\nTunnel Protocol: WireGuard",
			wantDisabled: false,
			wantFound:    true,
		},
		{
			name:         "json format disabled",
			output:       "{\n  \"mode\": \"warp\",\n  \"connectivity_checks_disabled\": true\n}",
			wantDisabled: true,
			wantFound:    true,
		},
		{
			name:         "json format enabled",
			output:       "{\n  \"mode\": \"warp\",\n  \"connectivity_checks_disabled\": false\n}",
			wantDisabled: false,
			wantFound:    true,
		},
		{
			name:         "colon format disabled",
			output:       "Connectivity checks: disabled",
			wantDisabled: true,
			wantFound:    true,
		},
		{
			name:         "colon format enabled",
			output:       "Connectivity checks: enabled",
			wantDisabled: false,
			wantFound:    true,
		},
		{
			name:         "unrelated output",
			output:       "Error: daemon not running",
			wantDisabled: false,
			wantFound:    false,
		},
		{
			name:         "empty output",
			output:       "",
			wantDisabled: false,
			wantFound:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDisabled, gotFound := isConnectivityCheckDisabled(tt.output)
			if gotDisabled != tt.wantDisabled || gotFound != tt.wantFound {
				t.Errorf("isConnectivityCheckDisabled() = (disabled: %v, found: %v), want (disabled: %v, found: %v)",
					gotDisabled, gotFound, tt.wantDisabled, tt.wantFound)
			}
		})
	}
}

func TestEnsureWarpConnectivityCheckDisabled(t *testing.T) {
	origRunner := cmdRunner
	defer func() { cmdRunner = origRunner }()

	t.Run("already disabled skips command execution", func(t *testing.T) {
		var executedCommands []string
		cmdRunner = func(name string, args ...string) ([]byte, error) {
			cmdStr := name + " " + strings.Join(args, " ")
			executedCommands = append(executedCommands, cmdStr)
			if cmdStr == "warp-cli settings list" {
				return []byte("Connectivity Checks Disabled: true"), nil
			}
			return []byte("Success"), nil
		}

		ok := ensureWarpConnectivityCheckDisabled()
		if !ok {
			t.Errorf("expected ensureWarpConnectivityCheckDisabled to return true")
		}
		if len(executedCommands) != 1 || executedCommands[0] != "warp-cli settings list" {
			t.Errorf("expected only 'warp-cli settings list' to be called, got %v", executedCommands)
		}
	})

	t.Run("enabled triggers disable command", func(t *testing.T) {
		var executedCommands []string
		cmdRunner = func(name string, args ...string) ([]byte, error) {
			cmdStr := name + " " + strings.Join(args, " ")
			executedCommands = append(executedCommands, cmdStr)
			if cmdStr == "warp-cli settings list" {
				return []byte("Connectivity Checks Disabled: false"), nil
			}
			return []byte("Success"), nil
		}

		ok := ensureWarpConnectivityCheckDisabled()
		if !ok {
			t.Errorf("expected ensureWarpConnectivityCheckDisabled to return true")
		}
		if len(executedCommands) != 2 {
			t.Fatalf("expected 2 commands, got %v", executedCommands)
		}
		if executedCommands[1] != "warp-cli debug connectivity-check disable" {
			t.Errorf("expected disable command, got %s", executedCommands[1])
		}
	})

	t.Run("disable failure logs warning and returns false", func(t *testing.T) {
		cmdRunner = func(name string, args ...string) ([]byte, error) {
			cmdStr := name + " " + strings.Join(args, " ")
			if cmdStr == "warp-cli settings list" {
				return []byte("Connectivity Checks Disabled: false"), nil
			}
			return []byte("Failed to connect to daemon"), fmt.Errorf("exit status 1")
		}

		ok := ensureWarpConnectivityCheckDisabled()
		if ok {
			t.Errorf("expected ensureWarpConnectivityCheckDisabled to return false when disable command fails")
		}
	})
}
