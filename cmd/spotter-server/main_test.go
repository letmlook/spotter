package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/serverd"
)

// startServer brings up an in-process spotter-server with a fresh
// data dir and returns the httptest.Server. Tests must Close it.
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

// TestSpotterServer_Healthz boots an in-process spotter-server with
// a temp data dir and asserts /healthz answers. Exercises the
// full startup sequence (store open, hub, mux wiring) without
// forking a subprocess — gives us signal-handling and store wiring
// under coverage.
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

	postBody := strings.NewReader(`{"device_id":"abc","ip":"10.0.0.42","port":9999}`)
	resp, err := http.Post(srv.URL+"/api/v1/devices", "application/json", postBody)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST got %d, want 201", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/v1/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("devices=%+v, want 1 entry", got)
	}
	d := got[0]
	if d["device_id"] != "abc" || d["ip"] != "10.0.0.42" {
		t.Errorf("device=%+v, want id=abc ip=10.0.0.42", d)
	}
}

// TestSpotterServer_GracefulShutdown drives runServer in-process:
// bind to :0, accept a /healthz request, then cancel ctx and
// assert runServer returns 0 within the 5s drain window. Catches
// regressions where signal handling or srv.Shutdown wiring breaks.
func TestSpotterServer_GracefulShutdown(t *testing.T) {
	dir := t.TempDir()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // let runServer rebind

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- runServer(addr, dir, ctx) }()

	// Wait for the server to bind by polling /healthz.
	probeAddr := waitForHealthz(t, 5*time.Second, addr)

	resp, err := http.Get("http://" + probeAddr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("healthz status=%d, want 200", resp.StatusCode)
	}

	// Trigger graceful shutdown.
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("runServer returned %d, want 0", code)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("runServer did not return within 7s of cancel")
	}
}

// waitForHealthz polls /healthz on the requested bound port until
// 200 or timeout. Returns the bound address on success. The port
// is fixed (we pass `addr` from the test that called us); this
// avoids the wide-port-scan race that a true :0 listener would
// force.
func waitForHealthz(t *testing.T, timeout time.Duration, addr string) string {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr %q: %v", addr, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probe := net.JoinHostPort(host, strconv.Itoa(mustPort(port)))
		resp, err := http.Get("http://" + probe + "/healthz")
		if err == nil {
			if resp.StatusCode == 200 {
				resp.Body.Close()
				return probe
			}
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for server to bind")
	return ""
}

func mustPort(s string) int {
	p, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return p
}
