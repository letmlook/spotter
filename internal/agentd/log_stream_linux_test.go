//go:build linux

package agentd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHandleLogs_DisabledReturns403(t *testing.T) {
	a, err := New(Config{
		DeviceID:        "x",
		ListenAddr:      "127.0.0.1:0",
		AgentVersion:    "0.1.0",
		EnableLogStream: false,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
	if !strings.Contains(string(body), "log streaming disabled") {
		t.Errorf("body = %q, want 'log streaming disabled'", body)
	}
}

func TestHandleLogs_NonGETReturns405(t *testing.T) {
	a, err := New(Config{
		DeviceID:        "x",
		ListenAddr:      "127.0.0.1:0",
		AgentVersion:    "0.1.0",
		EnableLogStream: true,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/logs", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestHandleLogs_JournalctlMissingReportsError(t *testing.T) {
	orig := startJournalctl
	startJournalctl = func(_ context.Context, _ JournalctlOpts) (io.ReadCloser, func(), error) {
		return nil, nil, errors.New("not found")
	}
	defer func() { startJournalctl = orig }()

	a, err := New(Config{
		DeviceID:        "x",
		ListenAddr:      "127.0.0.1:0",
		AgentVersion:    "0.1.0",
		EnableLogStream: true,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (header already flushed)", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "journalctl not available") {
		t.Errorf("body = %q, want error marker", body)
	}
}

func TestHandleLogs_NormalStream(t *testing.T) {
	// 模拟 journalctl：3 行 NDJSON，关闭后 reader 立刻 EOF。
	payload := `{"__REALTIME_TIMESTAMP":"1700000000000000","MESSAGE":"hello-1","__CURSOR":"c1"}
{"__REALTIME_TIMESTAMP":"1700000001000000","MESSAGE":"hello-2","__CURSOR":"c2"}
{"__REALTIME_TIMESTAMP":"1700000002000000","MESSAGE":"hello-3","__CURSOR":"c3"}
`
	rc, wr := io.Pipe()
	orig := startJournalctl
	var killed atomic.Bool
	var gotOpts JournalctlOpts
	startJournalctl = func(_ context.Context, opts JournalctlOpts) (io.ReadCloser, func(), error) {
		gotOpts = opts
		go func() {
			_, _ = wr.Write([]byte(payload))
			_ = wr.Close()
		}()
		return rc, func() { killed.Store(true); _ = rc.Close() }, nil
	}
	defer func() { startJournalctl = orig }()

	a, err := New(Config{
		DeviceID:        "x",
		ListenAddr:      "127.0.0.1:0",
		AgentVersion:    "0.1.0",
		EnableLogStream: true,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/logs?tail=100")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != payload {
		t.Errorf("body mismatch:\n got %q\nwant %q", body, payload)
	}
	// kill 回调由 handler 的 defer kill() 调用。
	if !killed.Load() {
		t.Errorf("kill callback was not invoked by handler defer")
	}
	// Default-only call (?tail=100, no ?unit=) must end up with
	// the configured unit alone in opts.Units.
	if len(gotOpts.Units) != 1 || gotOpts.Units[0] != "spotterd.service" {
		t.Errorf("default units = %v, want [spotterd.service]", gotOpts.Units)
	}
	if gotOpts.Tail != 100 {
		t.Errorf("tail = %d, want 100", gotOpts.Tail)
	}
}

// TestHandleLogs_MultiUnitAndFilters — drive the handler with
// a multi-unit ?unit= and exercise every new query param. The
// fake journalctl captures the opts it was called with and we
// assert the layered unit list (default + override), the
// grep / since / priority passthroughs, and the tail cap.
func TestHandleLogs_MultiUnitAndFilters(t *testing.T) {
	var gotOpts JournalctlOpts
	orig := startJournalctl
	startJournalctl = func(_ context.Context, opts JournalctlOpts) (io.ReadCloser, func(), error) {
		gotOpts = opts
		return io.NopCloser(strings.NewReader("")), func() {}, nil
	}
	defer func() { startJournalctl = orig }()

	a, err := New(Config{
		DeviceID:        "x",
		ListenAddr:      "127.0.0.1:0",
		AgentVersion:    "0.1.0",
		EnableLogStream: true,
		LogUnit:         "spotterd.service",
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/logs?unit=nginx.service,caddy.service&grep=ERROR&since=5min%20ago&priority=err&tail=50")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Layered units: default (spotterd.service) in front, then
	// the two overrides from ?unit=.
	want := []string{"spotterd.service", "nginx.service", "caddy.service"}
	if len(gotOpts.Units) != len(want) {
		t.Fatalf("units = %v, want %v", gotOpts.Units, want)
	}
	for i, u := range want {
		if gotOpts.Units[i] != u {
			t.Errorf("units[%d] = %q, want %q", i, gotOpts.Units[i], u)
		}
	}
	if gotOpts.Grep != "ERROR" {
		t.Errorf("grep = %q, want ERROR", gotOpts.Grep)
	}
	if gotOpts.Since != "5min ago" {
		t.Errorf("since = %q, want 5min ago", gotOpts.Since)
	}
	if gotOpts.Priority != "err" {
		t.Errorf("priority = %q, want err", gotOpts.Priority)
	}
	if gotOpts.Tail != 50 {
		t.Errorf("tail = %d, want 50", gotOpts.Tail)
	}
}

// TestSplitUnits covers the unit-list parser in isolation.
func TestSplitUnits(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a , b ,c  ", []string{"a", "b", "c"}},
		{",a,,b,", []string{"a", "b"}},
		{"only-whitespace ,  ", []string{"only-whitespace"}},
	}
	for _, c := range cases {
		got := splitUnits(c.in)
		if !equalSlice(got, c.want) {
			t.Errorf("splitUnits(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
