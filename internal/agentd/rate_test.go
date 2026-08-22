package agentd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimit_NilLimiterPassesThrough(t *testing.T) {
	// When no limiter is configured, requests pass through cleanly.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	mw := rateLimitMiddleware(next, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("want pass-through, got %d", rec.Code)
	}
}

func TestRateLimit_BlocksExcessPerIP(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	// rps=1, burst=2 → 3rd call within the same second gets 429.
	mw := rateLimitMiddleware(next, newIPLimiter(1, 2))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.7:55555"
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusTeapot {
			t.Errorf("burst %d: want pass-through, got %d", i, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.7:55555"
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on 429")
	}
}

func TestRateLimit_DifferentIPsBurstIndependently(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	mw := rateLimitMiddleware(next, newIPLimiter(1, 1))
	for _, ip := range []string{"10.0.0.1:1", "10.0.0.2:1", "10.0.0.3:1"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
		if rec.Code != http.StatusTeapot {
			t.Errorf("ip %s: want pass, got %d", ip, rec.Code)
		}
	}
}
