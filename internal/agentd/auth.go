package agentd

import (
	"log/slog"
	"net/http"
	"strings"
)

// authMiddleware guards "destructive" endpoints with a Bearer token
// when AuthConfig.Enabled is true. Read-only paths (/healthz,
// /api/v1/info, and /admin/*) are exempt so legacy v0.x clients
// can still discover devices and operators can browse the admin
// pages without a Bearer-capable browser extension. Reboot /
// shutdown / logs are protected. Admin pages self-gate via Basic
// auth (see admin.go) using the same token.
func authMiddleware(next http.Handler, cfg AuthConfig, log *slog.Logger) http.Handler {
	if !cfg.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read-only discovery endpoints must remain unauthenticated
		// so v0.x clients continue to discover devices. The /info
		// payload also surfaces auth.required so v0.3 clients know
		// to prompt for a token before invoking write paths.
		// Admin pages self-gate with Basic auth and would
		// otherwise double-prompt the operator.
		if r.URL.Path == "/healthz" ||
			r.URL.Path == "/api/v1/info" ||
			strings.HasPrefix(r.URL.Path, "/admin") {
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
