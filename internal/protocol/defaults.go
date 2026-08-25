package protocol

import "time"


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

	// DefaultLogUnit is the journalctl -u unit name the agent
	// tails when /api/v1/logs is enabled. Both ends previously
	// hard-coded "spotterd.service" inline — keep the two
	// agreeing via this constant.
	DefaultLogUnit = "spotterd.service"
)

// Scanner-side defaults shared between internal/clientconfig
// (settings.json on disk) and internal/scanner (Options.withDefaults
// at runtime). Previously the two carried separate copies of the
// same numbers — see clientconfig.defaultSettings and
// scanner.Options.withDefaults — that drifted (e.g. one copy
// kept 5s PollInterval when the other moved to 30s). Both now
// reference these constants so an operator's settings.json and
// the in-memory scanner.Options agree by construction.
const (
	DefaultPollInterval  = 30 * time.Second
	DefaultMcastInterval  = 60 * time.Second
	DefaultScanTimeout    = 30 * time.Second
	DefaultHTTPTimeout    = 3 * time.Second
)
