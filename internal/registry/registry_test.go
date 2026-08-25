package registry_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)

func TestRegistryAddAndList(t *testing.T) {
	dir := t.TempDir()
	r, err := registry.Open(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Add(registry.Entry{
		DeviceID:   "d1",
		IP:         "10.0.0.1",
		Port:       9999,
		Username:   "nvidia",
		DeployedAt: "2026-08-21T00:00:00Z",
		Online:     true,
	}); err != nil {
		t.Fatal(err)
	}

	list := r.List()
	if len(list) != 1 || list[0].DeviceID != "d1" {
		t.Errorf("list: %+v", list)
	}
}

func TestRegistryUpdate(t *testing.T) {
	dir := t.TempDir()
	r, _ := registry.Open(filepath.Join(dir, "devices.json"))
	_ = r.Add(registry.Entry{DeviceID: "d1", IP: "10.0.0.1"})

	err := r.Update("d1", func(e *registry.Entry) {
		e.Online = true
		e.LastSeenAt = "2026-08-21T00:01:00Z"
		e.LastInfo = &protocol.DeviceInfo{DeviceID: "d1"}
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.Get("d1")
	if !ok || !got.Online {
		t.Errorf("update: %+v", got)
	}
}

func TestRegistryPersist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	r, _ := registry.Open(path)
	_ = r.Add(registry.Entry{DeviceID: "d1", IP: "10.0.0.1"})
	r.Close() // flush + release lock

	// Re-open
	r2, err := registry.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := r2.List(); len(got) != 1 {
		t.Errorf("after reopen: %+v", got)
	}
}

func TestRegistryCorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	if err := writeFile(path, "{ this is not json"); err != nil {
		t.Fatal(err)
	}
	r, err := registry.Open(path)
	if err != nil {
		t.Fatalf("expected silent recovery, got: %v", err)
	}
	if got := r.List(); len(got) != 0 {
		t.Errorf("expected empty after recovery, got: %+v", got)
	}
}

func TestRegistryRemove(t *testing.T) {
	dir := t.TempDir()
	r, _ := registry.Open(filepath.Join(dir, "devices.json"))
	_ = r.Add(registry.Entry{DeviceID: "d1"})
	if err := r.Remove("d1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("d1"); ok {
		t.Error("remove failed")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func TestRegistrySubscribe_AddUpdateRemove(t *testing.T) {
	dir := t.TempDir()
	r, err := registry.Open(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	ch := r.Subscribe()

	if err := r.Add(registry.Entry{DeviceID: "a", IP: "10.0.0.1", Port: 9999}); err != nil {
		t.Fatal(err)
	}
	if err := r.Update("a", func(e *registry.Entry) { e.Online = true }); err != nil {
		t.Fatal(err)
	}
	if err := r.Remove("a"); err != nil {
		t.Fatal(err)
	}

	want := []registry.MutationOp{registry.OpAdd, registry.OpUpdate, registry.OpRemove}
	for i, op := range want {
		select {
		case ev := <-ch:
			if ev.Op != op || ev.DeviceID != "a" {
				t.Errorf("event %d: op=%s device=%s, want op=%s device=a", i, ev.Op, ev.DeviceID, op)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}
}

func TestRegistrySubscribe_DropOnSlowConsumer(t *testing.T) {
	dir := t.TempDir()
	r, _ := registry.Open(filepath.Join(dir, "devices.json"))
	ch := r.Subscribe()
	// Fill past the default buffer: the channel has capacity 64, so 65
	// back-to-back mutations must drop at least one without blocking.
	for i := 0; i < 100; i++ {
		if err := r.Add(registry.Entry{DeviceID: fmt.Sprintf("d%d", i), IP: "10.0.0.1"}); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	// Drain in non-blocking fashion and confirm we got "some" but not "all".
	got := 0
	for {
		select {
		case <-ch:
			got++
		default:
			if got >= 64 && got < 100 {
				return
			}
			t.Fatalf("unexpected drain count %d (want 64..99)", got)
		}
	}
}

func TestRegistryFlushOrderStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	r, _ := registry.Open(path)

	// Insert in non-sorted order; flush output must still serialise
	// keys lexicographically so devices.json does not flap on every
	// write.
	for _, id := range []string{"c", "a", "b"} {
		if err := r.Add(registry.Entry{DeviceID: id, IP: "10.0.0.1", Port: 9999}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	ia := strings.Index(content, `"a"`)
	ib := strings.Index(content, `"b"`)
	ic := strings.Index(content, `"c"`)
	if ia < 0 || ib < 0 || ic < 0 {
		t.Fatalf("expected all keys in flush output, got %q", content)
	}
	if !(ia < ib && ib < ic) {
		t.Fatalf("flush not key-sorted: a=%d b=%d c=%d\n%s", ia, ib, ic, content)
	}
}

func TestRegistryFlushOrderStable_AcrossReopens(t *testing.T) {
	// Add three devices, close, reopen. Subsequent Update should NOT
	// reorder keys on disk.
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	r, _ := registry.Open(path)
	for _, id := range []string{"b", "a", "c"} {
		_ = r.Add(registry.Entry{DeviceID: id, IP: "10.0.0.1", Port: 9999})
	}
	r.Close()

	r2, _ := registry.Open(path)
	if err := r2.Update("b", func(e *registry.Entry) { e.Online = true }); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	ia := strings.Index(content, `"a"`)
	ib := strings.Index(content, `"b"`)
	ic := strings.Index(content, `"c"`)
	if !(ia < ib && ib < ic) {
		t.Fatalf("reopen+update broke sort: a=%d b=%d c=%d\n%s", ia, ib, ic, content)
	}
}

func TestRegistryRemove_NoEntryIsNoop(t *testing.T) {
	dir := t.TempDir()
	r, _ := registry.Open(filepath.Join(dir, "devices.json"))
	// No row to remove; ClearRegistry-style flows iterate List() and
	// remove each by id — must not error.
	if err := r.Remove("nonexistent"); err != nil {
		t.Fatalf("remove nonexistent should be no-op: %v", err)
	}
}

func TestRegistryCorruptRecovery_LogsButContinues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "devices.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := registry.Open(path)
	if err != nil {
		t.Fatalf("expected silent recovery, got: %v", err)
	}
	if got := r.List(); len(got) != 0 {
		t.Errorf("expected empty after recovery, got %d entries", len(got))
	}
	// After recovery, the file should have been rewritten to "{}".
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Errorf("expected reset file content %q, got %q", "{}", string(data))
	}
}

// TestRegistryUpsert_Create covers the create-branch: device
// unknown → new entry inserted → created=true → broadcast
// OpAdd. Pins the newEntry-by-value contract; the call site
// never mutates the stored pointer until the broadcast fires.
func TestRegistryUpsert_Create(t *testing.T) {
	r, err := registry.Open(filepath.Join(t.TempDir(), "reg.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	got, err := r.Upsert("new-id", registry.Entry{
		DeviceID: "new-id",
		IP:       "10.0.0.5",
		Port:     9999,
	}, func(*registry.Entry) {})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !got {
		t.Errorf("created = false, want true on first Upsert")
	}
	entry, ok := r.Get("new-id")
	if !ok {
		t.Fatal("Get(new-id) after Upsert returned !ok")
	}
	if entry.IP != "10.0.0.5" || entry.Port != 9999 {
		t.Errorf("entry = %+v, want IP=10.0.0.5 Port=9999", entry)
	}
}

// TestRegistryUpsert_Update covers the update-branch: device
// already known → mutator applied → created=false → broadcast
// OpUpdate. The mutator must see the *current* entry (not a
// zero value) and changes must round-trip.
func TestRegistryUpsert_Update(t *testing.T) {
	r, _ := registry.Open(filepath.Join(t.TempDir(), "reg.json"))
	defer r.Close()
	_ = r.Add(registry.Entry{DeviceID: "d1", IP: "10.0.0.1", Port: 9999, Online: false})

	got, err := r.Upsert("d1", registry.Entry{DeviceID: "d1", IP: "10.0.0.99", Port: 9999}, func(e *registry.Entry) {
		e.Online = true
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got {
		t.Errorf("created = true on second Upsert, want false")
	}
	entry, _ := r.Get("d1")
	// Upsert on update branch only calls the mutator — newEntry
	// is ignored (that's the contract: caller picks whether to
	// overwrite via the mutator body).
	if entry.IP != "10.0.0.1" {
		t.Errorf("IP = %q, want 10.0.0.1 (untouched by update-branch mutator)", entry.IP)
	}
	if !entry.Online {
		t.Errorf("Online = false, want true (mutator applied)")
	}
}

// TestRegistryUpsert_BroadcastOp exercises the mutation event
// delivery — Upsert must emit OpAdd on create and OpUpdate on
// update (no OpRemove). Without this the scanner's pollFailures
// tracker wouldn't know to reset its counter for the device.
func TestRegistryUpsert_BroadcastOp(t *testing.T) {
	r, _ := registry.Open(filepath.Join(t.TempDir(), "reg.json"))
	defer r.Close()

	ch := r.Subscribe()

	// Create → OpAdd
	_, _ = r.Upsert("d1", registry.Entry{DeviceID: "d1", IP: "10.0.0.1"}, nil)
	// Update → OpUpdate
	_, _ = r.Upsert("d1", registry.Entry{DeviceID: "d1", IP: "10.0.0.2"}, func(e *registry.Entry) { e.Online = true })

	var ops []string
	deadline := time.After(300 * time.Millisecond)
collect:
	for {
		select {
		case ev := <-ch:
			ops = append(ops, string(ev.Op))
			if len(ops) >= 2 {
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	if len(ops) < 2 {
		t.Fatalf("got %d events, want ≥2: %v", len(ops), ops)
	}
	if ops[0] != "add" || ops[1] != "update" {
		t.Errorf("ops = %v, want [add update]", ops)
	}
}
