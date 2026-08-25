package timefmt

import (
	"regexp"
	"testing"
	"time"
)

// TestNowUTC_Format pins the wire-format string the agent and
// client both rely on. The format is RFC3339 with the trailing
// 'Z' zone marker and second precision (not RFC3339Nano) —
// changing either side's format silently regresses every wire
// payload that has a timestamp field.
func TestNowUTC_Format(t *testing.T) {
	got := NowUTC()
	if len(got) < 20 {
		t.Errorf("NowUTC() = %q, want ≥20 chars (RFC3339 sec)", got)
	}
	// RFC3339 with `time.Now().UTC()` is `YYYY-MM-DDTHH:MM:SSZ`.
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	if !re.MatchString(got) {
		t.Errorf("NowUTC() = %q, want RFC3339 with trailing Z (no nanos)", got)
	}
}

// TestNowUTC_UTC asserts the output is in UTC. Without the
// UTC() conversion the timestamp would carry the host's local
// zone (e.g. +08:00) which the client cannot reliably parse.
func TestNowUTC_UTC(t *testing.T) {
	// Run against a non-UTC location to confirm the .UTC() call
	// is honored. Skip if the host rejects the assignment.
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("America/Los_Angeles tz not available: %v", err)
	}
	prev := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = prev })

	got := NowUTC()
	if !regexp.MustCompile(`Z$`).MatchString(got) {
		t.Errorf("NowUTC() = %q, want trailing Z even when Local is non-UTC", got)
	}
}
