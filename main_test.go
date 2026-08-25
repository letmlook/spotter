package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/clientconfig"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

// fakeEmitter counts Emit calls and records event names.
type fakeEmitter struct {
	mu       sync.Mutex
	events   []string
	payloads []any
}

func (f *fakeEmitter) Emit(_ context.Context, name string, data ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, name)
	f.payloads = append(f.payloads, data)
}

// count 返回名为 prefix 开头的 event 数量。
func (f *fakeEmitter) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.events {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func newTestApp(t *testing.T, reg *registry.Registry, em Emitter) *App {
	t.Helper()
	settings, err := clientconfig.Open(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(reg, settings, slog.New(slog.NewTextHandler(io.Discard, nil)), em)
	// 替换 streamFn 为同步 fake
	a.logStreamApp.streamFn = func(ctx context.Context, ip string, port int, onLine func(scanner.LogLine)) error {
		// 默认 fake：emit 3 行后返回 nil
		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			onLine(scanner.LogLine{Ts: "2026-01-01T00:00:00Z", Line: "line", Cursor: "c"})
		}
		return nil
	}
	return a
}

func TestStartLogStream_NotRegistered(t *testing.T) {
	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	em := &fakeEmitter{}
	a := newTestApp(t, reg, em)
	err := a.StartLogStream("unknown")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found error, got %v", err)
	}
}

func TestStartLogStream_Offline(t *testing.T) {
	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	_ = reg.Add(registry.Entry{
		DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
		Online: false, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
	})
	em := &fakeEmitter{}
	a := newTestApp(t, reg, em)
	err := a.StartLogStream("d1")
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("want offline error, got %v", err)
	}
}

func TestStartLogStream_OnlineEmitsLines(t *testing.T) {
	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	_ = reg.Add(registry.Entry{
		DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
		Online: true, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
	})
	em := &fakeEmitter{}
	a := newTestApp(t, reg, em)

	if err := a.StartLogStream("d1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 等待 goroutine 完成 fake streamFn（3 行后返回 → defer emit end）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if em.count("device-log-end:") >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := em.count("device-log:d1"); got != 3 {
		t.Errorf("device-log:d1 count = %d, want 3", got)
	}
	if got := em.count("device-log-end:d1"); got != 1 {
		t.Errorf("device-log-end:d1 count = %d, want 1", got)
	}
	// Stop 后再 Start 必须幂等：当前 map 已清空（fake streamFn 已返回，defer 删了 entry），可以再 Start
}

func TestStartLogStream_Idempotent(t *testing.T) {
	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	_ = reg.Add(registry.Entry{
		DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
		Online: true, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
	})
	em := &fakeEmitter{}
	a := newTestApp(t, reg, em)
	// 把 streamFn 换成挂起型，确保 goroutine 不退出。
	blockCh := make(chan struct{})
	a.logStreamApp.streamFn = func(ctx context.Context, _ string, _ int, _ func(scanner.LogLine)) error {
		<-blockCh
		return nil
	}

	if err := a.StartLogStream("d1"); err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	// 等 map 里有 entry
	time.Sleep(50 * time.Millisecond)
	if err := a.StartLogStream("d1"); err != nil {
		t.Fatalf("Start 2 (idempotent): %v", err)
	}
	// 释放
	close(blockCh)
	time.Sleep(50 * time.Millisecond)

	// 验证 streamFn 只被调用一次：通过计数 fakeEmitter 上 device-log:d1（应是 0，因为我们没 emit 行）
	// 更直接：验证 logStreams 在 Start 2 后 map 中只一条；这里通过 Stop 行为间接验证。
	if err := a.StopLogStream("d1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	// 再次 Stop no-op
	if err := a.StopLogStream("d1"); err != nil {
		t.Fatalf("Stop 2 (no-op): %v", err)
	}
}

func TestStopLogStream_NoStream(t *testing.T) {
	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	em := &fakeEmitter{}
	a := newTestApp(t, reg, em)
	if err := a.StopLogStream("d1"); err != nil {
		t.Fatalf("Stop without Start should be no-op, got %v", err)
	}
}

func TestRunLogStream_ErrorPropagates(t *testing.T) {
	reg, _ := registry.Open(t.TempDir() + "/reg.json")
	_ = reg.Add(registry.Entry{
		DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
		Online: true, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
	})
	em := &fakeEmitter{}
	a := newTestApp(t, reg, em)
	// 改 streamFn 返回非 ctx.Canceled error
	a.logStreamApp.streamFn = func(_ context.Context, _ string, _ int, _ func(scanner.LogLine)) error {
		return errors.New("boom")
	}

	if err := a.StartLogStream("d1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if em.count("device-log-error:") >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := em.count("device-log-error:d1"); got != 1 {
		t.Errorf("device-log-error:d1 count = %d, want 1", got)
	}
	if got := em.count("device-log-end:d1"); got != 1 {
		t.Errorf("device-log-end:d1 count = %d, want 1", got)
	}
}
