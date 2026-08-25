// Package serverd is the spotter-server component: a small HTTP +
// WebSocket hub that stores device registrations and heartbeats. In
// v0.5 this is a PoC with a JSON-on-disk store; SQLite WAL is on
// the roadmap once the protocol stabilises.
package serverd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/jsonstore"
)

// Device is the persistent shape of a registered spotterd agent.
type Device struct {
	DeviceID   string    `json:"device_id"`
	IP         string    `json:"ip"`
	Port       int       `json:"port"`
	Username   string    `json:"username,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Online     bool      `json:"online"`
	LastSource string    `json:"last_source,omitempty"`
	// TokenHash records bcrypt(token) the registering agent used.
	// Server may later use it to dial back into the agent for
	// privileged ops; v0.5 keeps it for forward compatibility.
	TokenHash string `json:"token_hash,omitempty"`
}

// ErrNotFound indicates the device_id is unknown to the store.
var ErrNotFound = errors.New("serverd: device not found")

// Store is the persistence interface. The in-process mutex-guarded
// JSON file is the v0.5 implementation; later versions may swap to
// SQLite without changing callers.
type Store struct {
	path string
	mu   sync.Mutex
	devs map[string]*Device
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, devs: map[string]*Device{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > 0 {
		var raw map[string]Device
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		for k, v := range raw {
			d := v
			s.devs[k] = &d
		}
	}
	return s, nil
}

func (s *Store) Upsert(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.LastSeenAt.IsZero() {
		d.LastSeenAt = time.Now().UTC()
	}
	d.Online = true
	existing := s.devs[d.DeviceID]
	if existing != nil {
		// Preserve fields the registering agent didn't pass in.
		if d.Username == "" {
			d.Username = existing.Username
		}
		if d.TokenHash == "" {
			d.TokenHash = existing.TokenHash
		}
	}
	s.devs[d.DeviceID] = &d
	return s.flushLocked()
}

func (s *Store) Get(id string) (*Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devs[id]
	if !ok {
		return nil, ErrNotFound
	}
	// Return a copy so callers can't mutate internal state.
	cp := *d
	return &cp, nil
}

func (s *Store) List() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.devs))
	for _, d := range s.devs {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
	return out
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.devs, id)
	return s.flushLocked()
}

func (s *Store) MarkOffline(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devs[id]
	if !ok {
		return ErrNotFound
	}
	d.Online = false
	d.LastSeenAt = at.UTC()
	return s.flushLocked()
}

func (s *Store) flushLocked() error {
	return jsonstore.Save(s.path, jsonstore.Ordered(s.devs), 0600)
}
