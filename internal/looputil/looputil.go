// Package looputil centralises the ticker+select polling loops
// that previously appeared in four files:
//
//   - internal/scanner/poll.go   — pollLoop
//   - internal/scanner/mcast.go  — mcastLoop
//   - internal/agentd/udp.go     — udpReadLoop, runHelloEmit
//
// Each had near-identical shape: `for { select { case <-ctx.Done():
// return; case <-ticker.C: fn() } }` — and the four copies had
// drifted in subtle ways (e.g. udpReadLoop logs debug on
// timeout, mcastLoop does not).
package looputil

import (
	"context"
	"time"
)

// RunTicker calls fn every d until ctx is cancelled. The first
// invocation is delayed by d; this matches the previous hand-
// rolled `for { select { case <-ticker.C: ... } }` pattern.
func RunTicker(ctx context.Context, d time.Duration, fn func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}
