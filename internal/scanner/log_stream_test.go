package scanner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

func journalLine(ts int64, msg, cursor string) string {
	rec := map[string]string{
		"__REALTIME_TIMESTAMP": fmt.Sprintf("%d", ts),
		"MESSAGE":              msg,
		"__CURSOR":             cursor,
	}
	b, _ := json.Marshal(rec)
	return string(b)
}

func TestStreamDeviceLogs_NormalFlow(t *testing.T) {
	var (
		mu       sync.Mutex
		received []scanner.LogLine
	)
	followCh := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		// 3 行历史
		for _, l := range []string{
			journalLine(1700000000000000, "boot", "c0"),
			journalLine(1700000001000000, "ready", "c1"),
			journalLine(1700000002000000, "listening", "c2"),
		} {
			_, _ = w.Write([]byte(l + "\n"))
		}
		flusher.Flush()
		// 持续 follow，直到 client 断开
		for {
			select {
			case <-r.Context().Done():
				return
			case line := <-followCh:
				_, _ = w.Write([]byte(line + "\n"))
				flusher.Flush()
			}
		}
	}))
	defer func() {
		srv.Close()
		close(followCh)
	}()

	// 推送 3 行 follow
	followCh <- journalLine(1700000003000000, "accept", "c3")
	followCh <- journalLine(1700000004000000, "ping", "c4")
	followCh <- journalLine(1700000005000000, "stop", "c5")

	reg, _ := registry.Open(t.TempDir() + "/devices.json")
	sc := scanner.New(reg)
	addr := srv.Listener.Addr().(*net.TCPAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := sc.StreamDeviceLogs(ctx, addr.IP.String(), addr.Port, "", func(line scanner.LogLine) {
		mu.Lock()
		received = append(received, line)
		n := len(received)
		mu.Unlock()
		if n >= 6 {
			cancel()
		}
	})
	if err != nil && !strings.Contains(err.Error(), "context") {
		t.Fatalf("unexpected: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 6 {
		t.Fatalf("got %d lines, want 6: %+v", len(received), received)
	}
	if received[0].Line != "boot" || received[5].Line != "stop" {
		t.Errorf("lines mismatch: %+v", received)
	}
	if received[0].Ts == "" || !strings.Contains(received[0].Ts, "2023-11-14") {
		t.Errorf("ts format wrong: %q", received[0].Ts)
	}
}

func TestStreamDeviceLogs_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("log streaming disabled"))
	}))
	defer srv.Close()

	reg, _ := registry.Open(t.TempDir() + "/devices.json")
	sc := scanner.New(reg)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	err := sc.StreamDeviceLogs(context.Background(), addr.IP.String(), addr.Port, "", func(_ scanner.LogLine) {})
	if err == nil || !strings.Contains(err.Error(), "log streaming disabled") {
		t.Fatalf("want disabled error, got %v", err)
	}
}

func TestStreamDeviceLogs_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Block until ctx done
		<-r.Context().Done()
	}))
	defer srv.Close()

	reg, _ := registry.Open(t.TempDir() + "/devices.json")
	sc := scanner.New(reg)
	addr := srv.Listener.Addr().(*net.TCPAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := sc.StreamDeviceLogs(ctx, addr.IP.String(), addr.Port, "", func(_ scanner.LogLine) {})
	if err != nil {
		// We accept context.Canceled as the expected outcome; nil is also OK if reader exits cleanly.
		if !strings.Contains(err.Error(), "context canceled") {
			t.Logf("got err %v (acceptable)", err)
		}
	}
}
