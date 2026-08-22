package mdns

import (
	"net"
	"testing"
)

func TestLookupTXT_KeyValue(t *testing.T) {
	txt := []string{"device_id=abc123", "version=0.3"}
	if got := lookupTXT(txt, "device_id"); got != "abc123" {
		t.Errorf("device_id = %q, want abc123", got)
	}
	if got := lookupTXT(txt, "version"); got != "0.3" {
		t.Errorf("version = %q, want 0.3", got)
	}
	if got := lookupTXT(txt, "missing"); got != "" {
		t.Errorf("missing = %q, want empty", got)
	}
}

func TestLookupTXT_NoEquals(t *testing.T) {
	if got := lookupTXT([]string{"bare"}, "anything"); got != "" {
		t.Errorf("bare should not match anything: %q", got)
	}
}

func TestLookupTXT_Empty(t *testing.T) {
	if got := lookupTXT(nil, "anything"); got != "" {
		t.Errorf("nil slice: %q", got)
	}
}

func TestFirstIPv4(t *testing.T) {
	v4 := net.ParseIP("10.0.0.1").To4()
	v6 := net.ParseIP("fe80::1")
	if got := firstIPv4([]net.IP{v4, v6}); got != "10.0.0.1" {
		t.Errorf("first ipv4: %q, want 10.0.0.1", got)
	}
	if got := firstIPv4([]net.IP{v6}); got != "" {
		t.Errorf("no v4 → empty: %q", got)
	}
}

func TestNewAnnouncer_RejectsEmptyDeviceID(t *testing.T) {
	if _, err := NewAnnouncer("", 9999); err == nil {
		t.Fatal("expected error for empty device_id")
	}
}

func TestNewAnnouncer_RejectsBadPort(t *testing.T) {
	for _, p := range []int{-1, 70000} {
		if _, err := NewAnnouncer("dev", p); err == nil {
			t.Errorf("expected error for port %d", p)
		}
	}
}

// TestNewAnnouncer_RegisterAndShutdown exercises the real zeroconf
// stack on loopback. Skipped in environments without multicast
// (CI containers, sandboxed macOS).
func TestNewAnnouncer_RegisterAndShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("mDNS requires multicast; skipped in -short mode")
	}
	ann, err := NewAnnouncer("spotter-test-"+t.Name(), 0)
	if err != nil {
		t.Skipf("mDNS not available in this environment: %v", err)
	}
	if ann == nil {
		t.Fatal("Announcer should not be nil on success")
	}
	ann.Shutdown()
}
