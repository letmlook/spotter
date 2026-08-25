package protocol

import (
	"regexp"
	"testing"
	"time"
)

// TestDefaults_AllMatchPinNumericAndFormat protects the
// wire constants. cmd/agent's TOML loader, the clientconfig
// JSON on disk, the scanner.Options.withDefaults fallback,
// and the frontend wailsjs/go/models.ts all key off these
// names — silently changing a value here drifts the entire
// fleet.
func TestDefaults_AllMatchPin(t *testing.T) {
	cases := []struct {
		name string
		got  any
		want any
	}{
		{"DefaultListenAddr", DefaultListenAddr, "0.0.0.0:9999"},
		{"DefaultMulticastAddr", DefaultMulticastAddr, "239.255.42.42:9999"},
		{"DefaultDevicePort", DefaultDevicePort, 9999},
		{"DefaultLogUnit", DefaultLogUnit, "spotterd.service"},
		{"DefaultPollInterval", DefaultPollInterval, 30 * time.Second},
		{"DefaultMcastInterval", DefaultMcastInterval, 60 * time.Second},
		{"DefaultScanTimeout", DefaultScanTimeout, 30 * time.Second},
		{"DefaultHTTPTimeout", DefaultHTTPTimeout, 3 * time.Second},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestDefaults_DefaultPortMatchesDefaultMulticastPort guards
// against an off-by-one where the wire constants drift
// (multicast and HTTP share the same UDP-vs-TCP port for
// convenience — a single port keeps operators' firewalls
// sane).
func TestDefaults_DefaultPortMatchesDefaultMulticastPort(t *testing.T) {
	re := regexp.MustCompile(`:(\d+)$`)
	mcast := re.FindStringSubmatch(DefaultMulticastAddr)
	if len(mcast) != 2 {
		t.Fatalf("DefaultMulticastAddr %q has no trailing :PORT", DefaultMulticastAddr)
	}
	if mcast[1] != "9999" {
		t.Errorf("multicast port = %s, want 9999 (= DefaultDevicePort)", mcast[1])
	}
	if DefaultDevicePort != 9999 {
		t.Errorf("DefaultDevicePort = %d, want 9999", DefaultDevicePort)
	}
}
