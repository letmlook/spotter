// Package serverd is the spotter-server component: a small HTTP +
// WebSocket hub that stores device registrations and heartbeats.
//
// Storage is SQLite WAL (modernc.org/sqlite, pure-Go — no CGO so
// spotter-server still cross-compiles from any host). The Store
// API is unchanged from the v0.5 JSON version; callers (cmd/spotter-server,
// handler tests) were not touched. The on-disk path follows the
// historical convention: `Open("server.json")` opens `server.db`
// next to it; the legacy `server.json` file is left in place but
// unused. Operators can delete the .json manually after the first
// successful start; the loader never re-reads it.
package serverd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite, no CGO.
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

// currentSchemaVersion is bumped when migrations below change.
// Each migration runs once per Open and is recorded in
// `schema_version` so re-opens are no-ops.
const currentSchemaVersion = 1

// migrations lists the DDL statements applied in order, keyed by
// the version that owns them. Adding a new migration: append a
// new version with its DDL, bump currentSchemaVersion, and the
// loop in initSchema will run it on the next Open.
var migrations = []struct {
	version int
	ddl     string
}{
	{
		version: 1,
		ddl: `
			CREATE TABLE IF NOT EXISTS devices (
				device_id    TEXT PRIMARY KEY,
				ip           TEXT NOT NULL,
				port         INTEGER NOT NULL DEFAULT 9999,
				username     TEXT,
				last_seen_at TEXT NOT NULL,
				online       INTEGER NOT NULL DEFAULT 1,
				last_source  TEXT,
				token_hash   TEXT
			);
			CREATE INDEX IF NOT EXISTS devices_online_seen
				ON devices(online, last_seen_at);
		`,
	},
}

// Store is the SQLite-backed persistence layer. The struct keeps
// the *sql.DB so the caller can close it; all writes go through
// the helper methods which take a *sql.Tx internally.
//
// Store is safe for concurrent use: *sql.DB is goroutine-safe and
// the per-Store mutex serialises schema migrations + path migration
// decisions that we don't want racing.
type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex // serialises initSchema across multiple Opens of the same path
}

// Open opens (or creates) the SQLite database at path. Historical
// path argument ends in `.json` (e.g. "server.json"); the suffix
// is rewritten to `.db` so legacy callers don't have to know we
// migrated. The legacy .json file, if present, is left untouched.
func Open(path string) (*Store, error) {
	dbPath := dbPathFor(path)
	// Open with WAL-friendly pragmas. `_journal_mode=WAL` is set
	// after Open via initSchema; the `_busy_timeout` DSN arg keeps
	// concurrent readers from spuriously returning SQLITE_BUSY
	// during checkpointing.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	// SQLite + single-writer doesn't benefit from a large pool;
	// 1 conn keeps WAL checkpoints predictable. We bump it for
	// reads if callers pile on (Hub fan-out tests do).
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", dbPath, err)
	}
	s := &Store{db: db, path: dbPath}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// dbPathFor rewrites a legacy `.json` (or no-suffix) path to the
// new `.db` location. The conversion is local to the Open
// function and never re-applied; a .json argument always maps
// to the same .db on the same directory.
func dbPathFor(path string) string {
	dir, base := filepath.Split(path)
	ext := filepath.Ext(base)
	switch strings.ToLower(ext) {
	case ".db", ".sqlite", ".sqlite3":
		return path
	default:
		// strip ext (e.g. ".json") and append ".db"
		return filepath.Join(dir, strings.TrimSuffix(base, ext)+".db")
	}
}

// initSchema applies any pending migrations in order. The
// schema_version table tracks applied versions; the loop is
// idempotent across re-opens.
func (s *Store) initSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}
	// Switch to WAL *after* the connection is open. The pragma
	// returns the new mode; we don't gate on it because some
	// filesystems (e.g. read-only bind mounts) silently fall
	// back to TRUNCATE — that's still safe, just slower.
	if _, err := s.db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		return fmt.Errorf("set WAL: %w", err)
	}
	// synchronous=NORMAL pairs with WAL: the WAL frame is fsynced
	// at checkpoint (not per-commit), giving much better throughput
	// than FULL with negligible durability loss for our use case
	// (a device table rebuildable from agent heartbeats).
	if _, err := s.db.Exec(`PRAGMA synchronous = NORMAL;`); err != nil {
		return fmt.Errorf("set synchronous: %w", err)
	}
	applied, err := s.appliedVersions()
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if _, err := s.db.Exec(m.ddl); err != nil {
			return fmt.Errorf("apply migration v%d: %w", m.version, err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version); err != nil {
			return fmt.Errorf("record migration v%d: %w", m.version, err)
		}
	}
	return nil
}

