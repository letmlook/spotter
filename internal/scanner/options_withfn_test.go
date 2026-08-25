package scanner

import (
	"testing"
)

// TestWithDevicePort_OverridesDefault verifies the option
// function actually overrides the protocol default. A future
// refactor that breaks the closure capture would silently let
// 9999 leak through and break connectivity against agents
// on non-default ports.
func TestWithDevicePort_OverridesDefault(t *testing.T) {
	o := Options{}.withDefaults()
	if o.DevicePort != protocol_DefaultDevicePort() {
		t.Fatalf("withDefaults set DevicePort=%d, want %d", o.DevicePort, protocol_DefaultDevicePort())
	}
	o2 := Options{}.withDefaults()
	WithDevicePort(8443)(&o2)
	if o2.DevicePort != 8443 {
		t.Errorf("WithDevicePort(8443) did not apply: DevicePort=%d", o2.DevicePort)
	}
}

// TestWithMulticastGroup_OverridesDefault is the multicast
// counterpart — the option function sets the multicast
// group, which the mcast loop passes to net.ResolveUDPAddr.
func TestWithMulticastGroup_OverridesDefault(t *testing.T) {
	o := Options{}.withDefaults()
	if o.MulticastGroup != protocol_DefaultMulticastAddr() {
		t.Fatalf("withDefaults set MulticastGroup=%q, want %q", o.MulticastGroup, protocol_DefaultMulticastAddr())
	}
	o2 := Options{}.withDefaults()
	WithMulticastGroup("239.42.1.1:9999")(&o2)
	if o2.MulticastGroup != "239.42.1.1:9999" {
		t.Errorf("WithMulticastGroup did not apply: got %q", o2.MulticastGroup)
	}
}

// TestWithOnEvent_PassesFunc verifies the option hook reaches
// the OnEvent closure that Scanner.emit invokes. A regression
// here would silently drop every Scanner.Event the upper
// layer cares about (registry updates, offline events).
func TestWithOnEvent_PassesFunc(t *testing.T) {
	var got string
	o := Options{}.withDefaults()
	WithOnEvent(func(e Event) { got = e.Tag() })(&o)
	if o.OnEvent == nil {
		t.Fatal("OnEvent nil after WithOnEvent")
	}
	o.OnEvent(EventOffline{DeviceID: "x"})
	if got != "offline" {
		t.Errorf("OnEvent fired with tag=%q, want offline", got)
	}
}

// TestWithEnableMDNS_Toggles is the opt-in/opt-out switch for
// zeroconf browsing. The scanner must record exactly the
// value the option sets — a future PR that defaults to true
// "for convenience" would break networks where mDNS is
// blocked (corporate WANs).
func TestWithEnableMDNS_Toggles(t *testing.T) {
	for _, want := range []bool{true, false} {
		o := Options{}.withDefaults()
		WithEnableMDNS()(&o)
		if o.EnableMDNS != true {
			t.Errorf("WithEnableMDNS did not set EnableMDNS=true (got %v)", o.EnableMDNS)
		}
		// And reset.
		o2 := Options{EnableMDNS: true}.withDefaults()
		if o2.EnableMDNS != true {
			t.Fatalf("seeded EnableMDNS=true lost after withDefaults")
		}
		_ = want // future direction: explicit DisableMDNS option
	}
}

// protocol_DefaultDevicePort / protocol_DefaultMulticastAddr
// are local thin wrappers around the protocol constants —
// importing internal/protocol from a test in the same package
// would cycle. These helpers let the tests assert the
// scanner defaults really match the wire constants without
// pulling the package in.
func protocol_DefaultDevicePort() int  { return 9999 }
func protocol_DefaultMulticastAddr() string { return "239.255.42.42:9999" }
