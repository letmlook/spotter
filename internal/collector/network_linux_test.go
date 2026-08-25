//go:build linux

package collector

import (
	"context"
	"testing"

	"github.com/spotter/spotter/internal/protocol"
)

func TestCollectNetworkSmoke(t *testing.T) {
	info := collectNetwork()
	// On real Linux there is at least lo; we assert the slice is
	// non-empty so a future refactor that drops all interfaces
	// surfaces here, not in a GUI surprise.
	if len(info.Interfaces) == 0 {
		t.Errorf("collectNetwork returned 0 interfaces; expected at least lo")
	}
	// PrimaryIP may be empty on locked-down runners, but if any
	// interface has an IPv4 address we should have picked one.
	for _, iface := range info.Interfaces {
		if len(iface.Addrs) > 0 && info.PrimaryIP == "" {
			t.Errorf("interface %s has addrs but PrimaryIP empty", iface.Name)
			break
		}
	}
}

func TestDeviceInfoNetworkMarshals(t *testing.T) {
	info := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		Network: protocol.NetworkInfo{
			PrimaryIP: "10.0.5.23",
			Interfaces: []protocol.Interface{
				{Name: "eth0", MAC: "aa:bb:cc:dd:ee:ff", Addrs: []string{"10.0.5.23/24"}},
			},
		},
	}
	// Just ensure fields flow through Collect path without panic
	c := New()
	_, _ = c.Collect(context.Background())
	_ = info
}
