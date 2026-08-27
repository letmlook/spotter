package agentd

import "strings"

// JournalctlOpts is the parsed-and-validated parameter set the
// handler hands to startJournalctl. The fields mirror the query
// params /api/v1/logs accepts; see docs/api.md for the wire
// shape. This file is build-tag-free so the type is visible on
// every platform (the Linux implementation in log_stream_linux.go
// and the non-Linux stub in log_stream_other.go both reference it).
type JournalctlOpts struct {
	// Units is the list of systemd units to follow. Empty means
	// "all units" (system-wide journal), which is allowed but
	// the default unit from config is layered in front so the
	// typical "watch the agent" call still works without
	// ?unit=. The handler is responsible for the layering; this
	// struct is the raw, post-parse view.
	Units []string
	// Tail is the number of historical lines to replay before
	// following new lines. Defaults to defaultLogTail; capped
	// at MaxTail.
	Tail int
	// Grep is a case-sensitive regular expression filter.
	// Empty means no filter. Passed as --grep=...
	Grep string
	// Since is a free-form journalctl time spec like
	// "5min ago", "2026-08-27 12:00:00", or "yesterday".
	// Empty means "from now onwards" (with tail providing
	// pre-context). Passed as --since=...
	Since string
	// Priority restricts to <=N severity. journalctl uses
	// 0=emerg, 3=err, 4=warning, 6=info, 7=debug. Empty
	// means no priority filter.
	Priority string
}

// splitUnits parses a comma-separated ?unit= value. Whitespace
// is trimmed; empty entries are dropped. A blank input returns
// nil so the caller can distinguish "no override" from "explicit
// empty" (though the handler treats both the same).
func splitUnits(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
