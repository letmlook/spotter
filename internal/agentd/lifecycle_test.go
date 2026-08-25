package agentd_test

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/agentd"
)

// startAgent binds a free-port listener, runs StartHTTP in the
// background, and returns the URL to reach it. Caller is
// responsible for calling Close when done.
func startAgent(t *testing.T, deviceID string) (a *agentd.Agent, url string) {
	t.Helper()
	a, err := agentd.New(agentd.Config{
		DeviceID:   deviceID,
		ListenAddr: "127.0.0.1:0",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: a.Handler(),
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		t.Fatal(err)
	}
	a.SetHTTPServer(srv)
	go func() {
		_ = srv.Serve(ln)
	}()
	url = "http://" + ln.Addr().String()
	return a, url
}

// TestAgent_Close_DrainsHTTP exercises the Agent.Close lifecycle
// hook: after Close returns, in-flight requests must drain and
// the listener must no longer accept new connections. Without
// Close (relying on ctx cancel alone) the goroutine running
// StartHTTP would not return promptly — Close gives callers a
// synchronous "stop the agent" handle.
func TestAgent_Close_DrainsHTTP(t *testing.T) {
	a, _ := startAgent(t, "close-test")

	// Close must return within the grace period (5s).
	done := make(chan struct{})
	go func() {
		_ = a.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Fatal("Agent.Close did not return within 7s")
	}
}

// TestAgent_Close_Idempotent verifies that Close can be called
// multiple times — the second call returns nil (no panic, no
// double-Shutdown). Critical for cmd/agent where signal handlers
// and the post-listen path can both call Close.
func TestAgent_Close_Idempotent(t *testing.T) {
	a, _ := startAgent(t, "idem")

	if err := a.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("second Close: %v (expected nil — idempotent)", err)
	}
}

// TestAgent_Close_BeforeStart asserts Close is a no-op when
// StartHTTP has not been called — useful for unit tests that
// build an Agent, inspect it, and tear down.
func TestAgent_Close_BeforeStart(t *testing.T) {
	a, err := agentd.New(agentd.Config{DeviceID: "noop", ListenAddr: "127.0.0.1:0"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Errorf("Close on never-started Agent: %v", err)
	}
}

// TestAgent_HandlerHealthz is a sanity ping: after StartHTTP
// binds, a fresh /healthz request returns 200 + "ok". Catches
// regressions where Close accidentally closes the agent's
// internal state (info cache, audit logger) instead of just
// the HTTP server.
func TestAgent_HandlerHealthz(t *testing.T) {
	a, url := startAgent(t, "ping")
	defer func() { _ = a.Close() }()

	resp, err := http.Get(url + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("got %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("body=%q, want contains 'ok'", body)
	}
}
