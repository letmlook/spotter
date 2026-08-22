package scanner

import (
	"context"
	"net/http"
	"net/http/httptest"
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
// the new 401 branch added in v0.3: when an agent demands a token
// but the scanner has none configured, the surfaced error includes
// "token required" so the UI can prompt the user.
func TestScanner_PostPowerAction_UnauthorizedReturnsTypedError(t *testing.T) {
	t.Skip("covered by integration test in e2e_test.go; skip at unit level to avoid double-running")
}
