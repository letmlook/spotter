package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strconv"
	"time"

	"golang.org/x/time/rate"

	"github.com/spotter/spotter/internal/protocol"
)

// ExecSystemctl invokes systemctl with the given action. Package-level
// for test injection; production code does not override it.
var ExecSystemctl = func(action string) error {
	cmd := exec.Command("systemctl", action)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Hand the child to systemd (PID 1) so the agent doesn't keep it
	// attached to its std{fd}. Do NOT Wait — reboot/poweroff will hang
	// the connection.
	return cmd.Process.Release()
}

// Handler returns the HTTP handler exposing /healthz and /api/v1/info.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/api/v1/info", a.handleInfo)
	mux.HandleFunc("/api/v1/reboot", a.handlePowerAction("reboot"))
	mux.HandleFunc("/api/v1/shutdown", a.handlePowerAction("shutdown"))
	mux.HandleFunc("/api/v1/logs", a.handleLogs)
	mux.HandleFunc("POST /api/v1/power", a.handlePowerUnified)
	mux.HandleFunc("GET /api/v1/power/audit", a.handlePowerAuditGet)
	mux.HandleFunc("GET /api/v1/power/audit/recent", a.handlePowerAuditRecent)
	mux.HandleFunc("POST /api/v1/power/cancel", a.handlePowerCancel)
	mux.HandleFunc("GET /api/v1/metrics/recent", a.handleMetricsRecent)
	return a.recoverMiddleware(rateLimitMiddleware(authMiddleware(mux, a.cfg.Auth, a.logger), a.powerLimiter()))
}

// handlePowerCancel aborts a previously scheduled delayed power
// action. The request body must include a `request_id` field
// matching the one the dispatch endpoint was given. A blank or
// missing request_id is a 400; a well-formed but unknown id is a
// 404. On success the endpoint returns 202 with status
// "cancelled" and the underlying delayExec goroutine exits
// without invoking systemctl.
func (a *Agent) handlePowerCancel(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.EnablePowerActions {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "power actions disabled"})
		return
	}
	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.RequestID == "" {
		http.Error(w, "request_id is required", http.StatusBadRequest)
		return
	}
	a.pendingMu.Lock()
	ch, ok := a.pending[body.RequestID]
	if !ok {
		a.pendingMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":      "no pending action with that request_id",
			"status":     "not_found",
			"request_id": body.RequestID,
		})
		return
	}
	// Close the channel under the lock so concurrent cancels
	// don't both try to close; the second close would panic.
	select {
	case <-ch:
		// already closed (e.g. delayExec returned and unregistered)
	default:
		close(ch)
	}
	a.pendingMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":     "cancelled",
		"request_id": body.RequestID,
	})
}

// powerLimiter is the per-IP token bucket gating POST
// /api/v1/{reboot,shutdown}. nil when rate is not configured.
func (a *Agent) powerLimiter() *ipLimiter {
	if a.cfg.Server.PowerActionRatePerS <= 0 {
		return nil
	}
	a.limOnce.Do(func() {
		a.limiter = newIPLimiter(rate.Limit(a.cfg.Server.PowerActionRatePerS), 2)
	})
	return a.limiter
}

func (a *Agent) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		a.logger.Debug("write healthz", slog.String("err", err.Error()))
	}
}

func (a *Agent) handleInfo(w http.ResponseWriter, r *http.Request) {
	// Re-collect on every request so collected_at and uptime_seconds
	// reflect the current snapshot. If a collector isn't installed
	// (e.g. unit tests) we fall back to the cached snapshot. If the
	// fresh collect fails we serve the last known good copy and log
	// at Warn so the operator sees it without spamming Error.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	info, err := a.refreshInfo(ctx)
	if err != nil {
		a.logger.Warn("info: live collect failed; serving cached snapshot",
			slog.String("err", err.Error()))
		info = a.Info()
		// X-Spotter-Stale: true tells the GUI / client to render
		// this row's "CollectedAt" with a visible staleness
		// indicator so operators can tell a fresh collect from
		// the cached copy after a partial failure.
		w.Header().Set("X-Spotter-Stale", "true")
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		a.logger.Error("encode info", slog.String("err", err.Error()))
	}
}

