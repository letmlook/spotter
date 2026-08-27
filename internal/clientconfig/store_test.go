package clientconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// uuidV4Pattern matches the canonical 8-4-4-4-12 hex shape with a
// "4" version nibble in the third group, which is what
// uuid.NewString emits. We only assert the shape, not the variant
// nibble, because we trust google/uuid's generator.
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestStore_OpenMissing(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get().MulticastGroup; got != DefaultMulticastGroup {
		t.Errorf("default MulticastGroup: %q, want %q", got, DefaultMulticastGroup)
	}
	if got := store.Get().DevicePort; got != DefaultDevicePort {
		t.Errorf("default DevicePort: %d, want %d", got, DefaultDevicePort)
	}
}

func TestStore_GetRoundtrip(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "settings.json"))
	want := Settings{
		MulticastGroup: "239.255.42.42:9999",
		DevicePort:     9999,
		Theme:          "light",
		Language:       "en",
		AuthToken:      "tok-1",
	}
	if err := store.Set(want); err != nil {
		t.Fatal(err)
	}
	got := store.Get()
	if got.Theme != "light" || got.Language != "en" || got.AuthToken != "tok-1" {
		t.Errorf("round-trip lost fields: %+v", got)
	}
}

func TestStore_FillsDefaults(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "settings.json"))
	if err := store.Set(Settings{Theme: "dark"}); err != nil { // only theme set
		t.Fatal(err)
	}
	got := store.Get()
	if got.MulticastGroup != DefaultMulticastGroup ||
		got.DevicePort != DefaultDevicePort ||
		got.PollInterval != DefaultPollInterval {
		t.Errorf("defaults not applied: %+v", got)
	}
	if got.Theme != "dark" {
		t.Errorf("Theme override lost: %q", got.Theme)
	}
}

func TestStore_FilePersists0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	store, _ := Open(path)
	if err := store.Set(Settings{AuthToken: "secret"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("file mode: %o, want 0600 (token-bearing file)", mode)
	}
}

func TestStore_CorruptRecovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("expected silent recovery: %v", err)
	}
	if got := store.Get().MulticastGroup; got != DefaultMulticastGroup {
		t.Errorf("expected defaults after recovery: %q", got)
	}
	// Backup must exist.
	entries, _ := os.ReadDir(dir)
	var sawBackup bool
	for _, e := range entries {
		if strings.Contains(e.Name(), ".corrupt-") {
			sawBackup = true
		}
	}
	if !sawBackup {
		t.Error("expected corrupt backup file")
	}
}

func TestStore_Update(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "settings.json"))
	if err := store.Update(func(s *Settings) {
		s.Theme = "light"
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.Get().Theme; got != "light" {
		t.Errorf("Theme: %q", got)
	}
}

func TestSettings_MarshalsAndUnmarshals(t *testing.T) {
	in := Settings{
		MulticastGroup: "239.0.0.1:9999",
		DevicePort:     12345,
		Theme:          "dark",
		Language:       "ja",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Settings
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round-trip mismatch:\nin:  %+v\nout: %+v", in, out)
	}
}

func TestStore_ClientID_Generated(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get().ClientID
	if got == "" {
		t.Fatal("ClientID is empty on first Open; want a UUID v4")
	}
	if !uuidV4Pattern.MatchString(got) {
		t.Errorf("ClientID %q is not a UUID v4", got)
	}
}

func TestStore_ClientID_PersistsAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id1 := first.Get().ClientID
	if id1 == "" {
		t.Fatal("first Open did not generate ClientID")
	}
	// Reopen the same file. Identity must survive — the UUID is
	// what agents use to recognise "the same client" across
	// reconnects, so re-rolling it on every launch would break
	// that contract.
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Get().ClientID; got != id1 {
		t.Errorf("ClientID changed across Open: %q -> %q", id1, got)
	}
}

func TestStore_ClientID_NotOverwrittenBySet(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(filepath.Join(dir, "settings.json"))
	original := store.Get().ClientID
	if err := store.Set(Settings{Theme: "dark"}); err != nil {
		t.Fatal(err)
	}
	if got := store.Get().ClientID; got != original {
		t.Errorf("Set cleared ClientID: %q -> %q", original, got)
	}
}

func TestStore_ClientID_DistinctBetweenTwoOpens(t *testing.T) {
	// Two Open() calls on two different paths must produce two
	// different UUIDs — otherwise the identity is meaningless.
	a, _ := Open(filepath.Join(t.TempDir(), "a.json"))
	b, _ := Open(filepath.Join(t.TempDir(), "b.json"))
	if a.Get().ClientID == b.Get().ClientID {
		t.Errorf("two stores got identical ClientID %q; expected distinct", a.Get().ClientID)
	}
}
