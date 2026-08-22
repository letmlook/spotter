//go:build linux

package collector

import (
	"context"

	"github.com/spotter/spotter/internal/protocol"
)

// Collector gathers a snapshot of the local device's basic/network/Jetson
// info. All operations are read-only and platform-specific (Linux).
type Collector struct{}

// New returns a Collector using the default OS probes.
func New() *Collector { return &Collector{} }

// Collect returns a populated DeviceInfo. Field-level failures are
// tolerated; only a fully-broken collector returns an error.
func (c *Collector) Collect(ctx context.Context) (protocol.DeviceInfo, error) {
	info := protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		CollectedAt:   nowUTC(),
		Basic:         collectBasic(),
		Network:       collectNetwork(),
	}
	if j := collectJetson(ctx); j != nil {
		info.Jetson = j
	}
	if m := collectMetrics(ctx); m != nil {
		info.Metrics = m
	}
	return info, nil
}
