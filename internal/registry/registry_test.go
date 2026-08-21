package registry_test

import (
	"os"
	"path/filepath"
	"testing"

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
