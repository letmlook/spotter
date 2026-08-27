package agentd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/timefmt"
)

// PowerRequest is the body of POST /api/v1/power. We accept the
// unified shape (action + dry_run + delay_minutes) so v0.5+ clients
// can surface one dialog for every variant. v0.4 clients that
// still call /api/v1/{reboot,shutdown} continue to work via the
// legacy endpoints defined alongside this one.
type PowerRequest struct {
	Action        string `json:"action"`         // "reboot" or "shutdown"
	DryRun        bool   `json:"dry_run"`        // when true, return would_execute=true without acting
	DelayMinutes  int    `json:"delay_minutes"`  // 0 = immediate
	RequestID     string `json:"request_id"`     // optional; the cancel endpoint keys off this
}

// PowerResponse is the reply payload. WouldExecute is true on a
// dry-run or when the agent has scheduled a delayed action.
type PowerResponse struct {
	Status        string `json:"status"`         // "scheduled" / "would_execute" / "running" / "cancelled"
	Action        string `json:"action"`
	DryRun        bool   `json:"dry_run"`
	DelayMinutes  int    `json:"delay_minutes"`
	WouldExecute  bool   `json:"would_execute"`
	ExecuteAt     string `json:"execute_at,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
}

// AuditLogger appends one TSV line per power action to a file the
// operator can grep / forward to a SIEM. The mutex guards the
// underlying *os.File — appends are infrequent.
type AuditLogger struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// AuditEntry is one TSV line decoded back into a structured row.
// The wire shape is what /api/v1/power/audit/recent returns to
// the GUI; the file format stays TSV for grep-forwardability.
type AuditEntry struct {
	At         time.Time `json:"at"`
	Action     string    `json:"action"`
	DryRun     bool      `json:"dry_run"`
	RequestID  string    `json:"request_id,omitempty"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
	Result     string    `json:"result"`
}

func NewAuditLogger(path string) (*AuditLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	// O_RDWR (not O_WRONLY) so GET /api/v1/power can stream the
	// file back over HTTP. The audit log is append-only by
	// contract (Record never seeks before writing), but the GET
	// path needs read access on the same fd.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0640)
	if err != nil {
		return nil, fmt.Errorf("audit open: %w", err)
	}
	return &AuditLogger{path: path, f: f}, nil
}

func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.f != nil {
		return a.f.Close()
	}
	return nil
}

// Record writes a TSV line. Field order:
// timestamp \t action \t dry_run=... \t req=... \t ip=... \t status=...
//
// The status field uses the same `key=value` shape as the other
// metadata so the audit decoder can treat every column after the
// first two as opaque key-value pairs; this keeps the format
// forward-compatible (adding a new column is a parse change, not
// a format change).
func (a *AuditLogger) Record(action string, dryRun bool, requestID, remote, result string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.f == nil {
		return
	}
	fmt.Fprintf(a.f, "%s\t%s\tdry_run=%v\treq=%s\tip=%s\tstatus=%s\n",
		timefmt.NowUTC(),
		action, dryRun, requestID, remote, result)
}

// Recent returns the last n audit entries, oldest first. The
// function reads the file tail with a small overshoot so we
// don't have to seek-line-counted from byte 0 on a long-running
// log. Implementation: scan the file once into a ring of size n
// and return the ring in chronological order.
func (a *AuditLogger) Recent(n int) ([]AuditEntry, error) {
	if n <= 0 {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.f == nil {
		return nil, nil
	}
	// Read all entries by rewinding and scanning. The file is
	// append-only and small (a few KB per day on most fleets),
	// so reading it whole is cheaper than maintaining a
	// cursor. For very long-running deployments a future
	// optimisation is to read backwards in 4KB chunks and
	// stop at n newline boundaries.
	if _, err := a.f.Seek(0, 0); err != nil {
		return nil, err
	}
	dec := newAuditDecoder(a.f)
	all := make([]AuditEntry, 0, n)
	for {
		entry, err := dec.decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A single malformed line shouldn't sink the
			// whole response; skip and continue.
			continue
		}
		if len(all) < n {
			all = append(all, entry)
		} else {
			// Shift left and append — cheaper than a ring
			// because n is small (≤100 in practice).
			copy(all, all[1:])
			all[len(all)-1] = entry
		}
	}
	return all, nil
}

