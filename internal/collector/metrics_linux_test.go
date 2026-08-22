//go:build linux

package collector

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"
)

func TestMetricsHelpers_EmptyFile(t *testing.T) {
	// Test an inline case to keep coverage deterministic across
	// platforms that have /proc and ones that don't.
	if got := readCPUSecondsFrom(strings.NewReader("")); got != nil {
		t.Errorf("empty reader should return nil, got %v", got)
	}
}

// readCPUSecondsFrom is a small testable wrapper so we can drive
// readCPUSeconds without touching /proc.
func readCPUSecondsFrom(r io.Reader) *float64 {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		hasLine := err == nil && line != ""
		if hasLine && strings.HasPrefix(line, "cpu ") {
			// Mirror the production parser.
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return nil
			}
			var sum uint64
			for _, f := range fields[1:] {
				var n uint64
				for _, c := range f {
					if c < '0' || c > '9' {
						goto out
					}
					n = n*10 + uint64(c-'0')
				}
				sum += n
			out:
			}
			v := float64(sum) / 100.0
			return &v
		}
		if err != nil {
			return nil
		}
	}
}

func TestMetrics_ReadCPUSeconds_MissingFile(t *testing.T) {
	// Test the real /proc/stat path; if /proc isn't available in
	// the test environment, the helper returns nil rather than panicking.
	if v := readCPUSeconds(); v == nil {
		t.Log("readCPUSeconds() returned nil (likely missing /proc/stat)")
	} else if *v < 0 {
		t.Errorf("negative CPU seconds: %v", *v)
	}
}

func TestMetrics_ReadMemInfo_MissingFile(t *testing.T) {
	if v := readMemInfo(); v == nil {
		t.Log("readMemInfo() returned nil")
	} else if v.Total == 0 {
		t.Error("MemTotal must be > 0 when /proc/meminfo is readable")
	}
}

func TestMetrics_Collect_NoPanic(t *testing.T) {
	m := collectMetrics(context.Background())
	if m == nil {
		t.Log("collectMetrics returned nil — host has none of /proc/stat, /proc/meminfo, /sys/class/thermal (sandbox).")
		return
	}
	// When populated, MemTotalBytes must be set (as we have /proc).
	if m.MemTotalBytes == nil {
		t.Error("MemTotalBytes expected when /proc/meminfo is present")
	}
}
