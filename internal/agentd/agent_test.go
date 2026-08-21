package agentd_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/agentd"
	"github.com/spotter/spotter/internal/protocol"
)

type stubSource struct {
	mu   sync.Mutex
	info protocol.DeviceInfo
}

func (s *stubSource) Info() protocol.DeviceInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

func TestHealthz(t *testing.T) {
	src := &stubSource{info: protocol.DeviceInfo{DeviceID: "x"}}
	a, err := agentd.New(agentd.Config{
		DeviceID:     "x",
		ListenAddr:   "127.0.0.1:0",
		AgentVersion: "0.1.0",
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	a.SetInfoForTest(src.Info())

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("got %d %q", resp.StatusCode, body)
	}
}

func TestInfoEndpoint(t *testing.T) {
	want := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      "test-device",
		AgentVersion:  "0.1.0",
		Basic:         protocol.BasicInfo{Hostname: "h", Arch: "aarch64"},
	}
	a, err := agentd.New(agentd.Config{DeviceID: "test-device", ListenAddr: "127.0.0.1:0", AgentVersion: "0.1.0"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	a.SetInfoForTest(want)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got protocol.DeviceInfo
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.DeviceID != "test-device" || got.Basic.Hostname != "h" {
		t.Errorf("got %+v", got)
	}
	_ = time.Second
}

func TestNewValidatesConfig(t *testing.T) {
	if _, err := agentd.New(agentd.Config{ListenAddr: "127.0.0.1:0"}, nil); err == nil {
		t.Error("expected error for missing device_id")
	}
	if _, err := agentd.New(agentd.Config{DeviceID: "x"}, nil); err == nil {
		t.Error("expected error for missing listen_addr")
	}
}