// auditDecoder splits the TSV format into AuditEntry rows.
// Field order matches Record(): timestamp, action, dry_run=...,
// req=..., ip=..., result=...
type auditDecoder struct {
	sc *bufio.Scanner
}

func newAuditDecoder(r io.Reader) *auditDecoder {
	sc := bufio.NewScanner(r)
	// 1MB max line; we never write lines that long but a
	// truncated tail from a power cut shouldn't crash the
	// decoder.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &auditDecoder{sc: sc}
}

func (d *auditDecoder) decode() (AuditEntry, error) {
	if !d.sc.Scan() {
		if err := d.sc.Err(); err != nil {
			return AuditEntry{}, err
		}
		return AuditEntry{}, io.EOF
	}
	line := d.sc.Text()
	if line == "" {
		return AuditEntry{}, io.EOF // skip blanks; treat as EOF
	}
	fields := strings.SplitN(line, "\t", 6)
	if len(fields) < 6 {
		return AuditEntry{}, fmt.Errorf("audit: short line %q", line)
	}
	entry := AuditEntry{Action: fields[1]}
	if t, err := time.Parse(time.RFC3339Nano, fields[0]); err == nil {
		entry.At = t
	}
	// Fields 2..5 are all `key=value` pairs: dry_run=, req=,
	// ip=, status=. Decode them uniformly so a future column
	// addition (e.g. duration=) is a one-line change to this
	// switch.
	for _, kv := range fields[2:6] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
		case "dry_run":
			entry.DryRun = v == "true"
		case "req":
			entry.RequestID = v
		case "ip":
			entry.RemoteAddr = v
		case "status":
			entry.Result = v
		}
	}
	return entry, nil
}

// handlePowerDispatch was removed when /api/v1/power split into
// two endpoints (POST for unified dispatch, GET /api/v1/power/audit
// for the audit log). The mux in Handler() now matches method +
// path separately, and a 405 falls out automatically when neither
// pattern matches.

