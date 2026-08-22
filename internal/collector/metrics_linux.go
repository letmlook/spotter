//go:build linux

package collector

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/spotter/spotter/internal/protocol"
)

// collectMetrics gathers lightweight per-process system counters:
// CPU seconds across the system, memory totals, and CPU temperature
// (Jetson or generic thermal_zone). Returns nil when none of the
// metrics could be read (e.g. a non-Linux build, but this file is
// Linux-tagged so nil indicates a permission error on /proc).
func collectMetrics(ctx context.Context) *protocol.Metrics {
	m := &protocol.Metrics{}
	any := false

	if cpu := readCPUSeconds(); cpu != nil {
		m.CPUSecondsTotal = cpu
		any = true
	}
	if mem := readMemInfo(); mem != nil {
		total := mem.Total
		avail := mem.Available
		m.MemTotalBytes = &total
		m.MemAvailableBytes = &avail
		any = true
	}
	if t := readCPUTempC(); t != nil {
		temp := *t
		m.CPUTempC = &temp
		any = true
	}
	if !any {
		return nil
	}
	return m
}

// readCPUSeconds reads /proc/stat cpu line: total time spent in all
// states since boot. We expose the cumulative seconds so the client
// can render a sparkline after dividing by uptime.
func readCPUSeconds() *float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil
		}
		var sum uint64
		for _, f := range fields[1:] {
			n, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				continue
			}
			sum += n
		}
		// Linux ticks at USER_HZ (typically 100). Seconds = ticks/100.
		f := float64(sum) / 100.0
		return &f
	}
	return nil
}

type memInfo struct {
	Total, Available uint64
}

func readMemInfo() *memInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil
	}
	var m memInfo
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Values are in kB.
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			m.Total = kb * 1024
		case "MemAvailable:":
			m.Available = kb * 1024
		}
	}
	if m.Total == 0 {
		return nil
	}
	return &m
}

// readCPUTempC reads the first thermal_zone that reports a CPU
// temperature. Returns centigrade (int) or nil if no zone readable.
func readCPUTempC() *int {
	entries, err := os.ReadDir("/sys/class/thermal")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		raw, err := os.ReadFile("/sys/class/thermal/" + e.Name() + "/temp")
		if err != nil {
			continue
		}
		milli, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			continue
		}
		c := milli / 1000
		return &c
	}
	return nil
}
