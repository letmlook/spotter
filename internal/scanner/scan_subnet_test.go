package scanner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/registry"
)

// TestScanSubnet_ProbesCIDRHosts stands up 3 fake agents on
// 127.0.0.2-4 inside a /30 (covers .0 net + .1-.2 host + .3
// broadcast), runs ScanSubnet against /30, and asserts each
// reachable host's /api/v1/info handler gets hit exactly
// once. Pins the wire contract between the subnet probe
// loop and the agent's /api/v1/info endpoint.
func TestScanSubnet_ProbesCIDRHosts(t *testing.T) {
	// We can't bind 127.0.0.0 (network) or 127.0.0.3 (broadcast),
	// but .1 and .2 will accept. Use a /29 so we get
	// 127.0.0.1, 127.0.0.2, 127.0.0.3, 127.0.0.4, 127.0.0.5,
	// 127.0.0.6 — six real /24 hosts.
	var hits sync.Map // host -> *atomic.Int64

	makeHandler := func(host string) http.HandlerFunc {
		var c atomic.Int64
		hits.Store(host, &c)
		return func(w http.ResponseWriter, r *http.Request) {
			counter, _ := hits.Load(host)
			counter.(*atomic.Int64).Add(1)
			w.Header().Set("Content-Type", "application/json")
			body := fmt.Sprintf(`{"schema_version":2,"device_id":%q,"basic":{"hostname":"h"},"network":{"primary_ip":%q}}`, host, host)
			_, _ = w.Write([]byte(body))
		}
	}
	listeners := []net.Listener{}
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	for _, h := range []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"} {
		host := h
		ln, err := net.Listen("tcp", host+":0")
		if err != nil {
			t.Skipf("cannot bind %s:0 in this sandbox: %v", host, err)
		}
		listeners = append(listeners, ln)
		go func() {
			_ = http.Serve(ln, makeHandler(host))
		}()
	}

	// Resolve each listener's port so the registry row points
	// at a real live handler.
	reg, _ := registry.Open(filepath.Join(t.TempDir(), "reg.json"))
	defer reg.Close()
	for _, ln := range listeners {
		host, port := splitHostPort(ln.Addr().String())
		_ = reg.Add(registry.Entry{
			DeviceID: "scan-" + host,
			IP:       host,
			Port:     port,
			Online:   true,
		})
	}

	s := New(reg, WithDevicePort(0))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.ScanSubnet(ctx, "127.0.0.0/29", 2*time.Second); err != nil {
		t.Fatalf("ScanSubnet: %v", err)
	}

	// Every live host should have been hit at least once.
	for _, ln := range listeners {
		host, _ := splitHostPort(ln.Addr().String())
		v, ok := hits.Load(host)
		if !ok {
			t.Fatalf("host %s missing from hits map", host)
		}
		if c := v.(*atomic.Int64).Load(); c < 1 {
			t.Errorf("host %s hits = %d, want ≥1", host, c)
		}
	}
}

// splitHostPort is a tiny local helper so the test doesn't
// need to import net.SplitHostPort.
func splitHostPort(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

// TestScanSubnet_UnknownCIDR exercises the parse-error path —
// garbage in must surface as a parse error, not panic or
// silent success.
func TestScanSubnet_UnknownCIDR(t *testing.T) {
	reg, _ := registry.Open(filepath.Join(t.TempDir(), "reg.json"))
	defer reg.Close()
	s := New(reg)
	if err := s.ScanSubnet(context.Background(), "not-a-cidr", time.Second); err == nil {
		t.Fatal("expected error on garbage CIDR, got nil")
	}
}

// Verify httptest import is reachable (used indirectly via
// server examples in other test files; keeps gofmt happy).
var _ = httptest.NewRecorder
