// Package lanscan enumerates the host's network interfaces and ranks
// candidate CIDRs by likelihood of being on a LAN. Extracted from
// main.go and cmd/spotter-cli/main.go so both binaries share one
// implementation; the older copies each carried a comment admitting
// "re-implements main.go's LocalSubnets inline so the CLI binary
// doesn't depend on package main".
package lanscan

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// LocalSubnets returns the CIDRs of every non-loopback, up IPv4
// interface on the host, ordered with RFC1918 ranges first (10/8,
// 172.16/12, 192.168/16) so the most likely LAN segment is at
// index 0. Link-local 169.254/16 is filtered out.
//
// Both the GUI (main.go) and the CLI (cmd/spotter-cli) call this;
// GUI uses it to pre-fill / auto-trigger subnet scans, the CLI
// uses it to pick the scan target when the user did not pass an
// explicit CIDR.
func LocalSubnets() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var cidrs []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			ip := ipnet.IP.To4()
			// Skip link-local (169.254/16).
			if ip[0] == 169 && ip[1] == 254 {
				continue
			}
			ones, _ := ipnet.Mask.Size()
			cidrs = append(cidrs, fmt.Sprintf("%s/%d", ip.Mask(ipnet.Mask), ones))
		}
	}
	sort.SliceStable(cidrs, func(i, j int) bool {
		return RFC1918Rank(cidrs[i]) < RFC1918Rank(cidrs[j])
	})
	return cidrs
}

// RFC1918Rank returns 0 for RFC1918 (LAN), 1 for everything else.
// Used by LocalSubnets to put the LAN subnet first. Exported so
// callers (and tests) can classify arbitrary CIDRs. Garbage input
// (no slash, unparseable IP) returns 1.
func RFC1918Rank(cidr string) int {
	slash := strings.IndexByte(cidr, '/')
	if slash < 0 {
		return 1
	}
	ip := net.ParseIP(cidr[:slash])
	if ip == nil {
		return 1
	}
	// net.ParseIP returns IPv4 in 16-byte (v4-in-v6) form; consult
	// the trailing four bytes so we read 10.x / 172.16-31.x /
	// 192.168.x correctly. Without To4 we'd compare against the
	// high zero bytes of the mapped IPv6 prefix and rank every
	// RFC1918 subnet as "non-LAN".
	v4 := ip.To4()
	if v4 == nil {
		return 1
	}
	switch {
	case v4[0] == 10:
		return 0
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
		return 0
	case v4[0] == 192 && v4[1] == 168:
		return 0
	}
	return 1
}
