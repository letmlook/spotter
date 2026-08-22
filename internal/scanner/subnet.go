package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

// MaxScanHosts caps a single subnet scan to prevent runaway network load.
const MaxScanHosts = 4096

// scanTimeout is the per-host TCP connect timeout.
const scanTimeout = 500 * time.Millisecond

// ScanSubnet probes every IP in cidr for spotterd's HTTP endpoint. Any
// host that responds to a TCP connect on port 9999 is queried for
// /api/v1/info and merged into the registry (existing device: update;
// new device: EventUnknownDeviceDiscovered).
func (s *Scanner) ScanSubnet(ctx context.Context, cidr string, overallTimeout time.Duration) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse cidr %q: %w", cidr, err)
	}
	hosts := expandCIDR(ipnet)
	if len(hosts) > MaxScanHosts {
		return fmt.Errorf("range too large: %d hosts (max %d)", len(hosts), MaxScanHosts)
	}
	if overallTimeout == 0 {
		overallTimeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, overallTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, ip := range hosts {
		ip := ip
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.probeOne(ctx, ip)
		}()
	}
	wg.Wait()
	return nil
}

// expandCIDR walks every IP contained in ipnet. For /30 and smaller
// masks it returns the network and broadcast addresses too (callers
// who want to skip those can mask using OnEvent logic).
func expandCIDR(ipnet *net.IPNet) []net.IP {
	var out []net.IP
	ip := ipnet.IP.Mask(ipnet.Mask)
	// `nextIP` returns nil once it would overflow 255.255.255.255;
	// guarding the `for` condition (rather than a leading `if {break}`)
	// is what staticcheck's QF1006 prefers.
	for ip != nil && ipnet.Contains(ip) {
		out = append(out, append(net.IP(nil), ip...))
		ip = nextIP(ip)
	}
	return out
}

// nextIP returns ip+1, or nil if ip would overflow 255.255.255.255.
func nextIP(ip net.IP) net.IP {
	next := append(net.IP(nil), ip...)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	if ip.To4() != nil && next.Equal(net.IPv4(255, 255, 255, 255).To4()) {
		return nil
	}
	return next
}

// probeOne dials ip:DevicePort and, on success, fetches /api/v1/info
// and merges the result into the registry.
func (s *Scanner) probeOne(ctx context.Context, ip net.IP) {
	addr := net.JoinHostPort(ip.String(), fmt.Sprintf("%d", s.opts.DevicePort))
	d := net.Dialer{Timeout: scanTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		// Per-host dial failures are routine (only one host per /24 is
		// likely running spotterd); log at Debug to keep Info-level
		// scan summaries uncluttered. Non-timeout errors (refused, no
		// route) bubble up to Info for visibility.
		if ctx.Err() == nil {
			s.opts.Logger.Debug("subnet probe dial failed",
				"ip", ip.String(), "err", err.Error())
		}
		return
	}
	if err := conn.Close(); err != nil {
		s.opts.Logger.Debug("subnet probe close failed",
			"ip", ip.String(), "err", err.Error())
	}

	url := "http://" + addr + "/api/v1/info"
	req, err := s.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		s.opts.Logger.Debug("subnet probe build request failed",
			"ip", ip.String(), "err", err.Error())
		return
	}
	resp, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		s.opts.Logger.Debug("subnet probe HTTP failed",
			"ip", ip.String(), "err", err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.opts.Logger.Debug("subnet probe non-200",
			"ip", ip.String(), "code", resp.StatusCode)
		return
	}
	var info protocol.DeviceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		s.opts.Logger.Warn("subnet probe decode failed",
			"ip", ip.String(), "err", err.Error())
		return
	}
	s.mergeInfo("subnet", ip.String(), s.opts.DevicePort, info)
}
