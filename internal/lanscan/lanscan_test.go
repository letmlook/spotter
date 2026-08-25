package lanscan

import "testing"

func TestRFC1918Rank(t *testing.T) {
	cases := []struct {
		cidr string
		want int
	}{
		// RFC1918 — LAN, rank 0
		{"10.0.0.0/8", 0},
		{"10.255.255.0/24", 0},
		{"172.16.0.0/12", 0},
		{"172.31.255.0/24", 0},
		{"192.168.0.0/24", 0},
		{"192.168.1.0/24", 0},
		// Just outside the RFC1918 windows — rank 1
		{"172.15.0.0/24", 1},
		{"172.32.0.0/24", 1},
		// Public / everything else
		{"8.8.8.0/24", 1},
		{"1.1.1.0/24", 1},
		// Link-local (also rank 1; we never return these from LocalSubnets
		// but the rank function alone is honest about it).
		{"169.254.0.0/16", 1},
	}
	for _, c := range cases {
		if got := RFC1918Rank(c.cidr); got != c.want {
			t.Errorf("RFC1918Rank(%q) = %d, want %d", c.cidr, got, c.want)
		}
	}
}

func TestRFC1918Rank_GarbageCIDR(t *testing.T) {
	// No slash — strings.IndexByte returns -1 and ParseIP gets
	// garbage. Must return 1, not panic.
	if got := RFC1918Rank("not-an-ip"); got != 1 {
		t.Errorf("RFC1918Rank(garbage) = %d, want 1", got)
	}
}
