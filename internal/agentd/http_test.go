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

	"github.com/spotter/spotter/internal/agentd"
)

func TestPowerAction_DisabledReturns403(t *testing.T) {
	a, err := agentd.New(agentd.Config{
		DeviceID:           "x",
		ListenAddr:         "127.0.0.1:0",
		AgentVersion:       "0.1.0",
		EnablePowerActions: false,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, action := range []string{"reboot", "shutdown"} {
		resp, err := http.Post(ts.URL+"/api/v1/"+action, "application/json", nil)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403", action, resp.StatusCode)
		}
		var got map[string]string
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: decode: %v", action, err)
		}
		if got["error"] != "power actions disabled" {
			t.Errorf("%s: error=%q, want %q", action, got["error"], "power actions disabled")
		}
	}
}

func TestPowerAction_EnabledSchedulesAndCallsExecutor(t *testing.T) {
	var (
		mu      sync.Mutex
		invoked []string
	)
	orig := agentd.ExecSystemctl
	agentd.ExecSystemctl = func(action string) error {
		mu.Lock()
		invoked = append(invoked, action)
		mu.Unlock()
		return nil
	}
	defer func() { agentd.ExecSystemctl = orig }()

	a, err := agentd.New(agentd.Config{
		DeviceID:           "x",
		ListenAddr:         "127.0.0.1:0",
		AgentVersion:       "0.1.0",
		EnablePowerActions: true,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, action := range []string{"reboot", "shutdown"} {
		resp, err := http.Post(ts.URL+"/api/v1/"+action, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("%s: got %d, want 202", action, resp.StatusCode)
		}
		var got map[string]string
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: decode: %v", action, err)
		}
		if got["status"] != "scheduled" || got["action"] != action {
			t.Errorf("%s: body=%v", action, got)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(invoked) != 2 || invoked[0] != "reboot" || invoked[1] != "shutdown" {
		t.Errorf("invoked=%v, want [reboot shutdown]", invoked)
	}
}

func TestPowerAction_NonPOSTReturns405(t *testing.T) {
	a, err := agentd.New(agentd.Config{
		DeviceID:           "x",
		ListenAddr:         "127.0.0.1:0",
		AgentVersion:       "0.1.0",
		EnablePowerActions: true,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, action := range []string{"reboot", "shutdown"} {
		resp, err := http.Get(ts.URL + "/api/v1/" + action)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s: got %d, want 405", action, resp.StatusCode)
		}
		if !strings.Contains(string(body), "method not allowed") {
			t.Errorf("%s: body=%q", action, body)
		}
	}
}
