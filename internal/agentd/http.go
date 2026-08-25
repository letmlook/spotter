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
	mux.HandleFunc("/api/v1/power", a.handlePowerDispatch)
	mux.HandleFunc("/api/v1/power/cancel", a.handlePowerCancel)
	return a.recoverMiddleware(rateLimitMiddleware(authMiddleware(mux, a.cfg.Auth, a.logger), a.powerLimiter()))
}

// handlePowerCancel terminates the most recent pending delayed
// dispatch for this device. Until v0.6 ships a pid-file-backed
// cancel API, the endpoint returns 501 Not Implemented rather
// than the previous fake 200 success — the GUI's Cancel button
// should now visibly no-op instead of silently lying.
func (a *Agent) handlePowerCancel(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  "cancel not yet implemented",
		"status": "would_cancel",
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

// StartHTTP runs the HTTP server on cfg.ListenAddr. It blocks until ctx is
// cancelled (or ListenAndServe returns an unexpected error), then calls
// srv.Shutdown with a 5-second timeout.
func (a *Agent) StartHTTP(ctx context.Context) error {
	srv := &http.Server{
		Addr:    a.cfg.ListenAddr,
		Handler: a.Handler(),
	}
	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errCh <- srv.Shutdown(shutdownCtx)
	}()
	a.logger.Info("http listening", slog.String("addr", a.cfg.ListenAddr))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// Wait for shutdown to complete (or return its error).
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
		unit = "spotterd.service"
	}
	tail := parseLogTail(r.URL.Query().Get("tail"), defaultLogTail)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	rc, kill, err := startJournalctl(r.Context(), unit, tail)
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
