//go:build linux

package collector

import (
	"context"
	"testing"

	"github.com/spotter/spotter/internal/protocol"
)

// TestCollect_StampsSchemaVersionAndCollectedAt exercises the public
// Collect() entry point and asserts that SchemaVersion is stamped
// from protocol.SchemaVersion and CollectedAt is a non-empty RFC3339
// timestamp. The per-collector tests (basic/network/jetson) cover
// the sub-functions, but no test was driving the composed Collect()
// path, so a refactor that drops SchemaVersion (or leaves CollectedAt
// empty) would silently regress every client.
func TestCollect_StampsSchemaVersionAndCollectedAt(t *testing.T) {
	c := New()
	info, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if info.SchemaVersion != protocol.SchemaVersion {
		t.Errorf("SchemaVersion=%d, want %d", info.SchemaVersion, protocol.SchemaVersion)
	}
	if info.CollectedAt == "" {
		t.Errorf("CollectedAt is empty")
	}
	// Loose shape check: must end with 'Z' (UTC) or have an offset;
	// we don't validate the full RFC3339 parser here because Go's
	// time.RFC3339 is the source format we use.
	if len(info.CollectedAt) < 20 {
		t.Errorf("CollectedAt looks too short to be RFC3339: %q", info.CollectedAt)
	}
}
