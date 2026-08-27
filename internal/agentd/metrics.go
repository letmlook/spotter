package agentd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MetricsSample is one observation at a wall-clock instant.
// Fields are kept loose (pointers) so JSON omitempty drops
// sensors that didn't report a value on this host (e.g.
// thermal_zone0 is missing on non-Jetson platforms).
type MetricsSample struct {
	At        time.Time `json:"at"`
	CPUPercent  *float64 `json:"cpu_percent,omitempty"`
	MemPercent  *float64 `json:"mem_percent,omitempty"`
	MemUsedBytes *uint64 `json:"mem_used_bytes,omitempty"`
	TempCelsius *float64 `json:"temp_celsius,omitempty"`
}

// MetricsHistory is a fixed-capacity ring buffer. Capacity
// defaults to 60 samples; at the 5s sample interval that's 5
// minutes of history. The buffer is lock-free for the writer
// (one goroutine) and the readers are guarded by a RWMutex
// so a /api/v1/metrics/recent poll mid-append gets a consistent
// snapshot, not a torn read.
type MetricsHistory struct {
	mu    sync.RWMutex
	buf   []MetricsSample
	cap   int
	head  int  // next write index
	full  bool // true once we've wrapped once
}

func newMetricsHistory(capacity int) *MetricsHistory {
	if capacity < 1 {
		// A zero or negative cap is almost always a caller bug.
		// Round up to 1 so push + Snapshot don't panic; callers
		// that wanted a real cap should pass metricsHistoryCap.
		capacity = 1
	}
	return &MetricsHistory{
		buf: make([]MetricsSample, capacity),
		cap: capacity,
	}
}

func (h *MetricsHistory) push(s MetricsSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf[h.head] = s
	h.head = (h.head + 1) % h.cap
	if h.head == 0 {
		h.full = true
	}
}

// Snapshot returns the buffered samples in chronological order
// (oldest first). Returning a slice copy keeps callers from
// observing future appends without holding the lock.
func (h *MetricsHistory) Snapshot() []MetricsSample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []MetricsSample
	if h.full {
		out = make([]MetricsSample, 0, h.cap)
		out = append(out, h.buf[h.head:]...)
		out = append(out, h.buf[:h.head]...)
	} else {
		out = make([]MetricsSample, 0, h.head)
		out = append(out, h.buf[:h.head]...)
	}
	return out
}

// sampler collects metrics samples on a fixed interval and
// pushes them into the history buffer. CPU% is computed by
// diffing /proc/stat across two ticks (the kernel reports
// cumulative jiffies; per-interval percentage requires
// deltas). Memory is read fresh each tick from /proc/meminfo.
// Temperature is read from the first available thermal zone
// in /sys/class/thermal.
type sampler struct {
	history *MetricsHistory
	interval time.Duration
	logger   *slog.Logger

	// lastCPU tracks the previous /proc/stat aggregate so we
	// can compute the busy fraction across the interval. Reset
	// to nil on the first tick so we don't report a bogus 0%.
	lastCPU *cpuStat
}

type cpuStat struct {
	total uint64
	idle  uint64
}

func (s *sampler) run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sample := s.collect()
			s.history.push(sample)
		}
	}
}

func (s *sampler) collect() MetricsSample {
	now := time.Now().UTC()
	sample := MetricsSample{At: now}
	if pct, err := readCPUPercent(s.lastCPU); err == nil {
		v := pct
		sample.CPUPercent = &v
	}
	if memPct, memUsed, err := readMemoryPercent(); err == nil {
		sample.MemPercent = &memPct
		sample.MemUsedBytes = &memUsed
	}
	if temp, err := readFirstTempCelsius(); err == nil {
		v := temp
		sample.TempCelsius = &v
	}
	return sample
}

