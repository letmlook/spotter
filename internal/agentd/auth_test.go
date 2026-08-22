package agentd

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spotter/spotter/internal/protocol"
)

// okHandler is a no-op downstream handler; if the middleware lets the
// request through we know the auth path either succeeded or the test
// configured no auth.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTeapot) // distinctive, not 200/401
})

func newAuthLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

func TestAuth_DisabledPassesEverythingThrough(t *testing.T) {
	mw := authMiddleware(okHandler, AuthConfig{Enabled: false}, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reboot", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("want %d, got %d", http.StatusTeapot, rec.Code)
	}
}

func TestAuth_HealthzExempt(t *testing.T) {
	logger, _ := newAuthLogger()
	mw := authMiddleware(okHandler, AuthConfig{Enabled: true, Token: "secret"}, logger)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("want healthz pass-through, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAuth_ValidTokenPassesThrough(t *testing.T) {
	logger, _ := newAuthLogger()
	mw := authMiddleware(okHandler, AuthConfig{Enabled: true, Token: "secret"}, logger)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reboot", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("want pass-through, got %d", rec.Code)
	}
}

func TestAuth_RejectsMissingToken(t *testing.T) {
	logger, buf := newAuthLogger()
	mw := authMiddleware(okHandler, AuthConfig{Enabled: true, Token: "secret"}, logger)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reboot", nil)
	req.RemoteAddr = "192.0.2.1:55555"
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Error("missing WWW-Authenticate header")
	}
	if !strings.Contains(buf.String(), "auth.rejected") {
		t.Errorf("expected auth.rejected log line, got: %q", buf.String())
	}
}

func TestAuth_RejectsWrongToken(t *testing.T) {
	logger, _ := newAuthLogger()
	mw := authMiddleware(okHandler, AuthConfig{Enabled: true, Token: "secret"}, logger)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reboot", nil)
	req.Header.Set("Authorization", "Bearer not-the-secret")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestAuth_RejectsMalformedHeader(t *testing.T) {
	logger, _ := newAuthLogger()
	mw := authMiddleware(okHandler, AuthConfig{Enabled: true, Token: "secret"}, logger)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reboot", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // wrong scheme
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for Basic auth, got %d", rec.Code)
	}
}

func TestDeviceInfoAuthFieldPopulated(t *testing.T) {
	// End-to-end: an Agent with auth enabled embeds
	// {auth:{required:true}} into its DeviceInfo and the JSON wire
	// payload reaches a real HTTP client.
	a, err := New(Config{
		DeviceID:   "test",
		ListenAddr: "127.0.0.1:0",
		Auth:       AuthConfig{Enabled: true, Token: "secret"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.SetInfo(protocol.DeviceInfo{SchemaVersion: protocol.SchemaVersion, DeviceID: "test"})

	info := a.Info()
	if info.Auth == nil || !info.Auth.Required {
		t.Fatalf("expected Auth.Required=true, got %+v", info.Auth)
	}

	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"auth":{"required":true}`) {
		t.Errorf("expected auth field in /api/v1/info response, got: %s", body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("decode: %v", err)
	}
}