// handlePowerAuditRecent returns the last N audit entries as a
// JSON array (vs handlePowerAuditGet's NDJSON stream over the
// whole file). `limit` defaults to 50 and is capped at 200 so
// the response stays under ~20 KiB even on chatty agents. Used
// by the GUI to render a recent-activity list in the detail
// panel.
func (a *Agent) handlePowerAuditRecent(w http.ResponseWriter, r *http.Request) {
	if a.audit == nil {
		http.Error(w, "audit log unavailable", http.StatusServiceUnavailable)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	entries, err := a.audit.Recent(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}

func (a *Agent) handlePowerAuditGet(w http.ResponseWriter, _ *http.Request) {
	if a.audit == nil {
		http.Error(w, "audit log unavailable", http.StatusServiceUnavailable)
		return
	}
	a.audit.mu.Lock()
	defer a.audit.mu.Unlock()
	if a.audit.f == nil {
		http.Error(w, "audit not open", http.StatusServiceUnavailable)
		return
	}
	if _, err := a.audit.f.Seek(0, 0); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	// io.Copy drains the file in 32 KB chunks; previously this
	// loop read once into a 4 KB buffer and broke, capping every
	// response at the first chunk.
	if _, err := io.Copy(w, a.audit.f); err != nil {
		a.logger.Error("audit stream", slog.String("err", err.Error()))
	}
}

// handlePowerUnified accepts a power dispatch. Delayed actions
// register a cancel channel keyed by request_id (when supplied)
// so /api/v1/power/cancel can interrupt them. A missing
// request_id on a delayed action is allowed but not cancellable
// — callers that want cancellation must supply one.
func (a *Agent) handlePowerUnified(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnablePowerActions {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "power actions disabled"})
		return
	}
	var req PowerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Action != "reboot" && req.Action != "shutdown" {
		http.Error(w, "action must be reboot or shutdown", http.StatusBadRequest)
		return
	}
	if req.DelayMinutes < 0 || req.DelayMinutes > 24*60 {
		http.Error(w, "delay_minutes must be between 0 and 1440", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	resp := PowerResponse{
		Action:       req.Action,
		DryRun:       req.DryRun,
		DelayMinutes: req.DelayMinutes,
		RequestID:    req.RequestID,
	}
	if req.DryRun {
		resp.Status = "would_execute"
		resp.WouldExecute = true
	} else {
		resp.Status = "scheduled"
		if req.DelayMinutes > 0 {
			executeAt := time.Now().Add(time.Duration(req.DelayMinutes) * time.Minute)
			resp.ExecuteAt = executeAt.UTC().Format(time.RFC3339)
			cancelCh := a.registerPending(req.RequestID)
			// Detach from r.Context(): net/http cancels the
			// request context as soon as the handler returns,
			// which would fire delayExec's ctx.Done branch
			// immediately and unregister the pending action
			// before the operator could cancel it. The agent
			// lifecycle ctx is owned by cmd/agent (signal-driven)
			// and is the right parent for long-running background
			// work; threading it through here would require a
			// wider refactor, so for now we use Background with
			// the request's values (for logging/tracing) and a
			// deadline that outlives the longest valid delay.
			detached, detachedCancel := context.WithTimeout(
				context.WithoutCancel(r.Context()),
				time.Duration(req.DelayMinutes+5)*time.Minute,
			)
			// We don't need the cancel in the parent path (the
			// detached ctx outlives the handler), but the
			// returned cancel must run somewhere to free the
			// context resources when the goroutine finishes.
			// Hand it to delayExec via a closure.
			go func() {
				defer detachedCancel()
				a.delayExec(detached, req, r.RemoteAddr, executeAt, cancelCh)
			}()
		} else {
			if err := ExecSystemctl(req.Action); err != nil {
				resp.Status = "error"
				a.logger.Error("power dispatch", slog.String("action", req.Action), slog.String("err", err.Error()))
			}
		}
	}
	_ = json.NewEncoder(w).Encode(resp)
	if a.audit != nil {
		a.audit.Record(req.Action, req.DryRun, req.RequestID, r.RemoteAddr, resp.Status)
	}
}

// delayExec sleeps until at, then runs ExecSystemctl. Three exit
// paths: ctx cancelled (client disconnect or agent shutdown),
// cancelCh closed (POST /api/v1/power/cancel), or timer fired.
// Only the timer path actually runs systemctl; the other two
// record "cancelled" in the audit log and return.
func (a *Agent) delayExec(ctx context.Context, req PowerRequest, remote string, at time.Time, cancelCh <-chan struct{}) {
	d := time.Until(at)
	if d <= 0 {
		d = time.Second
	}
	t := time.NewTimer(d)
	defer t.Stop()
	defer a.unregisterPending(req.RequestID)
	select {
	case <-ctx.Done():
		if a.audit != nil {
			a.audit.Record(req.Action, false, req.RequestID, remote, "cancelled")
		}
		return
	case <-cancelCh:
		if a.audit != nil {
			a.audit.Record(req.Action, false, req.RequestID, remote, "cancelled")
		}
		return
	case <-t.C:
	}
	_ = ExecSystemctl(req.Action)
	if a.audit != nil {
		a.audit.Record(req.Action, false, req.RequestID, remote, "delayed-executed")
	}
}

// registerPending attaches a fresh cancel channel to the in-memory
// pending map under requestID. A blank requestID is not
// registered (cancellation requires a stable key). The returned
// channel is closed by unregisterPending when delayExec returns
// or by handlePowerCancel when the operator wants to abort early.
// Safe to call when a.audit is nil — the field is only consulted
// for logging, and the channel itself is independent of the audit
// subsystem.
func (a *Agent) registerPending(requestID string) chan struct{} {
	if requestID == "" {
		// Return a never-fired channel so delayExec's select
		// statement doesn't have a nil case. The request is
		// uncancellable but otherwise behaves normally.
		return make(chan struct{})
	}
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	ch := make(chan struct{})
	a.pending[requestID] = ch
	return ch
}

func (a *Agent) unregisterPending(requestID string) {
	if requestID == "" {
		return
	}
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if ch, ok := a.pending[requestID]; ok {
		// Idempotent close so a delayed-executed path doesn't
		// double-panic if cancel races with the timer.
		select {
		case <-ch:
			// already closed
		default:
			close(ch)
		}
		delete(a.pending, requestID)
	}
}
