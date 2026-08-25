//go:build linux

package main

import (
	"testing"

	"github.com/spotter/spotter/internal/lanscan"
)

// TestApplyConfigDefaults verifies that applyConfigDefaults
// fills in the three defaults the operator commonly leaves
// blank (ListenAddr / MulticastGroup / AgentVersion). Without
// these, the agent would fail to bind or broadcast on a
// vanilla install. This test is linux-only because the cmd/
// package is `//go:build linux`; full-runAgent integration
// tests live in the runner pipeline (CI Linux) since they
// fork+exec the linux binary.
func TestApplyConfigDefaults(t *testing.T) {
	cfg := tomlConfig{DeviceID: "x"} // the others blank
	applyConfigDefaults(&cfg)
	if cfg.ListenAddr == "" {
		t.Errorf("ListenAddr empty after applyConfigDefaults")
	}
	if cfg.MulticastGroup == "" {
		t.Errorf("MulticastGroup empty after applyConfigDefaults")
	}
	if cfg.AgentVersion == "" {
		t.Errorf("AgentVersion empty after applyConfigDefaults")
	}
	if cfg.DeviceID != "x" {
		t.Errorf("DeviceID = %q, want x (must not be touched)", cfg.DeviceID)
	}
}

// keep references so unused-import detection does not flag.
var _ = lanscan.LocalSubnets
