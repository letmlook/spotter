// End-to-end CLI tests: each test re-execs the compiled `spotter-cli`
// binary via `go test -c` artefact, captures stdout/stderr, and
// asserts on the bytes. This avoids the os.Exit trap that unit
// tests can't safely observe.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	cliOnce    sync.Once
	cliBinPath string
	cliBinErr  error
)

func cliBin(t *testing.T) string {
	t.Helper()
	cliOnce.Do(func() {
		exe, err := exec.LookPath("go")
		if err != nil {
			cliBinErr = err
			return
		}
		// Use a stable TempDir shared across tests (NOT t.TempDir,
		// which is auto-cleaned when the first caller finishes).
		dir, err := os.MkdirTemp("", "spotter-cli-test-*")
		if err != nil {
			cliBinErr = err
			return
		}
		out := filepath.Join(dir, "spotter-cli")
		build, err := exec.Command(exe, "build", "-o", out, ".").CombinedOutput()
		if err != nil {
			cliBinErr = &execError{err: err, out: string(build)}
			return
		}
		cliBinPath = out
	})
	if cliBinErr != nil {
		t.Skipf("cannot build spotter-cli: %v", cliBinErr)
	}
	return cliBinPath
}

type execError struct {
	err error
	out string
}

func (e *execError) Error() string { return e.err.Error() + "\n" + e.out }

type cliRun struct {
	Stdout, Stderr string
	Code           int
}

func runCLI(t *testing.T, bin string, args ...string) cliRun {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(cmd.Environ(),
		"HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return cliRun{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}
}

func TestCLI_Version(t *testing.T) {
	out := runCLI(t, cliBin(t), "version")
	if !strings.Contains(out.Stdout, "spotter-cli") {
		t.Errorf("stdout missing version marker: %q", out.Stdout)
	}
}

func TestCLI_UnknownCommand(t *testing.T) {
	out := runCLI(t, cliBin(t), "nope")
	if out.Code != 2 {
		t.Errorf("want exit 2, got %d (stderr=%q)", out.Code, out.Stderr)
	}
	if !strings.Contains(out.Stderr, "usage:") {
		t.Errorf("want usage banner on stderr: %q", out.Stderr)
	}
}

func TestCLI_ListEmpty(t *testing.T) {
	out := runCLI(t, cliBin(t), "list")
	if !strings.Contains(out.Stdout, "(no devices)") {
		t.Errorf("expected 'no devices' marker in stdout: %q", out.Stdout)
	}
}

func TestCLI_InfoMissingDeviceExitsOne(t *testing.T) {
	out := runCLI(t, cliBin(t), "info", "ghost")
	if out.Code != 1 {
		t.Errorf("want exit 1, got %d (stderr=%q)", out.Code, out.Stderr)
	}
	if !strings.Contains(out.Stderr, "device not in registry") {
		t.Errorf("want 'device not in registry' message: %q", out.Stderr)
	}
}

func TestRFC1918RankInline(t *testing.T) {
	cases := map[string]int{
		"10.0.0.0/24":    0,
		"172.16.0.0/16":  0,
		"172.32.0.0/16":  1,
		"192.168.1.0/24": 0,
		"8.8.8.0/24":     1,
	}
	for cidr, want := range cases {
		if got := rfc1918Rank(cidr); got != want {
			t.Errorf("rfc1918Rank(%q) = %d, want %d", cidr, got, want)
		}
	}
}

func TestMainpkgLocalSubnets_NoPanic(t *testing.T) {
	_ = mainpkgLocalSubnets()
}