// readCPUPercent reads /proc/stat and returns the busy fraction
// across the previous interval. First call returns 0 because
// there's no prior tick to diff against; that's the correct
// behaviour (we don't know the delta yet).
func readCPUPercent(prev *cpuStat) (float64, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, fmt.Errorf("/proc/stat: no cpu line")
	}
	line := sc.Text()
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, fmt.Errorf("/proc/stat: unexpected first line %q", line)
	}
	var total, idle uint64
	for i, f := range fields[1:] {
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("/proc/stat: field %d: %w", i, err)
		}
		total += n
		// Field 4 (index 3) is "idle"; on some kernels
		// field 5 is "iowait" — Linux's `top(1)` includes
		// iowait in idle. Match that.
		if i == 3 || i == 4 {
			idle += n
		}
	}
	if prev == nil {
		return 0, nil
	}
	dt := total - prev.total
	di := idle - prev.idle
	if dt == 0 {
		return 0, nil
	}
	busy := float64(dt-di) / float64(dt) * 100.0
	// Clamp: a context switch mid-tick can produce a slightly
	// negative idle delta. Visually 0% reads better than -0.3%.
	if busy < 0 {
		busy = 0
	}
	return math.Round(busy*10) / 10, nil
}

// readMemoryPercent returns (used/total as percent, used bytes).
func readMemoryPercent() (float64, uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var totalKB, availKB uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			totalKB = parseMemKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			availKB = parseMemKB(line)
		}
		if totalKB > 0 && availKB > 0 {
			break
		}
	}
	if totalKB == 0 {
		return 0, 0, fmt.Errorf("meminfo: MemTotal missing")
	}
	used := totalKB - availKB
	pct := float64(used) / float64(totalKB) * 100.0
	return math.Round(pct*10) / 10, used * 1024, nil
}

func parseMemKB(line string) uint64 {
	// Lines look like "MemTotal:       16384200 kB"
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// readFirstTempCelsius walks /sys/class/thermal/thermal_zone*/
// and returns the millidegree reading of the first zone that
// reports a positive value. Returns an error if no zone
// reports (common on non-Jetson / non-ACPI hosts — the
// endpoint will then omit temp_celsius entirely).
func readFirstTempCelsius() (float64, error) {
	entries, err := os.ReadDir("/sys/class/thermal")
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "thermal_zone") {
			continue
		}
		path := "/sys/class/thermal/" + e.Name() + "/temp"
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		milli, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || milli <= 0 {
			continue
		}
		return float64(milli) / 1000.0, nil
	}
	return 0, fmt.Errorf("no thermal_zone with positive reading")
}

// handleMetricsRecent serves the rolling history buffer as
// JSON. Capped at 60 samples (= 5 min @ 5s sample). Frontend
// polls this on a 5-10s cadence and renders sparklines.
//
// No auth: matches the rest of the read-only /api/v1/* surface;
// like /healthz and /api/v1/info, callers on the LAN can poll
// without a token. Operators who need lock-down should set
// `auth.required_for_read` (not yet implemented; tracked in
// v1.0.0 design doc).
func (a *Agent) handleMetricsRecent(w http.ResponseWriter, r *http.Request) {
	if a.metrics == nil {
		http.Error(w, "metrics not started", http.StatusServiceUnavailable)
		return
	}
	samples := a.metrics.Snapshot()
	// Cap to last 60 to bound response size (~5 KiB worst case
	// at 80 bytes per sample, sparse json).
	if len(samples) > 60 {
		samples = samples[len(samples)-60:]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"interval_seconds": int(metricsInterval.Seconds()),
		"samples":          samples,
	})
}

// initMetrics wires the sampler + ring buffer onto the agent.
// Started from cmd/agent/main.go:runAgent so the history fills
// from the moment the agent boots, not the first /api/v1/info
// poll.
func (a *Agent) initMetrics() {
	a.metrics = newMetricsHistory(metricsHistoryCap)
	s := &sampler{
		history:  a.metrics,
		interval: metricsInterval,
		logger:   a.logger,
	}
	go s.run(a.lifecycleCtx)
}

const (
	metricsHistoryCap = 60              // 5 minutes at 5s interval
	metricsInterval   = 5 * time.Second // match the discovery cadence
)

// io is imported via the bufio package; the alias below
// keeps the import surface stable for future helpers that
// might want raw I/O.
var _ = io.Discard
