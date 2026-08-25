package protocol

// Wire defaults shared by every binary. Both the agent
// (cmd/agent) and the client (main.go) listen on DefaultListenAddr
// and announce / poll DefaultMulticastAddr on UDP. Keep the two
// binaries agreeing via these constants — previously each side
// re-hardcoded "9999" / "239.255.42.42:9999" inline and the two
// drifted silently.
const (
	// DefaultListenAddr is the HTTP+UDP listen address the
	// spotterd agent binds by default when the operator omits
	// [agent].listen_addr in /etc/spotterd/agent.toml.
	DefaultListenAddr = "0.0.0.0:9999"

	// DefaultMulticastAddr is the IPv4 multicast group + UDP
	// port the agent sends HELLO on and the client listens on
	// for HELLO-REPLY. Wired into RFC1918 / IGMPv2 / mDNS
	// libraries without surprise.
	DefaultMulticastAddr = "239.255.42.42:9999"

	// DefaultDevicePort is the HTTP port the agent's
	// /api/v1/* endpoints serve on. The client polls this port
	// when probing a device. Used as the fallback when a
	// registry.Entry.Port is unset.
	DefaultDevicePort = 9999
)
