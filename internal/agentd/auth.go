package agentd

import (
	"log/slog"
	"net/http"
	"strings"
)

// authMiddleware guards "destructive" endpoints with a Bearer token
// when AuthConfig.Enabled is true. Read-only paths (/healthz and
// /api/v1/info) are exempt so legacy v0.x clients — which don't
// know about tokens — can still discover devices and read the
// auth-required flag from the response body. Reboot/shutdown/logs
// are protected.
func authMiddleware(next http.Handler, cfg AuthConfig, log *slog.Logger) http.Handler {
	if !cfg.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read-only discovery endpoints must remain unauthenticated
		// so v0.x clients continue to discover devices. The /info
		// payload also surfaces auth.required so v0.3 clients know
		// to prompt for a token before invoking write paths.
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/v1/info" {
			next.ServeHTTP(w, r)
			return
		}
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") || h[len("Bearer "):] != cfg.Token {
			if log != nil {
				log.Warn("auth.rejected",
					slog.String("ip", r.RemoteAddr),
					slog.String("path", r.URL.Path),
				)
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="spotterd"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
