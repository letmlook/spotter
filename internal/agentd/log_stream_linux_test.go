//go:build linux

package agentd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	startJournalctl = func(_ context.Context, _ string, _ int) (io.ReadCloser, func(), error) {
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
	startJournalctl = func(_ context.Context, unit string, tail int) (io.ReadCloser, func(), error) {
		if unit != "spotterd.service" {
			return nil, nil, errors.New("unexpected unit: " + unit)
		}
		if tail != 100 {
			return nil, nil, errors.New("unexpected tail: " + strconv.Itoa(tail))
		}
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
}
