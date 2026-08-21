//go:build linux

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.toml")
	body := `
device_id = "jetson-01"
listen_addr = "0.0.0.0:8080"
multicast_group = "239.255.42.42:9999"
agent_version = "0.2.0"
`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.DeviceID != "jetson-01" {
		t.Errorf("DeviceID: got %q", got.DeviceID)
	}
	if got.ListenAddr != "0.0.0.0:8080" {
		t.Errorf("ListenAddr: got %q", got.ListenAddr)
	}
	if got.MulticastGroup != "239.255.42.42:9999" {
		t.Errorf("MulticastGroup: got %q", got.MulticastGroup)
	}
	if got.AgentVersion != "0.2.0" {
		t.Errorf("AgentVersion: got %q", got.AgentVersion)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.toml")
	got, err := loadConfig(path)
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if got.DeviceID != "" || got.ListenAddr != "" {
		t.Errorf("expected empty config, got %+v", got)
	}
}

func TestLoadConfigMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	// Unterminated string — BurntSushi/toml should reject.
	body := `device_id = "oops`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Error("expected error for malformed TOML, got nil")
	}
}

func TestNewLoggerLevels(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, c := range cases {
		got := newLogger(c.in)
		// JSON handler min level is opaque; instead inspect via Level().Level().
		if !got.Enabled(nil, c.want) {
			t.Errorf("newLogger(%q): expected level %s to be enabled", c.in, c.want)
		}
		// debug should also be enabled in debug mode, info should not be enabled in error mode
		if c.in == "error" && got.Enabled(nil, slog.LevelInfo) {
			t.Errorf("newLogger(error): info should be disabled")
		}
	}
	// Smoke: the logger must marshal to JSON without error.
	l := newLogger("info")
	if l == nil {
		t.Fatal("newLogger returned nil")
	}
	// JSON handler writes to stdout; just verify no panic on Info.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Info panicked: %v", r)
		}
	}()
	l.Info("smoke", slog.String("k", "v"))
	_ = strings.HasPrefix
}
