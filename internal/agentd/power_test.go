package agentd_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/agentd"
)

// stubExec replaces agentd.ExecSystemctl for one test and restores it
// afterwards. Records every action it sees.
func stubExec(t *testing.T) *[]string {
	t.Helper()
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
	t.Cleanup(func() { agentd.ExecSystemctl = orig })
	return &invoked
}

func newPowerAgent(t *testing.T, enable bool) *agentd.Agent {
	t.Helper()
	a, err := agentd.New(agentd.Config{
		DeviceID:           "x",
		ListenAddr:         "127.0.0.1:0",
		AgentVersion:       "0.1.0",
		EnablePowerActions: enable,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestAuditLogger_RoundTrip verifies that an AuditLogger writes the
// expected TSV row, and that Close is idempotent and frees the fd.
func TestAuditLogger_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	al, err := agentd.NewAuditLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	al.Record("reboot", false, "req-1", "10.0.0.1", "scheduled")
	al.Record("shutdown", true, "req-2", "10.0.0.2", "would_execute")
	if err := al.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 audit lines, got %d:\n%s", len(lines), data)
	}
	if !strings.Contains(lines[0], "reboot") || !strings.Contains(lines[0], "req-1") {
		t.Errorf("line 0 missing fields: %q", lines[0])
	}
	if !strings.Contains(lines[1], "shutdown") || !strings.Contains(lines[1], "dry_run=true") {
		t.Errorf("line 1 missing fields: %q", lines[1])
	}
	// Each line should have at least 6 tab-separated columns.
	for i, line := range lines {
		if got := strings.Count(line, "\t"); got < 5 {
			t.Errorf("line %d: want ≥6 columns, got %d (%q)", i, got, line)
		}
	}
}

// TestPowerUnified_DisabledReturns403 — the explicit gate: when the
// operator has not opted in, POST /api/v1/power returns 403 + the
// "power actions disabled" envelope (not 200 + silent no-op).
func TestPowerUnified_DisabledReturns403(t *testing.T) {
	a := newPowerAgent(t, false)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/power", "application/json",
		strings.NewReader(`{"action":"reboot"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("got %d, want 403", resp.StatusCode)
	}
	var got map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["error"] != "power actions disabled" {
		t.Errorf("error=%q, want %q", got["error"], "power actions disabled")
	}
}

// TestPowerUnified_BadAction — action must be reboot or shutdown.
func TestPowerUnified_BadAction(t *testing.T) {
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/power", "application/json",
		strings.NewReader(`{"action":"frobnicate"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d, want 400", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "action must be reboot or shutdown") {
		t.Errorf("body=%q, want action validation message", body)
	}
}

// TestPowerUnified_BadDelay — DelayMinutes ∈ [0, 1440].
func TestPowerUnified_BadDelay(t *testing.T) {
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, dm := range []int{-1, 24*60 + 1} {
		body, _ := json.Marshal(map[string]any{"action": "reboot", "delay_minutes": dm})
		resp, err := http.Post(ts.URL+"/api/v1/power", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("delay=%d: got %d, want 400 (body=%s)", dm, resp.StatusCode, respBody)
		}
	}
}

// TestPowerUnified_DryRun — must return 202 + would_execute=true and
// MUST NOT call ExecSystemctl.
func TestPowerUnified_DryRun(t *testing.T) {
	invoked := stubExec(t)
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/power", "application/json",
		strings.NewReader(`{"action":"reboot","dry_run":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("got %d, want 202", resp.StatusCode)
	}
	var got agentd.PowerResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "would_execute" || !got.WouldExecute {
		t.Errorf("status=%q would_execute=%v, want would_execute=true", got.Status, got.WouldExecute)
	}
	if !got.DryRun {
		t.Errorf("DryRun not echoed back")
	}
	// Drain any async work scheduled before stubExec took effect.
	time.Sleep(20 * time.Millisecond)
	if len(*invoked) != 0 {
		t.Errorf("dry_run should not invoke ExecSystemctl, got %v", *invoked)
	}
}

// TestPowerUnified_ImmediateExec — happy path: status=scheduled,
// ExecSystemctl called once with the right action, audit row written.
func TestPowerUnified_ImmediateExec(t *testing.T) {
	invoked := stubExec(t)
	a := newPowerAgent(t, true)
	alPath := filepath.Join(t.TempDir(), "audit.log")
	al, err := agentd.NewAuditLogger(alPath)
	if err != nil {
		t.Fatal(err)
	}
	defer al.Close()
	a.SetAuditLogger(al)

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, action := range []string{"reboot", "shutdown"} {
		body, _ := json.Marshal(map[string]any{"action": action, "request_id": "rid-" + action})
		resp, err := http.Post(ts.URL+"/api/v1/power", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var got agentd.PowerResponse
		_ = json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("%s: got %d, want 202", action, resp.StatusCode)
		}
		if got.Status != "scheduled" || got.Action != action {
			t.Errorf("%s: status=%q action=%q, want scheduled/%s", action, got.Status, got.Action, action)
		}
	}

	// ExecSystemctl should have been called exactly twice with the
	// actions we requested (one per POST above).
	wantCalls := map[string]bool{"reboot": true, "shutdown": true}
	for _, c := range *invoked {
		if !wantCalls[c] {
			t.Errorf("unexpected ExecSystemctl call: %q", c)
		}
		delete(wantCalls, c)
	}
	if len(wantCalls) != 0 {
		t.Errorf("missing ExecSystemctl calls: %v", wantCalls)
	}

	// Audit log should have one row per dispatch.
	audit, _ := os.ReadFile(alPath)
	rows := strings.Split(strings.TrimRight(string(audit), "\n"), "\n")
	if len(rows) != 2 {
		t.Errorf("want 2 audit rows, got %d:\n%s", len(rows), audit)
	}
	for _, row := range rows {
		if !strings.Contains(row, "result=scheduled") {
			t.Errorf("audit row missing result: %q", row)
		}
	}
}

// TestPowerUnified_Delayed — DelayMinutes=1 schedules and returns
// ExecuteAt; we do NOT wait for the goroutine. Verify the response
// shape and that ExecSystemctl is not called synchronously.
func TestPowerUnified_Delayed(t *testing.T) {
	invoked := stubExec(t)
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/power", "application/json",
		strings.NewReader(`{"action":"shutdown","delay_minutes":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d, want 202", resp.StatusCode)
	}
	var got agentd.PowerResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ExecuteAt == "" {
		t.Errorf("ExecuteAt empty for delayed dispatch")
	}
	if _, err := time.Parse(time.RFC3339, got.ExecuteAt); err != nil {
		t.Errorf("ExecuteAt not RFC3339: %q (%v)", got.ExecuteAt, err)
	}
	// Wait briefly to ensure no early fire; ExecSystemctl must not
	// have been called synchronously.
	time.Sleep(20 * time.Millisecond)
	if len(*invoked) != 0 {
		t.Errorf("delayed dispatch should not invoke ExecSystemctl synchronously, got %v", *invoked)
	}
}

// TestPowerAuditGet_NoAudit — when SetAuditLogger was never called,
// GET /api/v1/power returns 503 + "audit log unavailable".
func TestPowerAuditGet_NoAudit(t *testing.T) {
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/power/audit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "audit log unavailable") {
		t.Errorf("body=%q, want audit log unavailable", body)
	}
}

// TestPowerAuditGet_WithAudit — attach an audit log, GET /api/v1/power
// returns 200 + application/x-ndjson with the recorded rows.
func TestPowerAuditGet_WithAudit(t *testing.T) {
	a := newPowerAgent(t, true)
	alPath := filepath.Join(t.TempDir(), "audit.log")
	al, err := agentd.NewAuditLogger(alPath)
	if err != nil {
		t.Fatal(err)
	}
	defer al.Close()
	a.SetAuditLogger(al)

	al.Record("reboot", false, "rid-A", "10.0.0.5", "scheduled")
	al.Record("shutdown", true, "rid-B", "10.0.0.6", "would_execute")

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

		resp, err := http.Get(ts.URL + "/api/v1/power/audit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("Content-Type=%q, want application/x-ndjson", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "rid-A") || !strings.Contains(string(body), "rid-B") {
		t.Errorf("audit body missing rows: %q", body)
	}
}

// TestPowerUnified_MethodNotAllowed — PUT against /api/v1/power
// must 405 (only POST is registered).
func TestPowerUnified_MethodNotAllowed(t *testing.T) {
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/power", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", resp.StatusCode)
	}
}

// TestPowerAuditGet_MethodNotAllowed — POST against
// /api/v1/power/audit must 405 (only GET is registered).
func TestPowerAuditGet_MethodNotAllowed(t *testing.T) {
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/power/audit", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("got %d, want 405", resp.StatusCode)
	}
}

// TestPowerCancel_NotImplemented — until v0.6 ships a pid-file
// cancel API, the cancel endpoint returns 501 rather than the
// previous fake-200 success.
func TestPowerCancel_NotImplemented(t *testing.T) {
	a := newPowerAgent(t, true)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, ts.URL+"/api/v1/power/cancel", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s: got %d, want 501 (body=%s)", method, resp.StatusCode, body)
		}
		if !strings.Contains(string(body), "cancel not yet implemented") {
			t.Errorf("%s: body=%q, want honest not-implemented message", method, body)
		}
	}
}
