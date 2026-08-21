package scanner_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

func TestPollUpdatesOnline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/info" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(protocol.DeviceInfo{
			DeviceID: "d1",
			Basic:    protocol.BasicInfo{Hostname: "x"},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	reg, _ := registry.Open(filepath.Join(dir, "devices.json"))
	_ = reg.Add(registry.Entry{
		DeviceID: "d1",
		IP:       srv.Listener.Addr().(*net.TCPAddr).IP.String(),
		Port:     srv.Listener.Addr().(*net.TCPAddr).Port,
	})

	var events []string
	sc := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		events = append(events, e.Tag())
	}))

	// Run a single poll synchronously.
	if err := sc.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	entry, _ := reg.Get("d1")
	if !entry.Online {
		t.Errorf("expected online, got %+v", entry)
	}
	if entry.LastInfo == nil || entry.LastInfo.Basic.Hostname != "x" {
		t.Errorf("expected info hostname x, got %+v", entry.LastInfo)
	}
	// Event "info-updated" should have been emitted.
	found := false
	for _, e := range events {
		if e == "info-updated" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info-updated event, got %v", events)
	}
}

func TestPollOfflineAfter3Failures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	dir := t.TempDir()
	reg, _ := registry.Open(filepath.Join(dir, "devices.json"))
	_ = reg.Add(registry.Entry{
		DeviceID: "d1",
		IP:       srv.Listener.Addr().(*net.TCPAddr).IP.String(),
		Port:     srv.Listener.Addr().(*net.TCPAddr).Port,
		Online:   true,
	})

	var events []string
	sc := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		events = append(events, e.Tag())
	}))

	for i := 0; i < 3; i++ {
		if err := sc.PollOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	entry, _ := reg.Get("d1")
	if entry.Online {
		t.Error("expected offline after 3 failures")
	}
	found := false
	for _, e := range events {
		if e == "offline" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected offline event, got %v", events)
	}
}
