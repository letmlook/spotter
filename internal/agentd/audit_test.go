package agentd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"
)

// TestAuditLogger_RoundTrip — write N entries, read back, assert
// order and field shape.
func TestAuditLogger_Recent_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.tsv")
	al, err := NewAuditLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer al.Close()
	for i := 0; i < 5; i++ {
		al.Record("reboot", i%2 == 0, "req-"+string(rune('a'+i)), "10.0.0.1", "scheduled")
	}
	got, err := al.Recent(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	// Most-recent N: indices 2, 3, 4 of the writes = req-c, req-d, req-e.
	wantReq := []string{"req-c", "req-d", "req-e"}
	for i, e := range got {
		if e.RequestID != wantReq[i] {
			t.Errorf("[%d].RequestID = %q, want %q", i, e.RequestID, wantReq[i])
		}
		if e.Action != "reboot" {
			t.Errorf("[%d].Action = %q, want reboot", i, e.Action)
		}
		if e.Result != "scheduled" {
			t.Errorf("[%d].Result = %q, want scheduled", i, e.Result)
		}
	}
}

// TestAuditLogger_Recent_Zero — limit 0 returns nil (no allocation).
func TestAuditLogger_Recent_Zero(t *testing.T) {
	dir := t.TempDir()
	al, _ := NewAuditLogger(filepath.Join(dir, "audit.tsv"))
	defer al.Close()
	al.Record("reboot", false, "r", "ip", "scheduled")
	got, err := al.Recent(0)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("limit=0 should return nil, got %d", len(got))
	}
}

// TestHandlePowerAuditRecent_OK — record some entries, fetch
// the recent endpoint, assert JSON shape.
func TestHandlePowerAuditRecent_OK(t *testing.T) {
	a, _ := New(Config{DeviceID: "x", ListenAddr: "127.0.0.1:0"}, slog.Default())
	al, _ := NewAuditLogger(filepath.Join(t.TempDir(), "audit.tsv"))
	defer al.Close()
	a.SetAuditLogger(al)
	for i := 0; i < 4; i++ {
		al.Record("reboot", false, "r", "10.0.0.1", "scheduled")
	}
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/power/audit/recent?limit=2")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Count   int          `json:"count"`
		Entries []AuditEntry `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 2 {
		t.Errorf("count = %d, want 2", body.Count)
	}
	if len(body.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(body.Entries))
	}
}

// TestHandlePowerAuditRecent_503WhenNil — no audit logger
// attached returns 503 (not 500 or panic).
func TestHandlePowerAuditRecent_503WhenNil(t *testing.T) {
	a, _ := New(Config{DeviceID: "x", ListenAddr: "127.0.0.1:0"}, slog.Default())
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/power/audit/recent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

// TestHandlePowerAuditRecent_LimitCap — limit > 200 gets capped
// to 200 (we don't want a single request to drain the file).
func TestHandlePowerAuditRecent_LimitCap(t *testing.T) {
	a, _ := New(Config{DeviceID: "x", ListenAddr: "127.0.0.1:0"}, slog.Default())
	al, _ := NewAuditLogger(filepath.Join(t.TempDir(), "audit.tsv"))
	defer al.Close()
	a.SetAuditLogger(al)
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/power/audit/recent?limit=99999")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// Status is 200 either way; the cap is silent. We can't
	// assert on the cap value from the wire shape, but we can
	// at least confirm the request didn't 400 or 500.
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestAuditLogger_Recent_SkipsMalformed — a single corrupt line
// must not sink the whole response; the decoder skips and
// returns the surrounding good rows.
func TestAuditLogger_Recent_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.tsv")
	// Pre-write: 1 valid line, 1 garbage, 1 valid line.
	good := "2026-08-27T22:00:00Z\treboot\tdry_run=false\treq=r1\tip=10.0.0.1\tresult=scheduled\n"
	bad := "this is not a valid audit line\n"
	if err := os.WriteFile(path, []byte(good+bad+good), 0600); err != nil {
		t.Fatal(err)
	}
	al, _ := NewAuditLogger(path)
	defer al.Close()
	got, err := al.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	// 2 good + 1 bad; decoder skips the bad row, returns the
	// 2 good ones. Shift-left behaviour: last 2 of the 2 good
	// rows = both good rows.
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2 (skip the malformed row)", len(got))
	}
	for i, e := range got {
		if !strings.HasPrefix(e.RequestID, "r1") {
			t.Errorf("entry %d: req = %q, want r1 prefix", i, e.RequestID)
		}
	}
}
