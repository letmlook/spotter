//go:build linux

package collector

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestParseJetsonReleaseLegacyFormat covers the pre-7.x flat layout
// where every line was a top-level "Key: Value". Must still work to
// preserve backwards compatibility with older jetpack-stats installs.
func TestParseJetsonReleaseLegacyFormat(t *testing.T) {
	in := `Model: NVIDIA Jetson Orin Nano Developer Kit
Jetpack: 5.1.3
L4T: 35.5.0
CUDA: 11.4.315
cuDNN: 8.6.0
TensorRT: 8.5
Python: 3.8.10
`
	got := parseJetsonRelease(in)
	want := &protocol.JetsonInfo{
		Model:    "NVIDIA Jetson Orin Nano Developer Kit",
		Jetpack:  "5.1.3",
		L4T:      "35.5.0",
		CUDA:     "11.4.315",
		CUDNN:    "8.6.0",
		TensorRT: "8.5",
		Python:   "3.8.10",
	}
	if !equalJetson(got, want) {
		t.Errorf("legacy parse mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// TestParseJetsonReleaseJetpackStats7 covers the jetpack-stats 7.x
// grouped indented layout with ANSI SGR colour codes interleaved.
// Reproduces exactly what fitow@10.10.9.165 (Jetson AGX Thor,
// jetpack-stats 7.2.1) prints in `jetson_release -v`.
func TestParseJetsonReleaseJetpackStats7(t *testing.T) {
	// Verbatim output from fitow (with literal ESC byte), as observed
	// on 2026-08-24. We use "\x1b" so the test fixture stays readable.
	in := "\x1b[1mModel:\x1b[0m \x1b[1mNVIDIA Jetson AGX Thor Developer Kit\x1b[0m - Jetpack \x1b[1m7.2 GA\x1b[0m [\x1b[1mL4T \x1b[0m\x1b[1m39.2.0\x1b[0m]\n" +
		"\x1b[92;1mNV Power Mode\x1b[0m\x1b[0m[\x1b[1m1\x1b[0m]: \x1b[1m120W\x1b[0m\n" +
		"\x1b[92;1mSerial Number:\x1b[0m\x1b[0m [XXX Show with: jetson_release -s XXX]\n" +
		"\x1b[92;1mHardware:\x1b[0m\x1b[0m\n" +
		" - \x1b[1m699-level Part Number\x1b[0m: 699-13834-0008-400 H.1\n" +
		" - \x1b[1mP-Number\x1b[0m: p3834-0008\n" +
		" - \x1b[1mModule\x1b[0m: NVIDIA Jetson AGX Thor (Developer kit)\n" +
		" - \x1b[1mSoC\x1b[0m: tegra264\n" +
		" - \x1b[1mCUDA Arch BIN\x1b[0m: 11.0\n" +
		"\x1b[92;1mPlatform:\x1b[0m\x1b[0m\n" +
		" - \x1b[1mMachine\x1b[0m: aarch64\n" +
		" - \x1b[1mSystem\x1b[0m: Linux\n" +
		" - \x1b[1mDistribution\x1b[0m: Ubuntu 24.04 Noble Numbat\n" +
		" - \x1b[1mRelease\x1b[0m: 6.8.12-1021-tegra\n" +
		" - \x1b[1mPython\x1b[0m: 3.12.3\n" +
		"\x1b[92;1mjtop:\x1b[0m\x1b[0m\n" +
		" - \x1b[1mVersion\x1b[0m: 7.2.1\n" +
		" - \x1b[1mService\x1b[0m: \x1b[92;1mActive\x1b[0m\x1b[0m\n" +
		"\x1b[92;1mLibraries:\x1b[0m\x1b[0m\n" +
		" - \x1b[1mCUDA\x1b[0m: 13.2.86\n" +
		" - \x1b[1mcuDNN\x1b[0m: 9.20.0\n" +
		" - \x1b[1mTensorRT\x1b[0m: 10.16.2.10\n" +
		" - \x1b[1mVPI\x1b[0m: 4.1.4.0\n" +
		" - \x1b[1mVulkan\x1b[0m: 1.4.321\n" +
		" - \x1b[1mOpenCV\x1b[0m: 4.8.0 - with CUDA: \x1b[91mNO\x1b[0m\n"

	got := parseJetsonRelease(in)

	if got.Model != "NVIDIA Jetson AGX Thor Developer Kit" {
		t.Errorf("Model: got %q want %q", got.Model, "NVIDIA Jetson AGX Thor Developer Kit")
	}
	if got.Jetpack != "7.2 GA" {
		t.Errorf("Jetpack: got %q want %q", got.Jetpack, "7.2 GA")
	}
	if got.L4T != "39.2.0" {
		t.Errorf("L4T: got %q want %q", got.L4T, "39.2.0")
	}
	if got.CUDA != "13.2.86" {
		t.Errorf("CUDA: got %q want %q", got.CUDA, "13.2.86")
	}
	if got.CUDNN != "9.20.0" {
		t.Errorf("cuDNN: got %q want %q", got.CUDNN, "9.20.0")
	}
	if got.TensorRT != "10.16.2.10" {
		t.Errorf("TensorRT: got %q want %q", got.TensorRT, "10.16.2.10")
	}
	if got.Python != "3.12.3" {
		t.Errorf("Python: got %q want %q", got.Python, "3.12.3")
	}
	// The "CUDA Arch BIN" line must NOT clobber our CUDA field with "11.0".
	// (Exact-key match keeps it separate.)
}

// TestStripANSIRoundTrip covers the ANSI strip helper in isolation so a
// regression in the regex doesn't silently break the parser above.
func TestStripANSI(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain text", "plain text"},
		{"\x1b[1mbold\x1b[0m", "bold"},
		{"\x1b[92;101mgreen-on-cyan\x1b[0m", "green-on-cyan"},
		{"no codes here", "no codes here"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripANSI(c.in); got != c.want {
			t.Errorf("stripANSI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// equalJetson is a tiny struct-equality helper to keep test failures
// readable. Using reflect.DeepEqual directly would still work but
// produces less friendly error messages.
func equalJetson(a, b *protocol.JetsonInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

var _ = strings.HasPrefix // keep "strings" import alive when only used in helpers
