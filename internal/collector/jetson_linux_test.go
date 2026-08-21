//go:build linux

package collector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spotter/spotter/internal/protocol"
)

func TestCollectJetsonFromDeviceTree(t *testing.T) {
	// Build a fake root with /etc/nv_tegra_release + /proc/device-tree/model
	// + /sys/firmware/devicetree/base/serial-number
	root := t.TempDir()
	must := func(p, c string) {
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, "etc/nv_tegra_release"), "# R35 (release), REVISION: 5.0, GCID: 35550185\n")
	must(filepath.Join(root, "proc/device-tree/model"), "NVIDIA Jetson Orin Nano Developer Kit")
	must(filepath.Join(root, "sys/firmware/devicetree/base/serial-number"), "1420921088123")

	info := collectJetsonFromRoot(root)
	if info == nil {
		t.Fatal("expected non-nil JetsonInfo")
	}
	if info.L4T == "" {
		t.Error("L4T empty")
	}
	if info.Model != "NVIDIA Jetson Orin Nano Developer Kit" {
		t.Errorf("Model: got %q", info.Model)
	}
	if info.Serial != "1420921088123" {
		t.Errorf("Serial: got %q", info.Serial)
	}
}

func TestCollectJetsonNoJetson(t *testing.T) {
	root := t.TempDir() // empty
	info := collectJetsonFromRoot(root)
	if info != nil {
		t.Errorf("expected nil JetsonInfo for non-Jetson root, got %+v", info)
	}
}

func TestJetsonInfoPartialIsValid(t *testing.T) {
	// Only serial present -> still a valid JetsonInfo with one field
	root := t.TempDir()
	must := func(p, c string) {
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		_ = os.WriteFile(p, []byte(c), 0644)
	}
	must(filepath.Join(root, "sys/firmware/devicetree/base/serial-number"), "9999")
	info := collectJetsonFromRoot(root)
	if info == nil || info.Serial != "9999" {
		t.Errorf("partial JetsonInfo should be returned, got %+v", info)
	}
	_ = context.Background
	_ = protocol.SchemaVersion
}
