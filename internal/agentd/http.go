package agentd

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Handler returns the HTTP handler exposing /healthz and /api/v1/info.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/api/v1/info", a.handleInfo)
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
		a.log.Error("encode info", slog.String("err", err.Error()))
	}
}

func (a *Agent) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.log.Error("panic in handler",
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
