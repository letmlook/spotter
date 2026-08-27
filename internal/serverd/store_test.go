package serverd

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestStore_WALModeEnabled — the SQLite store must run in WAL
// journal mode so readers don't block the (rare) writer and a
// crash doesn't lose the most recent committed state. We open a
// fresh db, query `journal_mode`, and assert the result.
func TestStore_WALModeEnabled(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

// TestStore_LegacyJsonPathRewrittenToDb — Open("server.json")
// must transparently open "server.db" so the historical config
// path keeps working. The legacy .json file, if present, is
// left untouched (a manual cleanup step after the first start).
func TestStore_LegacyJsonPathRewrittenToDb(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "server.json")
	// Plant a legacy .json file. The store should NOT read it,
	// but it should also NOT delete it (operator's call).
	if err := os.WriteFile(jsonPath, []byte("not json, should be ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Path() == jsonPath {
		t.Errorf("path was not rewritten: %s", s.Path())
	}
	if filepath.Ext(s.Path()) != ".db" {
		t.Errorf("rewritten path should end in .db, got %s", s.Path())
	}
	// Legacy .json must still be on disk — the store does not
	// touch it. We only check existence; content is irrelevant
	// because the store never reads it.
	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("legacy .json was deleted: %v", err)
	}
}

// TestStore_ConcurrentUpserts — the contract is that the store
// is safe for concurrent callers. Spin 16 goroutines each
// inserting 100 devices, then assert the total is 1600 unique
// rows. With a single-writer SQLite this exercises the busy-timeout
// pragma in the DSN — if the timeout was too short, the test
// would surface SQLITE_BUSY errors.
func TestStore_ConcurrentUpserts(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const goroutines = 16
	const perG = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				d := Device{
					DeviceID: deviceIDFor(g, i),
					IP:       "10.0.0.1",
					Port:     9999,
				}
				if err := s.Upsert(d); err != nil {
					t.Errorf("upsert %d/%d: %v", g, i, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	list := s.List()
	if len(list) != goroutines*perG {
		t.Errorf("list size = %d, want %d", len(list), goroutines*perG)
	}
}

// TestStore_CloseIdempotent — double-Close should not panic.
// sql.DB.Close is documented to be safe to call multiple times,
// but the wrapper hides the *sql.DB; lock that contract down.
func TestStore_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Second Close: Go's database/sql is documented as safe to
	// call multiple times; our wrapper is too. If a future refactor
	// introduces a state-bearing close (e.g. unlinking the file),
	// this test will catch the double-close bug before it ships.
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestStore_FieldsPreservedOnPartialUpdate — Upsert must keep
// the existing Username / TokenHash if the caller doesn't pass
// them. This is the only way an agent can re-register after
// rotation without losing its identity. The bug we want to
// catch: a refactor that uses ON CONFLICT DO UPDATE SET ...
// (without the excluded.* aliases) would clobber username to "".
func TestStore_FieldsPreservedOnPartialUpdate(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Upsert(Device{DeviceID: "d1", IP: "10.0.0.1", Username: "alice", TokenHash: "hash"}); err != nil {
		t.Fatal(err)
	}
	// Re-register without username / token_hash — heartbeat path
	// only sets Online + LastSeenAt + IP.
	if err := s.Upsert(Device{DeviceID: "d1", IP: "10.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("d1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "alice" {
		t.Errorf("username lost: %q", got.Username)
	}
	if got.TokenHash != "hash" {
		t.Errorf("token_hash lost: %q", got.TokenHash)
	}
	if got.IP != "10.0.0.2" {
		t.Errorf("IP not updated: %q", got.IP)
	}
	if !got.Online {
		t.Error("re-Upsert must set Online=true")
	}
}

// TestStore_MarkOffline — exercises the only endpoint that
// writes Online=false. It must (a) update last_seen_at, (b)
// leave the row present, (c) return ErrNotFound for unknown ids.
func TestStore_MarkOffline(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Upsert(Device{DeviceID: "d1", IP: "10.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := s.MarkOffline("d1", now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("d1")
	if got == nil {
		t.Fatal("row vanished after MarkOffline")
	}
	if got.Online {
		t.Error("Online still true after MarkOffline")
	}
	if !got.LastSeenAt.Equal(now) {
		t.Errorf("LastSeenAt = %v, want %v", got.LastSeenAt, now)
	}
	// Unknown id.
	if err := s.MarkOffline("nope", now); err == nil {
		t.Error("MarkOffline(unknown) = nil, want ErrNotFound")
	}
}

// deviceIDFor composes a deterministic id from the goroutine
// index and the per-goroutine counter, so the concurrent test
// hits 16*100 unique primary keys and the unique constraint
// asserts the writes didn't collide.
func deviceIDFor(g, i int) string {
	// Use a 6-digit pair to keep ids short but unique.
	return "dev-" + pad(g, 2) + "-" + pad(i, 4)
}

func pad(n, w int) string {
	s := ""
	for v := n; v > 0 || len(s) < w; v /= 10 {
		s = string(rune('0'+v%10)) + s
	}
	return s
}
