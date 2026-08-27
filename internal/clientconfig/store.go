// Package clientconfig persists user-tunable settings that are NOT
// devices — multisource scan parameters, the multicast group, the
// scanner's sender ID, etc. Kept separate from the registry (which
// is per-device state) so a future "share registry" feature can ship
// without leaking these preferences.
package clientconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/spotter/spotter/internal/jsonstore"
	"github.com/spotter/spotter/internal/protocol"
)

// Defaults. Multicast group + device port + log unit + scanner
// intervals are re-exported from internal/protocol so the
// client's settings file (which uses the names
// DefaultMulticastGroup / DefaultDevicePort / DefaultLogUnit /
// DefaultPollInterval / DefaultMcastInterval) does not need to
// import scanner.
const (
	DefaultMulticastGroup = protocol.DefaultMulticastAddr
	DefaultDevicePort     = protocol.DefaultDevicePort
	DefaultScanTimeout    = protocol.DefaultScanTimeout
	DefaultHTTPTimeout    = protocol.DefaultHTTPTimeout
	DefaultMcastInterval  = protocol.DefaultMcastInterval
	DefaultPollInterval   = protocol.DefaultPollInterval
	DefaultLogUnit        = protocol.DefaultLogUnit
)

// Settings is the on-disk shape. Fields with omitempty are optional;
// missing values fall back to defaults applied in ApplyTo.
type Settings struct {
	MulticastGroup string        `json:"multicast_group,omitempty"`
	DevicePort     int           `json:"device_port,omitempty"`
	ScanTimeout    time.Duration `json:"scan_timeout,omitempty"`
	HTTPTimeout    time.Duration `json:"http_timeout,omitempty"`
	PollInterval   time.Duration `json:"poll_interval,omitempty"`
	McastInterval  time.Duration `json:"mcast_interval,omitempty"`
	Theme          string        `json:"theme,omitempty"` // "light" | "dark" | "system"
	Language       string        `json:"language,omitempty"`
	// AuthToken is stored verbatim on disk; the UI is responsible
	// for clearing it (e.g. on logout). Mode 0600, owner-only.
	AuthToken string `json:"auth_token,omitempty"`
	// EnableMDNS gates zeroconf announce/browse. Default false so
	// networks that block mDNS multicast (corporate WANs, isolated
	// segments) do not get unsolicited traffic. The UI exposes a
	// checkbox so operators can opt in.
	EnableMDNS bool `json:"enable_mdns,omitempty"`
	// ClientID is a UUID v4 stamped on every UDP HELLO and HTTP
	// header so an agent can identify a specific client across
	// reconnects. Generated once on first Open and persisted
	// alongside the other settings; it is intentionally NOT
	// re-generated on Update so the same physical client keeps
	// the same identity across restarts.
	ClientID string `json:"client_id,omitempty"`
}

// defaultSettings returns the zero-config Settings as if none had
// been written yet. ClientID is generated as a fresh UUID v4 —
// callers must NOT share the returned Settings across stores; the
// UUID is meant to identify the *local* client, not the file.
func defaultSettings() Settings {
	return Settings{
		MulticastGroup: DefaultMulticastGroup,
		DevicePort:     DefaultDevicePort,
		ScanTimeout:    DefaultScanTimeout,
		HTTPTimeout:    DefaultHTTPTimeout,
		PollInterval:   DefaultPollInterval,
		McastInterval:  DefaultMcastInterval,
		Theme:          "system",
		Language:       "zh-CN",
		ClientID:       uuid.NewString(),
	}
}

// Store is concurrency-safe. All writes flush immediately.
type Store struct {
	path string
	mu   sync.Mutex
	s    Settings
}

// Open loads (or initialises) a settings file at path. The first
// launch — file missing or empty — flushes the generated defaults
// (including a fresh ClientID UUID v4) so subsequent Opens see the
// same identity, and a corrupt file is renamed aside and replaced
// with the same defaults. This is the only path that allocates a
// new ClientID; Set/Update preserve whatever is already in the
// store so a user's sender identity survives Settings writes.
func Open(path string) (*Store, error) {
	store := &Store{path: path, s: defaultSettings()}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := store.flushLocked(); err != nil {
				return nil, fmt.Errorf("write initial %s: %w", path, err)
			}
			return store, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		if err := store.flushLocked(); err != nil {
			return nil, fmt.Errorf("write initial %s: %w", path, err)
		}
		return store, nil
	}
	if err := json.Unmarshal(data, &store.s); err != nil {
		// Corrupt: rename aside + start fresh. We don't want a parse
		// error to brick the GUI on first launch.
		backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano())
		_ = os.Rename(path, backup)
		store.s = defaultSettings()
		if err := store.flushLocked(); err != nil {
			return nil, fmt.Errorf("write post-recovery %s: %w", path, err)
		}
		return store, nil
	}
	// Apply defaults for any unset field.
	store.fillDefaults()
	return store, nil
}

// Get returns the current settings (defensive copy).
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s
}

// Set replaces settings entirely, fills defaults for unset fields,
// and flushes to disk. The ClientID is treated as a sticky
// identity: if the caller passes an empty string (the normal UI
// path, which never touches the field) the existing in-memory
// ClientID is preserved. Pass an explicit value to rotate it.
func (s *Store) Set(in Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.ClientID == "" {
		in.ClientID = s.s.ClientID
	}
	s.s = in
	s.fillDefaultsLocked()
	return s.flushLocked()
}

// Update applies mutator and persists. Useful for one-field tweaks
// from the UI without round-tripping through Set.
func (s *Store) Update(mut func(*Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mut(&s.s)
	s.fillDefaultsLocked()
	return s.flushLocked()
}

// Path returns the file location (useful for the UI to display).
func (s *Store) Path() string { return s.path }

func (s *Store) fillDefaults() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fillDefaultsLocked()
}

func (s *Store) fillDefaultsLocked() {
	d := defaultSettings()
	if s.s.MulticastGroup == "" {
		s.s.MulticastGroup = d.MulticastGroup
	}
	if s.s.DevicePort == 0 {
		s.s.DevicePort = d.DevicePort
	}
	if s.s.ScanTimeout == 0 {
		s.s.ScanTimeout = d.ScanTimeout
	}
	if s.s.HTTPTimeout == 0 {
		s.s.HTTPTimeout = d.HTTPTimeout
	}
	if s.s.PollInterval == 0 {
		s.s.PollInterval = d.PollInterval
	}
	if s.s.McastInterval == 0 {
		s.s.McastInterval = d.McastInterval
	}
	if s.s.Theme == "" {
		s.s.Theme = d.Theme
	}
	if s.s.Language == "" {
		s.s.Language = d.Language
	}
	// ClientID is intentionally NOT filled here. A sticky identity
	// is allocated in `defaultSettings` and persisted on the first
	// Open; Set/Update preserve whatever the caller handed in.
	// Filling from a fresh `defaultSettings()` here would rotate
	// the UUID on every Settings write and break agent-side
	// correlation across reconnects.
}

func (s *Store) flushLocked() error {
	return jsonstore.Save(s.path, &s.s, 0600)
}
