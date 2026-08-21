//go:build linux

package collector

import (
	"net"
	"sort"

	"github.com/spotter/spotter/internal/protocol"
)

func collectNetwork() protocol.NetworkInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return protocol.NetworkInfo{}
	}

	out := protocol.NetworkInfo{Interfaces: make([]protocol.Interface, 0, len(ifaces))}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ni := protocol.Interface{
			Name: iface.Name,
			MAC:  iface.HardwareAddr.String(),
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ni.Addrs = append(ni.Addrs, a.String())
		}
		out.Interfaces = append(out.Interfaces, ni)
	}
	sort.Slice(out.Interfaces, func(i, j int) bool {
		return out.Interfaces[i].Name < out.Interfaces[j].Name
	})

	out.PrimaryIP = choosePrimaryIP()
	return out
}

// choosePrimaryIP returns the source IP of the route to 8.8.8.8 (a
// reliable internet-reachable address), or the first non-loopback IP if
// the route can't be determined.
func choosePrimaryIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return firstNonLoopback()
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return firstNonLoopback()
}

func firstNonLoopback() string {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}
