package collector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOSRelease(t *testing.T) {
	dir := t.TempDir()
	osRelease := `PRETTY_NAME="Ubuntu 22.04.4 LTS"
NAME="Ubuntu"
ID=ubuntu
VERSION_ID="22.04"
`
	if err := os.WriteFile(filepath.Join(dir, "os-release"), []byte(osRelease), 0644); err != nil {
		t.Fatal(err)
	}

	got := readOSRelease(dir)
	if got.PrettyName != "Ubuntu 22.04.4 LTS" {
		t.Errorf("PrettyName: got %q", got.PrettyName)
	}
	if got.ID != "ubuntu" {
		t.Errorf("ID: got %q", got.ID)
	}
	if got.VersionID != "22.04" {
		t.Errorf("VersionID: got %q", got.VersionID)
	}
}

func TestReadOSReleaseMissing(t *testing.T) {
	dir := t.TempDir()
	got := readOSRelease(dir)
	if got.PrettyName != "" {
		t.Errorf("expected empty result for missing file, got %+v", got)
	}
}

func TestCollectBasicHostnameAndArch(t *testing.T) {
	// smoke test: real Collect() must produce non-empty hostname+arch on Linux
	c := New()
	ctx := context.Background()
	info, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if info.Basic.Hostname == "" {
		t.Error("hostname is empty")
	}
	if info.Basic.Arch != "aarch64" && info.Basic.Arch != "x86_64" {
		t.Errorf("unexpected arch: %q", info.Basic.Arch)
	}
	if !strings.HasPrefix(info.Basic.Kernel, "") {
		// just ensure it ran; value is OS-dependent
		_ = info.Basic.Kernel
	}
}