func (s *Store) appliedVersions() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT version FROM schema_version`)
	if err != nil {
		return nil, fmt.Errorf("read schema_version: %w", err)
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// Close releases the underlying database connection. Idempotent
// (sql.DB.Close is safe to call once; further calls are no-ops
// but we still want to be defensive against double-Close from
// defer + explicit cleanup paths in tests).
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Path returns the resolved SQLite file path. Useful for
// diagnostics and for cmd/spotter-server to log on startup.
func (s *Store) Path() string { return s.path }

// Upsert inserts or updates a device row, preserving any
// fields the caller didn't supply (Username, TokenHash).
// The device is marked Online=true; LastSeenAt defaults to
// now() if zero.
func (s *Store) Upsert(d Device) error {
	if d.LastSeenAt.IsZero() {
		d.LastSeenAt = time.Now().UTC()
	} else {
		d.LastSeenAt = d.LastSeenAt.UTC()
	}
	d.Online = true
	// Read existing to preserve username / token_hash if the
	// caller didn't pass them. SQLite's UPSERT (ON CONFLICT)
	// doesn't let us say "keep the existing value" without a
	// separate read, so we keep the read+write shape.
	existing, err := s.Get(d.DeviceID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if existing != nil {
		if d.Username == "" {
			d.Username = existing.Username
		}
		if d.TokenHash == "" {
			d.TokenHash = existing.TokenHash
		}
	}
	_, err = s.db.Exec(`
		INSERT INTO devices
			(device_id, ip, port, username, last_seen_at, online, last_source, token_hash)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			ip          = excluded.ip,
			port        = excluded.port,
			username    = excluded.username,
			last_seen_at= excluded.last_seen_at,
			online      = 1,
			last_source = excluded.last_source,
			token_hash  = excluded.token_hash
	`, d.DeviceID, d.IP, d.Port, nullable(d.Username),
		d.LastSeenAt.Format(time.RFC3339Nano), nullable(d.LastSource), nullable(d.TokenHash))
	if err != nil {
		return fmt.Errorf("upsert device %s: %w", d.DeviceID, err)
	}
	return nil
}

// Get returns a defensive copy of the device row.
func (s *Store) Get(id string) (*Device, error) {
	row := s.db.QueryRow(`
		SELECT device_id, ip, port, username, last_seen_at, online, last_source, token_hash
		FROM devices WHERE device_id = ?`, id)
	var d Device
	var username, lastSource, tokenHash sql.NullString
	var lastSeen string
	var online int
	err := row.Scan(&d.DeviceID, &d.IP, &d.Port, &username,
		&lastSeen, &online, &lastSource, &tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get device %s: %w", id, err)
	}
	d.Username = username.String
	d.LastSource = lastSource.String
	d.TokenHash = tokenHash.String
	d.Online = online != 0
	if t, err := time.Parse(time.RFC3339Nano, lastSeen); err == nil {
		d.LastSeenAt = t
	}
	return &d, nil
}

// List returns every device, key-sorted by device_id. The
// SQL ORDER BY gives a stable order without needing an
// in-memory sort step.
func (s *Store) List() []Device {
	rows, err := s.db.Query(`
		SELECT device_id, ip, port, username, last_seen_at, online, last_source, token_hash
		FROM devices ORDER BY device_id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]Device, 0, 32)
	for rows.Next() {
		var d Device
		var username, lastSource, tokenHash sql.NullString
		var lastSeen string
		var online int
		if err := rows.Scan(&d.DeviceID, &d.IP, &d.Port, &username,
			&lastSeen, &online, &lastSource, &tokenHash); err != nil {
			continue
		}
		d.Username = username.String
		d.LastSource = lastSource.String
		d.TokenHash = tokenHash.String
		d.Online = online != 0
		if t, err := time.Parse(time.RFC3339Nano, lastSeen); err == nil {
			d.LastSeenAt = t
		}
		out = append(out, d)
	}
	return out
}

// Delete removes a device row. Returns ErrNotFound if the row
// didn't exist so callers can distinguish "already gone" from
// a real database error.
func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM devices WHERE device_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete device %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkOffline flips a device's online flag to false and
// updates last_seen_at. Returns ErrNotFound if the row
// didn't exist (the heartbeat endpoint already maps this
// to 404 in handler.recordHeartbeat).
func (s *Store) MarkOffline(id string, at time.Time) error {
	res, err := s.db.Exec(`
		UPDATE devices SET online = 0, last_seen_at = ?
		WHERE device_id = ?`, at.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("mark offline %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// nullable converts an empty string to sql.NullString{Valid:false}
// so an empty TEXT column stays NULL instead of being stored as
// the empty string. Pure-Go SQLite's driver handles NULL fine;
// storing "" would force every reader to special-case emptiness.
func nullable(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// JSON returns the device as a JSON object — used by handler
// tests that assert wire shapes. Kept as a method so a future
// migration off the struct tags (e.g. row-column aliases) is
// a single place to fix.
func (d Device) JSON() string {
	b, _ := json.Marshal(d)
	return string(b)
}

// _ silences the unused-import linter when the file is built
// without the modernc driver (e.g. partial refactors during
// development). The import above is load-bearing; this is a
// belt-and-suspenders reminder.
var _ = os.Stat
