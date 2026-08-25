package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spotter/spotter/internal/lanscan"
	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)

// withIsolatedHome configures HOME / XDG_CONFIG_HOME to point at a
// dedicated TempDir so cmdList / cmdInfo / cmdScan can open the
// right devices.json and settings.json paths.
func withIsolatedHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

// seededEntry writes an entry into <dir>/Spotter/devices.json with
// a populated LastInfo so cmdInfo can render JSON without a live
// poll.
func seededEntry(t *testing.T, dir, id, ip string, online bool) {
	t.Helper()
	reg, err := registry.Open(filepath.Join(dir, "Spotter", "devices.json"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer reg.Close()
	if err := reg.Add(registry.Entry{
		DeviceID:   id,
		IP:         ip,
		Port:       9999,
		Username:   "nvidia",
		Online:     online,
		LastSource: "test",
		LastInfo: &protocol.DeviceInfo{
			SchemaVersion: protocol.SchemaVersion,
			DeviceID:      id,
			Basic:         protocol.BasicInfo{Hostname: id},
			Network:       protocol.NetworkInfo{PrimaryIP: ip},
		},
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
}

// runInProcess invokes run() with the given args and returns the
// merged output buffer and exit code.
func runInProcess(args []string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func TestRun_NoArgs_PrintsUsage(t *testing.T) {
	out, err, code := runInProcess(nil)
	if code != 2 {
		t.Errorf("want exit 2, got %d", code)
	}
	if !strings.Contains(err, "usage:") {
		t.Errorf("want usage banner on stderr: %q", err)
	}
	_ = out
}

func TestRun_Version(t *testing.T) {
	out, _, code := runInProcess([]string{"version"})
	if code != 0 {
		t.Errorf("want 0, got %d", code)
	}
	if !strings.Contains(out, "spotter-cli") {
		t.Errorf("want version string: %q", out)
	}
}

func TestRun_UnknownCommand_UsageAndExitTwo(t *testing.T) {
	out, err, code := runInProcess([]string{"nope"})
	if code != 2 {
		t.Errorf("want 2, got %d", code)
	}
	if !strings.Contains(err, "usage:") {
		t.Errorf("want usage banner: %q", err)
	}
	if !strings.Contains(err, "unknown command") {
		t.Errorf("want unknown-command marker: %q", err)
	}
	_ = out
}

func TestRun_ListEmpty(t *testing.T) {
	withIsolatedHome(t)
	out, _, code := runInProcess([]string{"list"})
	if code != 0 {
		t.Fatalf("want 0, got %d", code)
	}
	if !strings.Contains(out, "(no devices)") {
		t.Errorf("want 'no devices' marker in stdout: %q", out)
	}
}

func TestRun_ListWithMultipleEntries(t *testing.T) {
	dir := withIsolatedHome(t)
	seededEntry(t, dir, "alpha", "10.0.0.1", true)
	seededEntry(t, dir, "bravo", "10.0.0.2", false)
	seededEntry(t, dir, "charlie", "10.0.0.3", true)
	out, _, code := runInProcess([]string{"list"})
	if code != 0 {
		t.Fatalf("want 0, got %d", code)
	}
	for _, want := range []string{"alpha", "bravo", "charlie", "online", "offline", "9999"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q: %q", want, out)
		}
	}
}

func TestRun_InfoMissingDevice(t *testing.T) {
	withIsolatedHome(t)
	_, err, code := runInProcess([]string{"info", "ghost"})
	if code != 1 {
		t.Errorf("want 1, got %d", code)
	}
	if !strings.Contains(err, "device not in registry") {
		t.Errorf("want 'device not in registry': %q", err)
	}
}

func TestRun_InfoNoArgs(t *testing.T) {
	withIsolatedHome(t)
	_, err, code := runInProcess([]string{"info"})
	if code != 2 {
		t.Errorf("want 2 (usage), got %d", code)
	}
	if !strings.Contains(err, "usage:") {
		t.Errorf("want usage banner: %q", err)
	}
}

func TestRun_InfoCachedDevice_EmitsJSON(t *testing.T) {
	dir := withIsolatedHome(t)
	seededEntry(t, dir, "schema-dev", "10.0.0.42", true)
	out, _, code := runInProcess([]string{"info", "schema-dev"})
	if code != 0 {
		t.Fatalf("want 0, got %d", code)
	}
	// 1. JSON schema: must decode into protocol.DeviceInfo.
	var got protocol.DeviceInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("info output not JSON: %v\n%s", err, out)
	}
	if got.DeviceID != "schema-dev" {
		t.Errorf("DeviceID: %q", got.DeviceID)
	}
	if got.Basic.Hostname != "schema-dev" {
		t.Errorf("Basic.Hostname: %q", got.Basic.Hostname)
	}
	// 2. Pretty-printed: SetIndent("","  ") → at least one indented
	//    line beginning with two spaces and "device_id".
	if !strings.Contains(out, "  \"device_id\"") {
		t.Errorf("expected pretty-printed JSON: %q", out)
	}
}

func TestRun_InfoDeviceWithoutCachedInfo(t *testing.T) {
	dir := withIsolatedHome(t)
	// Insert entry with no LastInfo by registry.Add and then mutate
	// the LastInfo to nil via Update — simpler: Add then directly.
	reg, err := registry.Open(filepath.Join(dir, "Spotter", "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(registry.Entry{DeviceID: "noinfo", IP: "10.0.0.4", Port: 9999}); err != nil {
		t.Fatal(err)
	}
	reg.Close()
	_, errOut, code := runInProcess([]string{"info", "noinfo"})
	if code != 1 {
		t.Errorf("want 1 (no cached info), got %d", code)
	}
	if !strings.Contains(errOut, "no cached info") {
		t.Errorf("want 'no cached info' message: %q", errOut)
	}
}

func TestRun_ScanInvalidCIDR_ExitOne(t *testing.T) {
	dir := withIsolatedHome(t)
	_, errOut, code := runInProcess([]string{"scan", "--cidr=not-a-cidr"})
	if code != 1 {
		t.Errorf("want 1, got %d (stderr=%q)", code, errOut)
	}
	if !strings.Contains(errOut, "CIDR") && !strings.Contains(errOut, "parse") {
		t.Errorf("want CIDR/parse error: %q", errOut)
	}
	_ = dir
}

func TestRun_ScanExplicitCIDR_KnownLAN(t *testing.T) {
	// Bind a fake spotterd to 127.0.0.1:9999 (the device port
	// scanner probes) so the 127.0.0.0/30 scan finds the
	// listener. httptest.NewServer binds a random port and
	// cannot be reached from the scanner; NewUnstartedServer
	// + explicit Listener.Close() + Start() on our listener
	// is the documented pattern.
	ln, err := net.Listen("tcp", "127.0.0.1:9999")
	if err != nil {
		t.Skipf("cannot bind 127.0.0.1:9999 in this sandbox: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema_version":2,"device_id":"loopback-probe","basic":{"hostname":"h"}}`))
	}))
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	withIsolatedHome(t)
	_, errOut, code := runInProcess([]string{"scan", "--cidr=127.0.0.0/30", "--timeout=3s"})
	if code != 0 {
		t.Errorf("scan returned %d, want 0 (stderr=%q)", code, errOut)
	}
	if !strings.Contains(errOut, "scanning 127.0.0.0/30") {
		t.Errorf("want scanning progress: %q", errOut)
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
		if got := lanscan.RFC1918Rank(cidr); got != want {
			t.Errorf("lanscan.RFC1918Rank(%q) = %d, want %d", cidr, got, want)
		}
	}
}

func TestMainpkgLocalSubnets_NoPanic(t *testing.T) {
	_ = lanscan.LocalSubnets()
}
