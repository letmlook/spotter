package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spotter/spotter/internal/serverd"
)

// TestSpotterServer_Healthz boots an in-process spotter-server
// with a temp data dir and asserts /healthz answers. Exercises
// the full startup sequence (store open, hub, mux wiring)
// without forking a subprocess — gives us signal-handling and
// store wiring under coverage.
func TestSpotterServer_Healthz(t *testing.T) {
	srv := startServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz got %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("healthz body=%q, want \"ok\"", body)
	}
}

// TestSpotterServer_ListDevicesEmpty verifies a freshly-booted
// server returns [] for /api/v1/devices (no devices registered).
// Without an entry in the registry we can't assert device
// contents, but the empty-list shape is a stable contract the
// GUI keys off.
func TestSpotterServer_ListDevicesEmpty(t *testing.T) {
	srv := startServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("empty list body=%q, want \"[]\"", body)
	}
}

// TestSpotterServer_RegisterAndGet — POST /api/v1/devices, then
// GET it back. Pins the JSON shape across the wire.
func TestSpotterServer_RegisterAndGet(t *testing.T) {
	srv := startServer(t)
	defer srv.Close()

	postBody := strings.NewReader(`{"device_id":"abc","ip":"10.0.0.42"}`)
	resp, err := http.Post(srv.URL+"/api/v1/devices", "application/json", postBody)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST got %d, want 201", resp.StatusCode)
	}

	getResp, err := http.Get(srv.URL + "/api/v1/devices/abc")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Errorf("GET got %d, want 200", getResp.StatusCode)
	}
	var d serverd.Device
	if err := json.NewDecoder(getResp.Body).Decode(&d); err != nil {
		t.Fatal(err)
	}
	if d.DeviceID != "abc" || d.IP != "10.0.0.42" {
		t.Errorf("device=%+v, want id=abc ip=10.0.0.42", d)
	}
}

// startServer builds and starts an in-process spotter-server on
// a random localhost port. Returns the httptest.Server; the
// caller must Close it.
func startServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	store, err := serverd.Open(filepath.Join(dir, "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	hub := serverd.NewHub()
	srv := httptest.NewServer(serverd.NewHandler(store, hub))
	t.Cleanup(func() { srv.Close() })
	return srv
}
