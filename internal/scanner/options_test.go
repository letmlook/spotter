package scanner

import (
	"testing"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

// TestOptions_DefaultsMatchProtocol pins the wiring between
// scanner.Options.withDefaults and internal/protocol.Default*.
// The two used to drift independently — clientconfig.Settings
// held one copy, scanner.Options.withDefaults held another,
// and the actual fleet defaulted to whichever the operator
// happened to overwrite first. Now both sides reference the
// same protocol constants; this test guards against a future
// refactor that re-introduces a copy.
func TestOptions_DefaultsMatchProtocol(t *testing.T) {
	o := Options{}.withDefaults()

	if o.MulticastGroup != protocol.DefaultMulticastAddr {
		t.Errorf("MulticastGroup = %q, want %q", o.MulticastGroup, protocol.DefaultMulticastAddr)
	}
	if o.DevicePort != protocol.DefaultDevicePort {
		t.Errorf("DevicePort = %d, want %d", o.DevicePort, protocol.DefaultDevicePort)
	}
	if o.PollInterval != protocol.DefaultPollInterval {
		t.Errorf("PollInterval = %v, want %v", o.PollInterval, protocol.DefaultPollInterval)
	}
	if o.McastInterval != protocol.DefaultMcastInterval {
		t.Errorf("McastInterval = %v, want %v", o.McastInterval, protocol.DefaultMcastInterval)
	}
}

// TestOptions_DefaultsAreNonZero covers the invariants the
// scanner / agent rely on: zero or negative intervals would
// deadlock the ticker loop. If a future PR drops a default
// to zero, this test fails before the runtime does.
func TestOptions_DefaultsAreNonZero(t *testing.T) {
	o := Options{}.withDefaults()

	if o.PollInterval <= 0 {
		t.Errorf("PollInterval = %v, want > 0 (would deadlock poll loop)", o.PollInterval)
	}
	if o.McastInterval <= 0 {
		t.Errorf("McastInterval = %v, want > 0 (would deadlock mcast loop)", o.McastInterval)
	}
	if o.HTTPClient == nil {
		t.Errorf("HTTPClient nil — pollOne would panic on Do(req)")
	}
	if o.HTTPClient.Timeout == 0 {
		// net/http default (no timeout) would block forever on
		// a misbehaving agent. Pin non-zero.
		t.Errorf("HTTPClient.Timeout = 0 — poll would hang on dead agent")
	}
	if o.HTTPClient.Timeout > 30*time.Second {
		// DefaultScanTimeout is 30s; HTTPClient timeout should
		// not exceed it or the GUI's "still loading…" UX stalls.
		t.Errorf("HTTPClient.Timeout = %v, want ≤ 30s", o.HTTPClient.Timeout)
	}
}
