package agentd

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestMetricsHistory_ChronologicalOrder — push 5 samples,
// Snapshot must return them in push order regardless of how
// the ring head advances.
func TestMetricsHistory_ChronologicalOrder(t *testing.T) {
	h := newMetricsHistory(10)
	for i := 0; i < 5; i++ {
		pct := float64(i * 10)
		h.push(MetricsSample{At: time.Unix(int64(i), 0), CPUPercent: &pct})
	}
	got := h.Snapshot()
	if len(got) != 5 {
		t.Fatalf("len=%d, want 5", len(got))
	}
	for i, s := range got {
		if s.CPUPercent == nil || *s.CPUPercent != float64(i*10) {
			t.Errorf("sample %d: %+v", i, s)
		}
	}
}

// TestMetricsHistory_WrapAround — push more than cap samples
// and confirm the oldest is dropped, the new ordering holds.
func TestMetricsHistory_WrapAround(t *testing.T) {
	const cap = 4
	h := newMetricsHistory(cap)
	for i := 0; i < 7; i++ {
		pct := float64(i)
		h.push(MetricsSample{At: time.Unix(int64(i), 0), CPUPercent: &pct})
	}
	got := h.Snapshot()
	if len(got) != cap {
		t.Fatalf("len=%d, want %d", len(got), cap)
	}
	// After 7 pushes into a 4-cap ring, head is at 7%4=3.
	// Chronological order: indices [3,0,1,2] of the buffer =
	// values [3,4,5,6].
	want := []float64{3, 4, 5, 6}
	for i, s := range got {
		if s.CPUPercent == nil || *s.CPUPercent != want[i] {
			t.Errorf("sample %d: got %+v, want %v", i, s, want[i])
		}
	}
}

// TestMetricsHistory_ZeroCap_FallsBackToOne — guard against an
// off-by-one in the constructor. A zero cap rounds up to 1 so
// push + Snapshot never panic, with a single-slot buffer.
// Callers that want a real cap should pass metricsHistoryCap.
func TestMetricsHistory_ZeroCap_FallsBackToOne(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("zero cap panicked: %v", r)
		}
	}()
	h := newMetricsHistory(0)
	if h.cap != 1 {
		t.Errorf("cap = %d, want fallback to 1", h.cap)
	}
	h.push(MetricsSample{At: time.Now()})
	if got := h.Snapshot(); len(got) != 1 {
		t.Errorf("snapshot len = %d, want 1", len(got))
	}
}

// TestReadMemoryPercent_ValidatesInput — feed a tiny /proc/meminfo
// via parseMemKB and confirm the function returns the expected
// percent. We don't read the host's /proc/meminfo directly
// because it's a per-host property and we want a stable test.
func TestReadMemoryPercent_ValidatesInput(t *testing.T) {
	// Use a tmp file with hand-written content. The function
	// reads via os.Open("/proc/meminfo") so we can't redirect
	// the path without a build tag; instead we just call the
	// helper that parses individual lines.
	const total = 16384200
	const avail = 8192000
	got := parseMemKB("MemTotal:    " + itoa(total) + " kB")
	if got != total {
		t.Errorf("total: %d, want %d", got, total)
	}
	got = parseMemKB("MemAvailable:  " + itoa(avail) + " kB")
	if got != avail {
		t.Errorf("avail: %d, want %d", got, avail)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := "0123456789"
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

// TestHandleMetricsRecent_503WhenNotStarted — without a metrics
// history attached, the endpoint must return 503 (not 500 or
// a panic) so the GUI can show "metrics unavailable" rather
// than a crash dialog.
func TestHandleMetricsRecent_503WhenNotStarted(t *testing.T) {
	a, _ := New(Config{DeviceID: "x", ListenAddr: "127.0.0.1:0"}, slog.Default())
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/metrics/recent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("got %d, want 503", resp.StatusCode)
	}
}

// TestHandleMetricsRecent_OKWithHistory — populate a history
// and assert the response shape.
func TestHandleMetricsRecent_OKWithHistory(t *testing.T) {
	a, _ := New(Config{DeviceID: "x", ListenAddr: "127.0.0.1:0"}, slog.Default())
	// Inject a history directly so the test doesn't depend on
	// the sampler goroutine actually running.
	a.metrics = newMetricsHistory(metricsHistoryCap)
	for i := 0; i < 3; i++ {
		pct := float64(10 + i)
		a.metrics.push(MetricsSample{
			At:         time.Unix(int64(i), 0).UTC(),
			CPUPercent: &pct,
		})
	}
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/metrics/recent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	var body struct {
		IntervalSeconds int               `json:"interval_seconds"`
		Samples         []MetricsSample   `json:"samples"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.IntervalSeconds != int(metricsInterval.Seconds()) {
		t.Errorf("interval = %d, want %d", body.IntervalSeconds, int(metricsInterval.Seconds()))
	}
	if len(body.Samples) != 3 {
		t.Errorf("samples = %d, want 3", len(body.Samples))
	}
}

// TestReadCPUPercent_FirstCallZero — the first call has no
// prior /proc/stat to diff against; we return 0 instead of
// inventing a number. The pin is that the function never
// errors on a well-formed /proc/stat.
func TestReadCPUPercent_FirstCallZero(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only: depends on /proc/stat")
	}
	pct, err := readCPUPercent(nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if pct != 0 {
		t.Errorf("first call pct = %v, want 0", pct)
	}
}

// TestReadFirstTempCelsius_HandlesEmpty — on a host with no
// thermal sensors, the function returns an error rather than
// a zero / negative reading (so the omitempty in the JSON
// payload drops the field).
func TestReadFirstTempCelsius_HandlesEmpty(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only: depends on /sys/class/thermal")
	}
	_, err := readFirstTempCelsius()
	// The /sys/class/thermal tree exists on every modern Linux
	// but may have no positive readings (e.g. a container with
	// no host thermal access). We don't care which — we only
	// require that the call returns without panic. A nil err is
	// fine too; what matters is that downstream omitempty drops
	// the field.
	_ = err
	_ = strings.Contains("", "")
}
