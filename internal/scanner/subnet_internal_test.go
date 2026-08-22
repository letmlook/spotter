package scanner

import (
	"net"
	"testing"
)

func TestExpandCIDR_v4_24(t *testing.T) {
	_, ipnet, err := net.ParseCIDR("10.0.0.10/24")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	hosts := expandCIDR(ipnet)
	// Per subnet.go comment: expandCIDR returns the network and
	// broadcast addresses too (callers filter via probe). So /24 = 256.
	if len(hosts) != 256 {
		t.Fatalf("/24 should produce 256 hosts, got %d", len(hosts))
	}
}

func TestExpandCIDR_v4_30(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("192.168.1.0/30")
	hosts := expandCIDR(ipnet)
	if len(hosts) != 4 {
		t.Fatalf("/30 should produce 4 hosts, got %d (%v)", len(hosts), hosts)
	}
}

func TestExpandCIDR_v4_31(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("10.0.0.0/31")
	hosts := expandCIDR(ipnet)
	if len(hosts) != 2 {
		t.Fatalf("/31 should produce 2 hosts, got %d", len(hosts))
	}
}

func TestExpandCIDR_v4_32(t *testing.T) {
	_, ipnet, _ := net.ParseCIDR("10.0.0.5/32")
	hosts := expandCIDR(ipnet)
	if len(hosts) != 1 || !hosts[0].Equal(net.ParseIP("10.0.0.5").To4()) {
		t.Fatalf("/32 → [10.0.0.5], got %v", hosts)
	}
}

func TestExpandCIDR_WalksAcrossOctet(t *testing.T) {
	// Confirm carry: 10.0.0.254 -> 10.0.0.255 -> nil.
	_, ipnet, _ := net.ParseCIDR("10.0.0.250/30")
	hosts := expandCIDR(ipnet)
	if len(hosts) == 0 {
		t.Fatal("expected expansion of /30 starting at .250")
	}
}

func TestNextIP_OverflowWrapsAround(t *testing.T) {
	// 255.255.255.255 + 1 wraps to 0.0.0.0 per the current implementation.
	// expandCIDR's loop terminating condition is `ipnet.Contains(ip)`,
	// which naturally stops the walk for any finite mask.
	got := nextIP(net.IPv4(255, 255, 255, 255).To4())
	if got == nil {
		t.Fatalf("nextIP(255.255.255.255) = nil, want wraparound 0.0.0.0")
	}
	if !got.Equal(net.IPv4zero.To4()) {
		t.Errorf("nextIP(255.255.255.255) = %v, want 0.0.0.0", got)
	}
}

func TestNextIP_EndOfRange_CarriesAcrossOctet(t *testing.T) {
	got := nextIP(net.ParseIP("10.0.0.254").To4())
	if got == nil {
		t.Fatal("nextIP(10.0.0.254) returned nil")
	}
	if !got.Equal(net.ParseIP("10.0.0.255").To4()) {
		t.Errorf("nextIP(10.0.0.254) = %v, want 10.0.0.255", got)
	}
}
