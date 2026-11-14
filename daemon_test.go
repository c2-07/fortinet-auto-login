package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDaemonCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var tickCount int32
	mockTick := func(wasFortinet *bool) {
		atomic.AddInt32(&tickCount, 1)
	}

	done := make(chan struct{})
	go func() {
		daemonWithTickHandler(ctx, 10*time.Millisecond, mockTick)
		close(done)
	}()

	// Wait for at least 2 ticks
	time.Sleep(35 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited promptly upon context cancellation
		if atomic.LoadInt32(&tickCount) < 1 {
			t.Errorf("expected at least 1 tick, got %d", atomic.LoadInt32(&tickCount))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("daemon did not terminate promptly upon context cancellation")
	}
}
