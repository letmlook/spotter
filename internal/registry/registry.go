// Package registry persists the client's view of deployed devices to a
// local JSON file. All mutations are flushed immediately and broadcast
// to subscribers so in-process caches (e.g. scanner's pollFailures) can
// stay consistent with on-disk state without polling.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/jsonstore"

	"github.com/spotter/spotter/internal/protocol"
)

// MutationOp identifies the kind of change that produced a MutationEvent.
type MutationOp string

const (
	OpAdd    MutationOp = "add"
	OpUpdate MutationOp = "update"
	OpRemove MutationOp = "remove"
)

// MutationEvent is broadcast to subscribers after every mutation.
// Subscription is non-blocking: a slow consumer drops events rather
// than blocking mutation flushes.
type MutationEvent struct {
	Op       MutationOp
	DeviceID string
}

// defaultSubscriberBuffer is the per-subscriber channel depth. Sized
// to absorb short bursts (a ClearRegistry that removes N devices
// triggers N Remove events); older code paths (single Add/Update) sit
// well below it.
const defaultSubscriberBuffer = 64

// Entry is a row in devices.json. The fields fall into two
// distinct groups; the split is documented here so callers
// know which fields change on a poll-vs-discovery vs which
// only the user can mutate.
//
//   IDENTITY (set once at registration, mutated by the scanner on
//   drift):
//     DeviceID, IP, Port, Username, DeployedAt, Tags
//
//   LIFECYCLE (mutated on every poll/discovery event):
//     LastSeenAt, LastSource, Online
//
//   CACHED SNAPSHOT (refreshed by the scanner every poll tick;
//   stale=true when the agent served its cached copy after a fresh-
//   collect failure):
//     LastInfo *protocol.DeviceInfo
//
// LastInfo is a "shadow copy" of the most recent GET /api/v1/info
// response. It exists so the UI can render an offline device's
// last known hostname / IP / OS without a fresh round trip — it
// is NOT the source of truth for any identity field. Mutating
// identity fields through LastInfo would let a stale cache
// silently overwrite a freshly-registered entry, so all writes
// go through the structured fields above.
type Entry struct {
	DeviceID   string               `json:"device_id"`
	IP         string               `json:"ip"`
	Port       int                  `json:"port"`
	Username   string               `json:"username"`
	DeployedAt string               `json:"deployed_at"`
	LastSeenAt string               `json:"last_seen_at"`
	LastSource string               `json:"last_source"`
	Online     bool                 `json:"online"`
	LastInfo   *protocol.DeviceInfo `json:"last_info,omitempty"` // cached snapshot — see comment above
	Tags       []string             `json:"tags,omitempty"`        // v0.5: user-applied labels
}

// Registry is safe for concurrent use.
type Registry struct {
	path    string
	mu      sync.Mutex
	entries map[string]*Entry
	subMu   sync.Mutex
	subs    []chan<- MutationEvent
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
		// Best-effort backup; non-fatal so Open can still return a usable
		// empty registry. The registry package avoids slog (the only
		// consumer is the client) and falls back to stderr if even the
		// backup write fails.
		if writeErr := os.WriteFile(backup, data, 0600); writeErr != nil {
			fmt.Fprintf(os.Stderr, "registry: corrupt backup %s failed: %v\n", backup, writeErr)
		}
		// Start fresh.
		r.entries = map[string]*Entry{}
		// Rewrite the bad file as empty so future opens are clean. A
		// failure here is also non-fatal.
		if writeErr := os.WriteFile(path, []byte("{}"), 0600); writeErr != nil {
			fmt.Fprintf(os.Stderr, "registry: reset corrupt %s failed: %v\n", path, writeErr)
		}
		return r, nil
	}
	if r.entries == nil {
		r.entries = map[string]*Entry{}
	}
	return r, nil
}

// Subscribe returns a buffered channel that receives a MutationEvent
// after every successful Add/Update/Remove. The Registry holds a
// reference to ch; callers must consume or the channel will eventually
// fill (events then drop). Cancel by closing the channel is not
// supported — instead use the returned channel in a select that exits
// when the registry itself goes out of scope.
func (r *Registry) Subscribe() <-chan MutationEvent {
	r.subMu.Lock()
	defer r.subMu.Unlock()
	ch := make(chan MutationEvent, defaultSubscriberBuffer)
	r.subs = append(r.subs, ch)
	return ch
}

func (r *Registry) broadcastLocked(ev MutationEvent) {
	r.subMu.Lock()
	subs := make([]chan<- MutationEvent, len(r.subs))
	copy(subs, r.subs)
	r.subMu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			// Drop on slow consumer; the next mutation will surface state
			// again when the consumer catches up.
		}
	}
}

// Add inserts a new entry. Errors if device_id already exists.
func (r *Registry) Add(e Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[e.DeviceID]; ok {
		return fmt.Errorf("device %q already in registry", e.DeviceID)
	}
	r.entries[e.DeviceID] = &e
	if err := r.flushLocked(); err != nil {
		return err
	}
	r.broadcastLocked(MutationEvent{Op: OpAdd, DeviceID: e.DeviceID})
	return nil
}

// Remove deletes an entry by device_id. Removing a non-existent entry
// is a no-op (no error), so callers can pass device_ids retrieved from
// a stale snapshot.
func (r *Registry) Remove(deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[deviceID]; !ok {
		return nil
	}
	delete(r.entries, deviceID)
	if err := r.flushLocked(); err != nil {
		return err
	}
	r.broadcastLocked(MutationEvent{Op: OpRemove, DeviceID: deviceID})
	return nil
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
	if err := r.flushLocked(); err != nil {
		return err
	}
	r.broadcastLocked(MutationEvent{Op: OpUpdate, DeviceID: deviceID})
	return nil
}

// Upsert inserts the entry when absent, or applies mut when
// present. Returns created=true if a new row was added. Replaces
// the Get-then-Add-or-Update dance that previously lived at three
// call sites in main.go (auto-accept on unknown discovery,
// ProbeByIP, AcceptUnknownDevice).
func (r *Registry) Upsert(deviceID string, newEntry Entry, mut func(*Entry)) (created bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[deviceID]; ok {
		mut(e)
		if err := r.flushLocked(); err != nil {
			return false, err
		}
		r.broadcastLocked(MutationEvent{Op: OpUpdate, DeviceID: deviceID})
		return false, nil
	}
	r.entries[deviceID] = &newEntry
	if err := r.flushLocked(); err != nil {
		return true, err
	}
	r.broadcastLocked(MutationEvent{Op: OpAdd, DeviceID: deviceID})
	return true, nil
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

// Close releases any held resources and signals subscribers to exit.
// All Subscribe channels returned by Open are closed by Close, so a
// Scanner's watchRegistry goroutine exits cleanly. Idempotent.
func (r *Registry) Close() error {
	r.subMu.Lock()
	defer r.subMu.Unlock()
	for _, ch := range r.subs {
		close(ch)
	}
	r.subs = nil
	return nil
}

func (r *Registry) flushLocked() error {
	return jsonstore.Save(r.path, jsonstore.Ordered(r.entries), 0600)
}
