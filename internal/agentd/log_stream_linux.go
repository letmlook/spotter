//go:build linux

package agentd

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
)

// startJournalctl invokes journalctl in follow mode and returns a
// ReadCloser of its stdout plus a kill callback. Returns an error
// when journalctl is missing or fails to start. The package-level
// variable is overridable in tests.
var startJournalctl = func(ctx context.Context, opts JournalctlOpts) (io.ReadCloser, func(), error) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, nil, err
	}
	args := []string{"--no-pager", "--output=json", "-f"}
	for _, u := range opts.Units {
		if u == "" {
			continue
		}
		args = append(args, "-u", u)
	}
	if opts.Tail > 0 {
		args = append(args, "-n", strconv.Itoa(opts.Tail))
	}
	if opts.Grep != "" {
		args = append(args, "--grep", opts.Grep)
	}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}
	if opts.Priority != "" {
		args = append(args, "--priority", opts.Priority)
	}
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
			if err := cmd.Process.Kill(); err != nil {
				slog.Default().Debug("journalctl kill failed",
					"err", err.Error())
			}
		}
		// 回收 Wait，防止僵尸；异步避免阻塞 kill 调用方。
		go func() {
			if err := cmd.Wait(); err != nil {
				slog.Default().Debug("journalctl wait failed",
					"err", err.Error())
			}
		}()
	}
	return stdout, kill, nil
}
