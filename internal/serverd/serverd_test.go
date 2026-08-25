package serverd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*httptest.Server, *Store, *Hub) {
	t.Helper()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub()
	srv := httptest.NewServer(NewHandler(store, hub))
	t.Cleanup(srv.Close)
	return srv, store, hub
}

func TestHealthz(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestRegisterAndGet(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body, _ := json.Marshal(Device{DeviceID: "d1", IP: "10.0.0.1"})
	resp, err := http.Post(srv.URL+"/api/v1/devices", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	resp2, err := http.Get(srv.URL + "/api/v1/devices/d1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("get: %d", resp2.StatusCode)
	}
	var got Device
	if err := json.NewDecoder(resp2.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.IP != "10.0.0.1" {
		t.Errorf("IP: %q", got.IP)
	}
	if !got.Online {
		t.Error("registered device should be online")
	}
}

func TestListDevices_StableOrder(t *testing.T) {
	srv, _, _ := newTestServer(t)
	for _, id := range []string{"c", "a", "b"} {
		body, _ := json.Marshal(Device{DeviceID: id, IP: "10.0.0.1"})
		resp, err := http.Post(srv.URL+"/api/v1/devices", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	resp, err := http.Get(srv.URL + "/api/v1/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list []Device
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].DeviceID != "a" || list[1].DeviceID != "b" || list[2].DeviceID != "c" {
		t.Errorf("not key-sorted: %v", list)
	}
}

func TestHeartbeatFromUnknownDevice_404(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/api/v1/devices/unknown/heartbeat", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestHeartbeatFromRegisteredDevice_204(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body, _ := json.Marshal(Device{DeviceID: "d1", IP: "10.0.0.1"})
	resp, _ := http.Post(srv.URL+"/api/v1/devices", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	resp2, err := http.Post(srv.URL+"/api/v1/devices/d1/heartbeat", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 204 {
		t.Errorf("want 204, got %d", resp2.StatusCode)
	}
}

func TestDelete_RemovesAnd404After(t *testing.T) {
	srv, _, _ := newTestServer(t)
	body, _ := json.Marshal(Device{DeviceID: "d1", IP: "10.0.0.1"})
	resp, _ := http.Post(srv.URL+"/api/v1/devices", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/devices/d1", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 204 {
		t.Errorf("delete: want 204, got %d", resp2.StatusCode)
	}
	resp3, err := http.Get(srv.URL + "/api/v1/devices/d1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 404 {
		t.Errorf("want 404 after delete, got %d", resp3.StatusCode)
	}
}

func TestHub_PublishDeliversToSubscribers(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ch := hub.Subscribe(ctx)
	hub.Publish(Event{Type: "device-added", DeviceID: "x"})
	select {
	case ev := <-ch:
		if ev.DeviceID != "x" {
			t.Errorf("got %+v", ev)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

func TestHub_DropsSlowConsumers(t *testing.T) {
	hub := NewHub()
	// 0-buffer subscriber: any send to ch while nobody's reading
	// must drop on the floor. Drive 100 publishes with no
	// receiver; if the implementation ever blocks instead of
	// dropping, this test will hang and -timeout will surface it.
	ch := make(chan Event) // unbuffered
	hub.mu.Lock()
	hub.subs[ch] = struct{}{}
	hub.mu.Unlock()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			hub.Publish(Event{Type: "flood", DeviceID: "d"})
		}
		close(done)
	}()
	select {
	case <-done:
		// ok — publishes all returned without blocking
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on slow consumer (must drop)")
	}
	hub.mu.Lock()
	delete(hub.subs, ch)
	hub.mu.Unlock()
}

func TestPersistence_ReloadsAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.json")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Upsert(Device{DeviceID: "d1", IP: "10.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s2.Get("d1")
	if err != nil {
		t.Fatal(err)
	}
	if d.IP != "10.0.0.1" {
		t.Errorf("reloaded: %+v", d)
	}
}

// TestHub_SubCount exercises the subscriber-count metric
// used by /api/v1/metrics-style endpoints and integration
// tests verifying subscriber lifecycle. Without this
// observable an operator has no way to detect a stuck or
// orphaned WebSocket fan-out.
func TestHub_SubCount(t *testing.T) {
	hub := NewHub()
	if got := hub.SubCount(); got != 0 {
		t.Errorf("SubCount = %d on a fresh Hub, want 0", got)
	}
	ctx1, cancel1 := context.WithCancel(context.Background())
	hub.Subscribe(ctx1)
	if got := hub.SubCount(); got != 1 {
		t.Errorf("SubCount after 1 subscribe = %d, want 1", got)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	hub.Subscribe(ctx2)
	if got := hub.SubCount(); got != 2 {
		t.Errorf("SubCount after 2 subscribes = %d, want 2", got)
	}
	cancel1()
	// Subcount drops asynchronously via the goroutine inside
	// Subscribe that deletes on ctx.Done.
	waitForCount := func(want int, timeout time.Duration) {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) && hub.SubCount() != want {
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitForCount(1, time.Second)
	if got := hub.SubCount(); got != 1 {
		t.Errorf("SubCount after cancel1 = %d, want 1", got)
	}
	cancel2()
	waitForCount(0, time.Second)
	if got := hub.SubCount(); got != 0 {
		t.Errorf("SubCount after cancel2 = %d, want 0", got)
	}
}
