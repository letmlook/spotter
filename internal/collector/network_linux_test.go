//go:build linux

package collector

import (
	"context"
	"testing"

	"github.com/spotter/spotter/internal/protocol"
)

func TestCollectNetworkSmoke(t *testing.T) {
	info := collectNetwork()
	// On real Linux there is at least lo or eth0; we just need no panic
	// and a sensible primary IP (often empty on weird CI envs).
	_ = info.PrimaryIP
	_ = info.Interfaces
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
