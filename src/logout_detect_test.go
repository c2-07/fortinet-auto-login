package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFastCheckAuthState(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	t.Run("204 returns logged in with single request", func(t *testing.T) {
		var hits int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		t.Setenv("FORTINET_CHECK_URL", srv.URL)
		state, portal := fastCheckAuthState(srv.Client())
		if state != authStateLoggedIn || portal != "" {
			t.Fatalf("got (%v, %q), want (authStateLoggedIn, \"\")", state, portal)
		}
		if atomic.LoadInt32(&hits) != 1 {
			t.Errorf("expected exactly 1 probe request, got %d", hits)
		}
	})

	t.Run("captive portal redirect returns logged out with portal URL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body><script>window.location="https://192.168.55.253:1003/fgtauth?abcdef0123456789";</script></body></html>`))
		}))
		defer srv.Close()

		t.Setenv("FORTINET_CHECK_URL", srv.URL)
		state, portal := fastCheckAuthState(srv.Client())
		if state != authStateLoggedOut {
			t.Fatalf("got state %v, want authStateLoggedOut", state)
		}
		want := "https://192.168.55.253:1003/fgtauth?abcdef0123456789"
		if portal != want {
			t.Errorf("portal = %q, want %q", portal, want)
		}
	})
}

func TestAdaptiveSchedule(t *testing.T) {
	sched := newAdaptiveSchedule(30*time.Second, 5*time.Minute)
	if sched.current != 30*time.Second {
		t.Fatalf("initial current = %v, want 30s", sched.current)
	}

	// Exponential backoff capped at 5m
	sched.onHealthy() // 1m
	sched.onHealthy() // 2m
	sched.onHealthy() // 4m
	sched.onHealthy() // 5m (capped)
	if sched.current != 5*time.Minute {
		t.Errorf("capped current = %v, want 5m", sched.current)
	}

	// Clamp to keepalive deadline when session expires in 3 minutes (margin 2m -> refresh in 1m)
	now := time.Now()
	s := session{
		Countdown:   300,
		RefreshedAt: now.Add(-120 * time.Second).Unix(), // 180s left; refresh at 120s left -> 60s from now
	}
	delay := sched.nextDelay(s, now, 2*time.Minute)
	if delay < 55*time.Second || delay > 65*time.Second {
		t.Errorf("nextDelay = %v, expected ~60s clamped to keepalive deadline", delay)
	}

	// Event resets backoff immediately
	sched.onEvent()
	if sched.current != 30*time.Second {
		t.Errorf("after onEvent current = %v, want 30s", sched.current)
	}
}

func TestAdaptiveDaemon_ReactsImmediatelyToNetworkChange(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	var probeHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&probeHits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("FORTINET_CHECK_URL", srv.URL)

	origWatch := localWatchInterval
	origFP := networkFingerprintFunc
	defer func() {
		localWatchInterval = origWatch
		networkFingerprintFunc = origFP
	}()

	localWatchInterval = 20 * time.Millisecond
	var fpVal atomic.Value
	fpVal.Store("en0=192.168.1.10/24")
	networkFingerprintFunc = func() string {
		return fpVal.Load().(string)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		// Long base interval (10s) so only the initial check + network-change trigger should fire
		daemon(ctx, 10*time.Second)
		close(done)
	}()

	// Wait for initial probe
	time.Sleep(60 * time.Millisecond)
	if got := atomic.LoadInt32(&probeHits); got != 1 {
		cancel()
		<-done
		t.Fatalf("expected 1 initial probe, got %d", got)
	}

	// Simulate DHCP / Wi-Fi change -> daemon must probe immediately instead of waiting 10s
	fpVal.Store("en0=192.168.13.228/24")
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	if got := atomic.LoadInt32(&probeHits); got < 2 {
		t.Errorf("expected immediate probe on network change, got only %d total probes", got)
	}
}
