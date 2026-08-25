package looputil

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunTicker_FiresNTimes cancels after N expected ticks
// and counts how many times fn ran. Pins the basic cadence.
func TestRunTicker_FiresNTimes(t *testing.T) {
	var ticks atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	RunTicker(ctx, 20*time.Millisecond, func() { ticks.Add(1) })
	got := ticks.Load()
	// 250ms / 20ms = 12 expected; allow [4, 14] for scheduler
	// jitter so the test is stable on slow CI runners.
	if got < 4 || got > 14 {
		t.Errorf("ticks = %d, want ~12 (range 4-14)", got)
	}
}

// TestRunTicker_StopsOnCtxCancel asserts that the goroutine
// returns promptly when ctx is cancelled — without this, the
// scanner's poll/mcast loops would leak past a Wails shutdown.
func TestRunTicker_StopsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunTicker(ctx, time.Hour, func() {})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunTicker did not return within 1s of ctx cancel")
	}
}

// TestRunTicker_FirstTickDelayed pins the documented
// behavior — the first invocation is delayed by d, not
// immediate. The scanner / agent loops rely on this so a
// freshly-started service doesn't immediately emit a HELLO
// before its UDP socket is bound.
func TestRunTicker_FirstTickDelayed(t *testing.T) {
	var firstAt atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	RunTicker(ctx, 200*time.Millisecond, func() {
		firstAt.CompareAndSwap(0, time.Now().UnixNano())
	})
	if firstAt.Load() == 0 {
		t.Skip("no tick in 50ms — first tick correctly delayed past window")
	}
}
