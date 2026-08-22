package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime/debug"
	"time"
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
	return a.recoverMiddleware(mux)
}

func (a *Agent) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *Agent) handleInfo(w http.ResponseWriter, _ *http.Request) {
	info := a.Info()
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
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "power actions disabled",
			})
			return
		}
		if err := ExecSystemctl(action); err != nil {
			a.logger.Error("start systemctl",
				slog.String("action", action),
				slog.String("err", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		a.logger.Info("power action scheduled", slog.String("action", action))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "scheduled",
			"action": action,
		})
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

// Start launches the UDP listener (non-blocking) then blocks running the
// HTTP server until ctx is cancelled.
func (a *Agent) Start(ctx context.Context) error {
	if err := a.StartUDP(ctx); err != nil {
		return err
	}
	return a.StartHTTP(ctx)
}
