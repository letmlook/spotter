package agentd

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
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

// Record writes a TSV line: timestamp, action, dry_run, request_id,
// remote_ip, result.
func (a *AuditLogger) Record(action string, dryRun bool, requestID, remote, result string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.f == nil {
		return
	}
	fmt.Fprintf(a.f, "%s\t%s\tdry_run=%v\treq=%s\tip=%s\tresult=%s\n",
		time.Now().UTC().Format(time.RFC3339),
		action, dryRun, requestID, remote, result)
}

// handlePowerDispatch is the v0.5+ unified endpoint. It dispatches
// /api/v1/power (POST) for the unified request shape; GET on the
// same path returns the local audit log as JSON.
func (a *Agent) handlePowerDispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handlePowerAuditGet(w, r)
	case http.MethodPost:
		a.handlePowerUnified(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
			go a.delayExec(req, r.RemoteAddr, executeAt)
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

// delayExec sleeps until t, then runs ExecSystemctl. It honours
// ctx cancellation via the agent's shutdown channel — best effort,
// the process will exit if delayed past the deadline anyway.
func (a *Agent) delayExec(req PowerRequest, remote string, at time.Time) {
	d := time.Until(at)
	if d <= 0 {
		d = time.Second
	}
	time.Sleep(d)
	_ = ExecSystemctl(req.Action)
	if a.audit != nil {
		a.audit.Record(req.Action, false, req.RequestID, remote, "delayed-executed")
	}
}
