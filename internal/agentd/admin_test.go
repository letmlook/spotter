package agentd

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/spotter/spotter/internal/protocol"
)

// makeTestAgent constructs an Agent with a fixed DeviceInfo
// and a known auth token (or no token if empty). The test
// exercises the admin pages without needing collectors.
func makeTestAgent(t *testing.T, token string) *Agent {
	t.Helper()
	a, err := New(Config{
		DeviceID:    "test-agent",
		ListenAddr:  "127.0.0.1:0",
		AgentVersion: "0.1.0-test",
		Auth:        AuthConfig{Enabled: token != "", Token: token},
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	a.info = protocol.DeviceInfo{
		SchemaVersion: 3,
		DeviceID:      "test-agent",
		CollectedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		AgentVersion:  "0.1.0-test",
		Basic: protocol.BasicInfo{
			Hostname:      "test-host",
			Username:      "tester",
			OS:            protocol.OSInfo{PrettyName: "Ubuntu 22.04", ID: "ubuntu", VersionID: "22.04"},
			Kernel:        "5.15.0-test",
			Arch:          "x86_64",
			UptimeSeconds: 12345,
		},
		Network: protocol.NetworkInfo{
			PrimaryIP: "10.0.0.42",
			Interfaces: []protocol.Interface{
				{Name: "eth0", MAC: "00:11:22:33:44:55", Addrs: []string{"10.0.0.42/24"}},
			},
		},
	}
	return a
}

// TestAdminIndex_Renders — happy path, no auth, page renders
// with the device hostname in the title.
func TestAdminIndex_Renders(t *testing.T) {
	a := makeTestAgent(t, "")
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body, _ := readAll(resp.Body)
	if !strings.Contains(string(body), "test-host") {
		t.Errorf("body missing hostname: %s", body)
	}
	if !strings.Contains(string(body), "v0.1.0-test") {
		t.Errorf("body missing version: %s", body)
	}
	if !strings.Contains(string(body), "test-agent") {
		t.Errorf("body missing device id: %s", body)
	}
}

// TestAdminIndex_RequiresAuth — with a token configured, the
// page must respond 401 to a no-Authorization request.
func TestAdminIndex_RequiresAuth(t *testing.T) {
	a := makeTestAgent(t, "secret-token")
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("WWW-Authenticate header missing")
	}
}

// TestAdminIndex_AcceptsValidToken — Basic auth with the
// matching token must return 200.
func TestAdminIndex_AcceptsValidToken(t *testing.T) {
	a := makeTestAgent(t, "secret-token")
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/admin", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte("ignored:secret-token"),
	))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestAdminIndex_RejectsWrongToken — wrong password returns
// 401, not 500 (so the operator's browser shows the auth
// prompt, not a server error).
func TestAdminIndex_RejectsWrongToken(t *testing.T) {
	a := makeTestAgent(t, "secret-token")
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/admin", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte("ignored:wrong-token"),
	))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// TestAdminStatic_CSSAvailable — the embedded CSS must be
// served with the right content type. Curl-friendly URLs
// like `/admin/static/style.css` should "just work".
func TestAdminStatic_CSSAvailable(t *testing.T) {
	a := makeTestAgent(t, "")
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/admin/static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ct)
	}
	body, _ := readAll(resp.Body)
	if !strings.Contains(string(body), ":root") {
		t.Errorf("body missing CSS root: %s", body)
	}
}

// TestAdminMetricsJSON_503WhenNoMetrics — when the metrics
// goroutine hasn't started, the JSON endpoint must return
// 503 (not 200 with empty array), so operators can
// distinguish "no samples yet" from "metrics are off".
func TestAdminMetricsJSON_503WhenNoMetrics(t *testing.T) {
	a := makeTestAgent(t, "")
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/admin/metrics.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

// TestAdminMetricsJSON_OKWithHistory — when the ring buffer
// has samples, JSON is emitted with the same shape as
// /api/v1/metrics/recent.
func TestAdminMetricsJSON_OKWithHistory(t *testing.T) {
	a := makeTestAgent(t, "")
	a.metrics = newMetricsHistory(metricsHistoryCap)
	a.metrics.push(MetricsSample{At: time.Now().UTC()})
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/admin/metrics.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, _ := readAll(resp.Body)
	if !strings.Contains(string(body), "interval_seconds") {
		t.Errorf("body missing interval_seconds: %s", body)
	}
	if !strings.Contains(string(body), "samples") {
		t.Errorf("body missing samples: %s", body)
	}
}

// TestAdminIndex_TrailingSlashAccepted — both /admin and
// /admin/ are valid; the trailing-slash form is what
// browser address bars produce after redirect.
func TestAdminIndex_TrailingSlashAccepted(t *testing.T) {
	a := makeTestAgent(t, "")
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/admin/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
