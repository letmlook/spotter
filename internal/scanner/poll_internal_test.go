package scanner

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/registry"
)

func TestPollFailures_BumpReset(t *testing.T) {
	pf := newPollFailures(3)
	if got := pf.bump("a"); got != 1 {
		t.Fatalf("bump 1 → %d", got)
	}
	if got := pf.bump("a"); got != 2 {
		t.Fatalf("bump 2 → %d", got)
	}
	pf.reset("a")
	if got := pf.bump("a"); got != 1 {
		t.Fatalf("post-reset bump → %d, want 1", got)
	}
}

func TestScanner_WatchRegistry_ResetFailOnRemove(t *testing.T) {
	dir := t.TempDir()
	r, err := registry.Open(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	events := make(chan Event, 4)
	_ = events

	// Add d1 first — without this, registry.Remove("d1") is a no-op
	// and the watcher never receives a Remove event.
	if err := r.Add(registry.Entry{DeviceID: "d1", IP: "10.0.0.1", Port: 9999}); err != nil {
		t.Fatal(err)
	}

	s := New(r,
		func(o *Options) { o.Logger = logger },
	)

	if s.failTrack.bump("d1") != 1 ||
		s.failTrack.bump("d1") != 2 ||
		s.failTrack.bump("d1") != 3 {
		t.Fatal("setup: fail count not at 3")
	}

	if err := r.Remove("d1"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, tracked := s.failTrack.get("d1"); !tracked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("watcher debug log:\n%s", logBuf.String())
	t.Fatal("watcher did not reset fail count after Remove")
}

func TestScanner_WatchRegistry_NoOpOnAdd(t *testing.T) {
	dir := t.TempDir()
	r, _ := registry.Open(filepath.Join(dir, "devices.json"))
	defer r.Close()
	s := New(r)
	if err := r.Add(registry.Entry{DeviceID: "a", IP: "1.2.3.4"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := s.failTrack.bump("a"); got != 1 {
		t.Fatalf("Add must not touch fail count: bump → %d, want 1", got)
	}
}

func TestScanner_WatchRegistry_ExitsOnClose(t *testing.T) {
	dir := t.TempDir()
	r, _ := registry.Open(filepath.Join(dir, "devices.json"))
	ch := r.Subscribe()
	_ = New(r)
	r.Close()
	select {
	case _, ok := <-ch:
		if ok {
			// Expected: channel closed, receive returns zero value + false.
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber channel was not closed on registry.Close")
	}
}
