package scanner

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spotter/spotter/internal/registry"
)

// TestWithAuthToken_StampsBearerHeader asserts that the
// scanner's newRequest stamps `Authorization: Bearer <token>`
// when WithAuthToken is set. Without this, an agent deployed
// with [auth].enabled would reject every poll with 401.
func TestWithAuthToken_StampsBearerHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":2,"device_id":"a","basic":{"hostname":"a"}}`))
	}))
	defer srv.Close()

	reg, _ := registry.Open(t.TempDir() + "/r.json")
	defer reg.Close()
	_ = reg.Add(registry.Entry{
		DeviceID: "authed",
		IP:       "127.0.0.1",
		Port:     srv.Listener.Addr().(*net.TCPAddr).Port,
	})

	s := New(reg, WithAuthToken("s3cr3t"))
	s.PollOnce(context.Background())

	if got != "Bearer s3cr3t" {
		t.Errorf("Authorization header = %q, want %q", got, "Bearer s3cr3t")
	}
}

// TestWithHTTPClient_OverridesDefault verifies that
// WithHTTPClient swaps in a custom client for the scanner's
// poll paths. This is the seam tests + integration suites
// use to inject a client with a custom Transport (for TLS
// pinning, custom timeout, mock responses, etc).
func TestWithHTTPClient_OverridesDefault(t *testing.T) {
	called := atomic.Int64{}
	custom := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			called.Add(1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"schema_version":2,"device_id":"x","basic":{"hostname":"x"}}`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}

	reg, _ := registry.Open(t.TempDir() + "/r.json")
	defer reg.Close()
	_ = reg.Add(registry.Entry{DeviceID: "x", IP: "127.0.0.1", Port: 9999})

	s := New(reg, WithHTTPClient(custom))
	s.PollOnce(context.Background())

	if called.Load() == 0 {
		t.Errorf("custom Transport never called — WithHTTPClient did not apply")
	}
}

// roundTripperFunc adapts a plain function to http.RoundTripper
// so tests don't need a full http.Transport.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
