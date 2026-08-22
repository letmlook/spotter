// Package mdns announces and discovers spotterd agents via mDNS /
// DNS-SD so the client can re-anchor a device after it migrates to
// a new network. Built on github.com/grandcat/zeroconf (pure-Go — no
// cgo, so the agent stays a static binary).
//
// Service type: "_spotter._tcp.local." with a TXT record carrying
// the device_id. The HTTP port (9999 by default) is announced via
// the SRV record's port field.
//
// We do NOT use the multicast group already defined for spotter —
// it is UDP-only and operates at L2; mDNS works across VLANs (L3
// multicast) and survives device IP changes.
package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/grandcat/zeroconf"
)

// ServiceType is the DNS-SD service type registered by every spotterd
// agent. The browser side uses this same constant.
const ServiceType = "_spotter._tcp"

// Domain is the conventional DNS-SD search domain; "local." is the
// default mDNS TLD.
const Domain = "local."

// TXTRecordDeviceID is the key used in the TXT record to carry the
// device_id. Browsers use it to match announcements back to known
// registry entries.
const TXTRecordDeviceID = "device_id"

// Announcer wraps a zeroconf.Server, allowing callers to refresh
// the announcement (not currently needed since zeroconf already
// refreshes automatically) or shut it down on agent exit.
type Announcer struct {
	server *zeroconf.Server
}

// NewAnnouncer registers a single service instance. deviceID is the
// agent's UUID (must be unique per device); port is the HTTP listener
// (9999 by default). port=0 means "let the OS pick a free port" and
// is useful for tests.
func NewAnnouncer(deviceID string, port int) (*Announcer, error) {
	if deviceID == "" {
		return nil, errors.New("mdns: device_id required")
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("mdns: invalid port %d", port)
	}
	txt := []string{TXTRecordDeviceID + "=" + deviceID}
	srv, err := zeroconf.Register(
		deviceID,    // instance name
		ServiceType, // service type
		Domain,      // domain
		port,        // port
		txt,         // TXT records (key=value pairs)
		nil,         // interfaces (nil = all)
	)
	if err != nil {
		return nil, fmt.Errorf("mdns register: %w", err)
	}
	return &Announcer{server: srv}, nil
}

// Shutdown releases the service registration. Safe to call on nil.
func (a *Announcer) Shutdown() {
	if a == nil || a.server == nil {
		return
	}
	a.server.Shutdown()
}

// ServiceEntry is a single mDNS result relevant to spotter.
type ServiceEntry struct {
	DeviceID string
	Addr     string // IPv4 dotted-quad; IPv6 ignored (LAN discoverability is IPv4-first)
	Port     int
}

// Browser watches the network for spotter service announcements and
// forwards each entry to onEntry. The browse goroutine runs until ctx
// is cancelled.
type Browser struct {
	resolver *zeroconf.Resolver
}

// NewBrowser constructs (but does not yet start) a Browser. Call
// Start(ctx, onEntry) to begin watching. The browser deduplicates
// entries by (DeviceID, Addr) — the same device keeps re-announcing
// over time, but onEntry fires once per unique pair.
func NewBrowser() (*Browser, error) {
	resolver, err := zeroconf.NewResolver(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("mdns resolver: %w", err)
	}
	return &Browser{resolver: resolver}, nil
}

// Start kicks off the browse loop. onEntry is invoked from a separate
// goroutine; it must be safe for concurrent use. Multiple entries for
// the same DeviceID over time keep flowing in, allowing callers to
// detect IP changes by comparing to the previous value.
func (b *Browser) Start(ctx context.Context, onEntry func(ServiceEntry)) error {
	entries := make(chan *zeroconf.ServiceEntry, 16)
	if err := b.resolver.Browse(ctx, ServiceType, Domain, entries); err != nil {
		return fmt.Errorf("mdns browse: %w", err)
	}
	go b.dispatch(entries, onEntry)
	return nil
}

func (b *Browser) dispatch(ch <-chan *zeroconf.ServiceEntry, onEntry func(ServiceEntry)) {
	for entry := range ch {
		if entry == nil {
			continue
		}
		deviceID := lookupTXT(entry.Text, TXTRecordDeviceID)
		if deviceID == "" {
			continue
		}
		addr := firstIPv4(entry.AddrIPv4)
		if addr == "" {
			continue
		}
		onEntry(ServiceEntry{
			DeviceID: deviceID,
			Addr:     addr,
			Port:     entry.Port,
		})
	}
}

// lookupTXT returns the value associated with key in a slice of
// TXT records formatted as "key=value". zeroconf encodes the raw TXT
// RR text into a []string; we accept either a "key=value" pair or
// just a bare value at index 0 (for compatibility with simple clients).
func lookupTXT(txt []string, key string) string {
	for _, raw := range txt {
		if eq := strings.IndexByte(raw, '='); eq >= 0 {
			if raw[:eq] == key {
				return raw[eq+1:]
			}
		}
	}
	return ""
}

// firstIPv4 returns the first IPv4 address in the provided slice,
// or empty string if none.
func firstIPv4(addrs []net.IP) string {
	for _, ip := range addrs {
		if ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}

// Close is here for symmetry with shutdown patterns; resolvers are
// GC'd when ctx is cancelled.
func (b *Browser) Close() {}
