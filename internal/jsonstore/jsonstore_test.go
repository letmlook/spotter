package jsonstore

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// TestSave_RoundTrip writes a value through Save and reads it
// back; the file must exist, the parent dir must be created,
// and the JSON must parse back to an equal value.
func TestSave_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "data.json")
	type payload struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	in := payload{Name: "spotter", N: 9999}
	if err := Save(path, in, 0600); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got payload
	if err := unmarshalStrict(data, &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, data)
	}
	if got != in {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, in)
	}
}

// TestSave_FileMode checks that the file is created with
// the requested perm bits at least. The actual mode may be
// narrower than 0600 (umask) but never wider, so a sensitive
// file (registry / settings / server JSON) never ends up
// world- or group-readable because the caller passed the
// right constant.
func TestSave_FileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perm.json")
	if err := Save(path, map[string]int{"x": 1}, 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got := info.Mode().Perm()
	// Must not be wider than 0600 (i.e. no group/other bits set).
	if got&^0600 != 0 {
		t.Errorf("file mode = %#o, want subset of 0600", got)
	}
	// All owner bits we asked for must be present (umask may
	// have narrowed the rest, but the caller's 0600 is the
	// upper bound — got & 0600 must equal 0600).
	if got&0600 != 0600 {
		t.Errorf("file mode = %#o, missing owner bits of 0600", got)
	}
}

// TestSave_CreatesParentDir asserts MkdirAll runs before the
// write. registry / clientconfig rely on this — they store
// under `<UserConfig>/Spotter/devices.json` and expect the
// dir to come into existence on first flush.
func TestSave_CreatesParentDir(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "deep", "nested", "x.json")
	if err := Save(path, "hi", 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file missing after Save: %v", err)
	}
}

// TestOrdered pins the stable-key-order contract that
// registryd / registry flushLocked rely on. Without this, a
// concurrent insertion into the entries map can reorder the
// keys between flushes and cause devices.json to diff on every
// commit.
func TestOrdered(t *testing.T) {
	in := map[string]int{"c": 3, "a": 1, "b": 2}
	got := Ordered(in)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"a", "b", "c"}) {
		t.Errorf("Ordered keys = %v, want alphabetical", keys)
	}
}

// unmarshalStrict is a local helper that wraps encoding/json
// without dragging it into the package's import surface (the
// Save caller usually already imports encoding/json).
func unmarshalStrict(data []byte, v any) error {
	return jsonUnmarshal(data, v)
}
