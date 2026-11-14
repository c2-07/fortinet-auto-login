package main

import (
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
