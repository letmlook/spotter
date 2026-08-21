// Package registry persists the client's view of deployed devices to a
// local JSON file. All mutations are flushed immediately.
package registry

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

// Entry is a row in devices.json. Includes last-known runtime info
// (LastInfo) so the UI can render offline devices' last known state.
type Entry struct {
	DeviceID   string               `json:"device_id"`
	IP         string               `json:"ip"`
	Port       int                  `json:"port"`
	Username   string               `json:"username"`
	DeployedAt string               `json:"deployed_at"`
	LastSeenAt string               `json:"last_seen_at"`
	LastSource string               `json:"last_source"`
	Online     bool                 `json:"online"`
	LastInfo   *protocol.DeviceInfo `json:"last_info,omitempty"`
}

// Registry is safe for concurrent use.
type Registry struct {
	path    string
	mu      sync.Mutex
	entries map[string]*Entry
}

// Open loads (or initializes) a registry at path. If the file is
// corrupt, it is backed up to <path>.corrupt-<timestamp> and a fresh
// empty registry is returned.
func Open(path string) (*Registry, error) {
	r := &Registry{path: path, entries: map[string]*Entry{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return r, nil
	}
	if err := json.Unmarshal(data, &r.entries); err != nil {
		backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano())
		_ = os.WriteFile(backup, data, 0644)
		// Start fresh.
		r.entries = map[string]*Entry{}
		// Best-effort: rewrite the bad file as empty so future opens are clean.
		_ = os.WriteFile(path, []byte("{}"), 0644)
		return r, nil
	}
	if r.entries == nil {
		r.entries = map[string]*Entry{}
	}
	return r, nil
}

// Add inserts a new entry. Errors if device_id already exists.
func (r *Registry) Add(e Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[e.DeviceID]; ok {
		return fmt.Errorf("device %q already in registry", e.DeviceID)
	}
	r.entries[e.DeviceID] = &e
	return r.flushLocked()
}

// Remove deletes an entry by device_id.
func (r *Registry) Remove(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, deviceID)
	return r.flushLocked()
}

// Update applies mutator to the entry identified by deviceID.
func (r *Registry) Update(deviceID string, mut func(*Entry)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[deviceID]
	if !ok {
		return fmt.Errorf("device %q not found", deviceID)
	}
	mut(e)
	return r.flushLocked()
}

// Get returns a copy of the entry.
func (r *Registry) Get(deviceID string) (Entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[deviceID]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

// FindByIP returns an entry matching IP/port.
func (r *Registry) FindByIP(ip string, port int) (Entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.IP == ip && e.Port == port {
			return *e, true
		}
	}
	return Entry{}, false
}

// List returns a snapshot of all entries.
func (r *Registry) List() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, *e)
	}
	return out
}

// Close releases any held resources. Currently a no-op (kept for API
// stability and future file locking).
func (r *Registry) Close() error { return nil }

func (r *Registry) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0644)
}
