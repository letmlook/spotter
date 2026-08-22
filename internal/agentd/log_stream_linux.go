//go:build linux

package agentd

import (
	"context"
	"io"
	"os/exec"
	"strconv"
)

// startJournalctl invokes journalctl in follow mode and returns a
// ReadCloser of its stdout plus a kill callback. Returns an error
// when journalctl is missing or fails to start. The package-level
// variable is overridable in tests.
var startJournalctl = func(ctx context.Context, unit string, tail int) (io.ReadCloser, func(), error) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, nil, err
	}
	args := []string{"-u", unit, "--no-pager", "--output=json", "-n", strconv.Itoa(tail), "-f"}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	kill := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		// 回收 Wait，防止僵尸；异步避免阻塞 kill 调用方。
		go func() { _ = cmd.Wait() }()
	}
	return stdout, kill, nil
}
