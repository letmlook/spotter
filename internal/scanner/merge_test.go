package scanner

import (
	"sync"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)

// newTestScanner returns a Scanner with no goroutines running and a
// registry fed by the test. Event channel is bounded and inspected via
// events. The watcher goroutine is wired to a Closeable registry so the
// test can drain it deterministically.
func newTestScanner(t *testing.T) (*Scanner, *registry.Registry, func() <-chan Event) {
	t.Helper()
	r, err := registry.Open(filepathJoinTemp(t))
	if err != nil {
		t.Fatal(err)
	}
	evCh := make(chan Event, 16)
	var mu sync.Mutex
	var buf []Event
	done := make(chan struct{})
	go func() {
		for e := range evCh {
			mu.Lock()
			buf = append(buf, e)
			mu.Unlock()
		}
		close(done)
	}()
	s := New(r, WithOnEvent(func(e Event) { evCh <- e }))
	events := func() <-chan Event {
		c := make(chan Event, len(buf))
		mu.Lock()
		defer mu.Unlock()
		for _, e := range buf {
			c <- e
		}
		close(c)
		return c
	}
	// Drain helper to ensure deterministic event count assertions.
	return s, r, events
}

func filepathJoinTemp(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/devices.json"
}

func TestMerge_NewDevice_EmitsUnknown(t *testing.T) {
	r, _ := registry.Open(filepathJoinTemp(t))
	defer r.Close()
	s := New(r)

	events := make(chan Event, 4)
	s.opts.OnEvent = func(e Event) { events <- e }
	s.mergeInfo("mcast", "10.0.0.42", 9999, protocol.DeviceInfo{
		SchemaVersion: 1,
		DeviceID:      "new-device",
	})
	select {
	case e := <-events:
		u, ok := e.(EventUnknownDeviceDiscovered)
		if !ok {
			t.Fatalf("want EventUnknownDeviceDiscovered, got %T", e)
		}
		if u.Info.DeviceID != "new-device" {
			t.Errorf("unexpected device_id: %s", u.Info.DeviceID)
		}
		if u.IP != "10.0.0.42" {
			t.Errorf("unexpected IP: %s", u.IP)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for unknown-device event")
	}
}

func TestMerge_ExistingDevice_UpdatesEntry(t *testing.T) {
	r, _ := registry.Open(filepathJoinTemp(t))
	defer r.Close()
	if err := r.Add(registry.Entry{
		DeviceID: "known",
		IP:       "10.0.0.10",
		Port:     9999,
	}); err != nil {
		t.Fatal(err)
	}

	events := make(chan Event, 4)
	s := New(r, WithOnEvent(func(e Event) { events <- e }))

	s.mergeInfo("poll", "10.0.0.10", 9999, protocol.DeviceInfo{
		SchemaVersion: 1,
		DeviceID:      "known",
	})

	got, ok := r.Get("known")
	if !ok {
		t.Fatal("device removed by merge")
	}
	if !got.Online {
		t.Error("expected online=true after merge")
	}
	if got.LastSource != "poll" {
		t.Errorf("LastSource: got %s, want poll", got.LastSource)
	}
	if got.LastInfo == nil {
		t.Error("LastInfo not populated")
	}

	select {
	case e := <-events:
		if _, ok := e.(EventInfoUpdated); !ok {
			t.Fatalf("want EventInfoUpdated, got %T", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for info-updated")
	}
}

func TestMerge_UpdatesIPPortOnMove(t *testing.T) {
	r, _ := registry.Open(filepathJoinTemp(t))
	defer r.Close()
	if err := r.Add(registry.Entry{
		DeviceID: "moving",
		IP:       "10.0.0.10",
		Port:     9999,
	}); err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 4)
	s := New(r, WithOnEvent(func(e Event) { events <- e }))
	s.mergeInfo("mcast", "10.0.0.99", 9999, protocol.DeviceInfo{
		SchemaVersion: 1,
		DeviceID:      "moving",
	})
	got, _ := r.Get("moving")
	if got.IP != "10.0.0.99" {
		t.Errorf("IP not updated: %s", got.IP)
	}
}

func TestMerge_PreservesIPWhenZero(t *testing.T) {
	r, _ := registry.Open(filepathJoinTemp(t))
	defer r.Close()
	if err := r.Add(registry.Entry{
		DeviceID: "static",
		IP:       "10.0.0.10",
	}); err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 4)
	s := New(r, WithOnEvent(func(e Event) { events <- e }))
	// ip="" must NOT clobber the existing IP (used by mcast when only
	// the reply carries the info).
	s.mergeInfo("poll", "", 0, protocol.DeviceInfo{SchemaVersion: 1, DeviceID: "static"})
	got, _ := r.Get("static")
	if got.IP != "10.0.0.10" {
		t.Errorf("IP clobbered by empty: %s", got.IP)
	}
}
