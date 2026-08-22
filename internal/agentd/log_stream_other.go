//go:build !linux

package agentd

import (
	"context"
	"errors"
	"io"
)

// startJournalctl is the non-Linux stub. The agent only runs on Linux
// (see cmd/agent/main.go's //go:build linux), so requests reaching this
// stub indicate a build misconfiguration. Defined here so the package
// compiles on macOS/Windows for local test runs of cross-platform
// helpers.
var startJournalctl = func(_ context.Context, _ string, _ int) (io.ReadCloser, func(), error) {
	return nil, nil, errors.New("journalctl only available on linux")
}
