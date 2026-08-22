package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// LogLine is a single record from the device's execution log stream.
// Ts is RFC3339Nano (UTC), Line is the MESSAGE, Cursor is journalctl's
// __CURSOR (reserved for future resume — not consumed in this version).
type LogLine struct {
	Ts     string `json:"ts"`
	Line   string `json:"line"`
	Cursor string `json:"cursor"`
}

// journalRecord is journalctl --output=json's wire format. Only the
// three fields we surface are kept; everything else is discarded.
type journalRecord struct {
	RealTimeUs string `json:"__REALTIME_TIMESTAMP"` // microseconds, decimal string
	Message    string `json:"MESSAGE"`
	Cursor     string `json:"__CURSOR"`
}

// StreamDeviceLogs opens a long-lived GET /api/v1/logs against the
// device and invokes onLine for each NDJSON record. Returns when ctx
// is cancelled, the stream ends, or any read/decode error occurs.
// Malformed lines are skipped (not fatal).
func (s *Scanner) StreamDeviceLogs(ctx context.Context, ip string, port int, onLine func(LogLine)) error {
	target := fmt.Sprintf("http://%s:%d/api/v1/logs?tail=100", ip, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/x-ndjson") // 文档化用；agent 不强制校验

	resp, err := s.opts.LogHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// 继续读取流
	case http.StatusForbidden:
		return fmt.Errorf("log streaming disabled")
	default:
		return fmt.Errorf("log stream: unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1<<20) // 1MB cap per line
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var rec journalRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			// best-effort：跳过坏行
			continue
		}
		line := LogLine{
			Ts:     formatJournalTs(rec.RealTimeUs),
			Line:   rec.Message,
			Cursor: rec.Cursor,
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		onLine(line)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

// formatJournalTs converts journalctl's __REALTIME_TIMESTAMP
// (microseconds since epoch, decimal string) to RFC3339Nano UTC.
func formatJournalTs(us string) string {
	if us == "" {
		return ""
	}
	n, err := strconv.ParseInt(us, 10, 64)
	if err != nil {
		return ""
	}
	return time.UnixMicro(n).UTC().Format(time.RFC3339Nano)
}
