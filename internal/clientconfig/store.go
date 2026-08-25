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
	"path/filepath"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/protocol"
)

// Defaults. Multicast group + device port are re-exported from
// internal/protocol so the client's settings file (which uses the
// names DefaultMulticastGroup / DefaultDevicePort) does not need
// to import scanner.
const (
	DefaultMulticastGroup = protocol.DefaultMulticastAddr
	DefaultDevicePort     = protocol.DefaultDevicePort
	DefaultScanTimeout    = 30 * time.Second
	DefaultHTTPTimeout    = 3 * time.Second
	DefaultMcastInterval  = 5 * time.Second
	DefaultPollInterval   = 5 * time.Second
	DefaultLogUnit        = "spotterd.service"
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
}

// defaultSettings returns the zero-config Settings as if none had
// been written yet.
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
	}
}

// Store is concurrency-safe. All writes flush immediately.
type Store struct {
	path string
	mu   sync.Mutex
	s    Settings
}

// Open loads (or initialises) a settings file at path.
func Open(path string) (*Store, error) {
	store := &Store{path: path, s: defaultSettings()}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(data, &store.s); err != nil {
		// Corrupt: rename aside + start fresh. We don't want a parse
		// error to brick the GUI on first launch.
		backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano())
		_ = os.Rename(path, backup)
		store.s = defaultSettings()
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
// and flushes to disk.
func (s *Store) Set(in Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
}

func (s *Store) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(&s.s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
