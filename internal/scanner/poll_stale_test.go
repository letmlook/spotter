package scanner

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/registry"
)

// TestPollOne_StaleHeaderSetsInfoStale verifies that when the
// agent serves its cached snapshot via the X-Spotter-Stale
// response header, the DeviceInfo the scanner writes to the
// registry carries stale=true. This is the only way the GUI
// can tell a fresh collect from a cached copy after a
// partial agent failure — without this propagation the UI
// would render fresh-looking timestamps on minutes-old data.
func TestPollOne_StaleHeaderSetsInfoStale(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		// First call: fresh. Second call: stale (agent
		// failed to collect, served cached snapshot).
		if hits.Load() == 1 {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Spotter-Stale", "true")
		}
		_, _ = w.Write([]byte(`{
			"schema_version": 2,
			"device_id": "stale-host",
			"basic": {"hostname": "stale-host"},
			"network": {"primary_ip": "10.0.0.50"}
		}`))
	}))
	defer srv.Close()

	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	defer reg.Close()
	s := New(reg, WithDevicePort(0)) // device port unused; URL parsed from server
	// Seed the registry so pollOne has an entry to refresh.
	_ = reg.Add(registry.Entry{
		DeviceID: "stale-host",
		IP: "127.0.0.1", // 127.0.0.1:PORT
		Port: srv.Listener.Addr().(*net.TCPAddr).Port,
	})

	// Helper: wait for N polls.
	waitFor := func(n int64, timeout time.Duration) {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) && hits.Load() < n {
			time.Sleep(20 * time.Millisecond)
		}
	}

	// First poll — agent returns fresh.
	s.PollOnce(context.Background())
	waitFor(1, time.Second)

	// Second poll — agent returns stale.
	s.PollOnce(context.Background())
	waitFor(2, time.Second)

	entry, ok := reg.Get("stale-host")
	if !ok {
		t.Fatal("registry should have stale-host entry after polls")
	}
	if entry.LastInfo == nil {
		t.Fatal("LastInfo nil after stale poll")
	}
	if !entry.LastInfo.Stale {
		t.Errorf("LastInfo.Stale = false after X-Spotter-Stale: true, want true")
	}
}

// TestPollOne_NoStaleHeaderStaysFresh is the inverse: a 200
// without the X-Spotter-Stale header must keep stale=false.
// Pins the wire contract so a future agent that always sets
// the header (even on success) doesn't silently mark every
// poll as stale.
func TestPollOne_NoStaleHeaderStaysFresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"schema_version": 2,
			"device_id": "fresh-host",
			"basic": {"hostname": "fresh-host"},
			"network": {"primary_ip": "10.0.0.51"}
		}`))
	}))
	defer srv.Close()

	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	defer reg.Close()
	s := New(reg, WithDevicePort(0))
	_ = reg.Add(registry.Entry{
		DeviceID: "fresh-host",
		IP: "127.0.0.1",
		Port: srv.Listener.Addr().(*net.TCPAddr).Port,
	})
	s.PollOnce(context.Background())

	entry, _ := reg.Get("fresh-host")
	if entry.LastInfo == nil {
		t.Fatal("LastInfo nil")
	}
	if entry.LastInfo.Stale {
		t.Errorf("LastInfo.Stale = true on a 200 without X-Spotter-Stale, want false")
	}
}
