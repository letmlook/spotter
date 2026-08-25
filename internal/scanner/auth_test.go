package scanner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/spotter/spotter/internal/registry"
)

// TestScanner_NewRequest_StampsAuthHeader verifies that any HTTP
// request built via Scanner.newRequest carries Authorization when
// AuthToken is set. The downstream paths (poll / probe / power /
// log stream) all funnel through this helper.
func TestScanner_NewRequest_StampsAuthHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	r, err := registry.Open(t.TempDir() + "/x.json")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	s := New(r, WithAuthToken("s3cr3t"))
	req, err := s.newRequest(context.Background(), http.MethodGet, srv.URL+"/api/v1/info")
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "Bearer s3cr3t" {
		t.Errorf("Authorization missing on req: %q", req.Header.Get("Authorization"))
	}
	// Round-trip via the test server to make sure the wire also carries
	// the header.
	resp, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.HasPrefix(got, "Bearer ") {
		t.Errorf("server saw Authorization: %q", got)
	}
	if got != "Bearer s3cr3t" {
		t.Errorf("expected Bearer s3cr3t, got %q", got)
	}
}

func TestScanner_NewRequest_NoHeaderWhenTokenEmpty(t *testing.T) {
	r, _ := registry.Open(t.TempDir() + "/x.json")
	defer r.Close()

	s := New(r) // no WithAuthToken
	req, err := s.newRequest(context.Background(), http.MethodGet, "http://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if h := req.Header.Get("Authorization"); h != "" {
		t.Errorf("expected empty Authorization, got %q", h)
	}
}

// TestScanner_PostPowerAction_UnauthorizedReturnsTypedError exercises
// the 401 branch added in v0.3: when an agent demands a token but
// the scanner has none configured, the surfaced error includes
// "token required" so the UI can prompt the user. (Previously a
// t.Skip stub citing e2e_test.go; no such test existed.)
func TestScanner_PostPowerAction_UnauthorizedReturnsTypedError(t *testing.T) {
	r, _ := registry.Open(t.TempDir() + "/x.json")
	defer r.Close()

	// Stand up a fake spotterd that demands a token we will not send.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"token required"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	s := New(r) // no WithAuthToken

	// Parse srv.URL into host + port so we can call the public
	// RebootDevice / ShutdownDevice entry points (the production
	// path the UI uses).
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse test server port from %q: %v", srv.URL, err)
	}

	for _, call := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{"reboot", func(ctx context.Context) error { return s.RebootDevice(ctx, host, port) }},
		{"shutdown", func(ctx context.Context) error { return s.ShutdownDevice(ctx, host, port) }},
	} {
		err := call.fn(context.Background())
		if err == nil {
			t.Errorf("%s: want error from 401, got nil", call.name)
			continue
		}
		// The user-facing message must include "token required" so
		// the UI's prompt is unambiguous.
		if !strings.Contains(err.Error(), "token required") {
			t.Errorf("%s: error %q does not mention 'token required'", call.name, err.Error())
		}
	}
}
