// Package timefmt centralises the timestamp formatting the agent
// and client stamp on every wire payload. Before this package the
// literal `time.Now().UTC().Format(time.RFC3339)` was inlined in
// 11+ files (main.go × 5, internal/collector/basic_linux.go,
// internal/agentd/{agent.go, power.go ×2, udp.go}, etc.). When the
// format moves — RFC3339Nano is already used by log_stream.go —
// every call site has to be hunted down by hand. Two private
// wrappers (`nowUTC`, `timeNowUTC`) existed inside scanner and
// collector respectively but neither was importable from the
// agent.
package timefmt

import "time"

// NowUTC returns the current time in UTC, formatted as RFC3339
// (e.g. "2026-08-25T22:00:00Z"). Centralised so the agent and
// client agree on the wire format; the previous inline literal
// was both duplicated and silently drifted to RFC3339Nano in
// log_stream.go.
func NowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