// handlePowerAction returns an http.Handler for POST /api/v1/{reboot,shutdown}.
// The action string is both the URL suffix passed to systemctl and the
// "action" field echoed in the 202 response body.
func (a *Agent) handlePowerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !a.cfg.EnablePowerActions {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			if err := json.NewEncoder(w).Encode(map[string]string{
				"error": "power actions disabled",
			}); err != nil {
				a.logger.Debug("encode power disabled", slog.String("err", err.Error()))
			}
			return
		}
		if err := ExecSystemctl(action); err != nil {
			a.logger.Error("start systemctl",
				slog.String("action", action),
				slog.String("err", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); err != nil {
				a.logger.Debug("encode power error", slog.String("err", err.Error()))
			}
			return
		}
		a.logger.Info("power action scheduled", slog.String("action", action))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"status": "scheduled",
			"action": action,
		}); err != nil {
			a.logger.Debug("encode power ack", slog.String("err", err.Error()))
		}
	}
}

func (a *Agent) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.logger.Error("panic in handler",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// StartHTTP runs the HTTP server on cfg.ListenAddr. It blocks until
// Close() is called or ListenAndServe returns an unexpected error.
// The ctx is honoured by StartUDP / runHeartbeat / runHelloEmit
// (the HTTP server itself is shut down via Close, which lets us
// drain in-flight requests instead of cancelling mid-response).
func (a *Agent) StartHTTP(ctx context.Context) error {
	srv := &http.Server{
		Addr:    a.cfg.ListenAddr,
		Handler: a.Handler(),
	}
	a.SetHTTPServer(srv)
	return srv.ListenAndServe()
}

// Start launches the UDP listener (non-blocking), the heartbeat
// goroutine, the proactive HELLO emitter, then blocks running the HTTP
// server until ctx is cancelled.
func (a *Agent) Start(ctx context.Context) error {
	if err := a.StartUDP(ctx); err != nil {
		return err
	}
	go a.runHeartbeat(ctx)
	go a.runHelloEmit(ctx)
	return a.StartHTTP(ctx)
}

const (
	defaultLogTail = 100
	maxLogTail     = 1000
)

// parseLogTail clamps the ?tail=N query parameter. Empty / 0 /
// negative / non-numeric values fall back to defaultLogTail; values
// above maxLogTail are clamped.
func parseLogTail(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > maxLogTail {
		return maxLogTail
	}
	return n
}

// copyAndFlush copies rc into w, flushing w after every successful Read.
// Returns nil on clean EOF; the first non-EOF error otherwise.
func copyAndFlush(w io.Writer, flusher http.Flusher, rc io.ReadCloser) error {
	buf := make([]byte, 16*1024)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			flusher.Flush()
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// handleLogs streams journalctl output for cfg.LogUnit (or
// "spotterd.service") to the client. The response is NDJSON; each
// record from journalctl --output=json is forwarded as-is and flushed
// immediately so the client sees new lines without buffering.
func (a *Agent) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.cfg.EnableLogStream {
		http.Error(w, "log streaming disabled", http.StatusForbidden)
		return
	}
	unit := a.cfg.LogUnit
	if unit == "" {
		unit = protocol.DefaultLogUnit
	}
	// Compose the unit list: ?unit=foo,bar is added in front of
	// the default so an operator who only sets ?unit=nginx still
	// gets nginx (and not the agent's own log appended to it).
	units := splitUnits(r.URL.Query().Get("unit"))
	if len(units) == 0 {
		units = []string{unit}
	} else {
		units = append([]string{unit}, units...)
	}
	tail := parseLogTail(r.URL.Query().Get("tail"), defaultLogTail)
	q := r.URL.Query()
	opts := JournalctlOpts{
		Units:    units,
		Tail:     tail,
		Grep:     q.Get("grep"),
		Since:    q.Get("since"),
		Priority: q.Get("priority"),
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	rc, kill, err := startJournalctl(r.Context(), opts)
	if err != nil {
		a.logger.Error("start journalctl",
			slog.String("unit", unit),
			slog.String("err", err.Error()))
		// 已 WriteHeader(200)；写一行 error 后关闭流，让客户端 reader
		// 解析失败 → UI 显示错误。
		if _, werr := io.WriteString(w, `{"error":"journalctl not available"}`+"\n"); werr != nil {
			a.logger.Debug("write journalctl-error ndjson", slog.String("err", werr.Error()))
		}
		flusher.Flush()
		return
	}
	defer kill()

	if err := copyAndFlush(w, flusher, rc); err != nil {
		if !errors.Is(err, context.Canceled) {
			a.logger.Info("log stream ended",
				slog.String("unit", unit),
				slog.String("err", err.Error()))
		}
	}
}
