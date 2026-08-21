//go:build linux

// spotterd is the device-side daemon. It runs as a systemd unit on
// Linux ARM64 devices (Jetson, Ubuntu Server, etc.) and exposes
// HTTP+UDP endpoints that the Windows client polls/discovers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/BurntSushi/toml"

	"github.com/spotter/spotter/internal/agentd"
	"github.com/spotter/spotter/internal/collector"
)

const defaultAgentVersion = "0.1.0"
const defaultListenAddr = "0.0.0.0:9999"
const defaultMulticastGroup = "239.255.42.42:9999"

type tomlConfig struct {
	DeviceID       string `toml:"device_id"`
	ListenAddr     string `toml:"listen_addr"`
	MulticastGroup string `toml:"multicast_group"`
	AgentVersion   string `toml:"agent_version"`
}

func main() {
	var (
		configPath = flag.String("config", "/etc/spotterd/agent.toml", "path to TOML config")
		logLevel   = flag.String("log-level", "info", "log level (debug/info/warn/error)")
	)
	flag.Parse()

	log := newLogger(*logLevel)
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Error("load config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	if cfg.DeviceID == "" {
		log.Error("config missing device_id")
		os.Exit(1)
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	if cfg.MulticastGroup == "" {
		cfg.MulticastGroup = defaultMulticastGroup
	}
	if cfg.AgentVersion == "" {
		cfg.AgentVersion = defaultAgentVersion
	}

	agent, err := agentd.New(agentd.Config{
		DeviceID:       cfg.DeviceID,
		ListenAddr:     cfg.ListenAddr,
		MulticastGroup: cfg.MulticastGroup,
		AgentVersion:   cfg.AgentVersion,
	}, log)
	if err != nil {
		log.Error("create agent", slog.String("err", err.Error()))
		os.Exit(1)
	}

	// Cross-platform signal handling:
	//   - os.Interrupt maps to SIGINT on Unix and CTRL+C on Windows.
	//   - syscall.SIGTERM is included for systemd. On Windows it is
	//     converted to os.Interrupt semantics by the runtime when the
	//     process is killed, but adding it is harmless.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Collect initial info.
	c := collector.New()
	info, err := c.Collect(ctx)
	if err != nil {
		log.Warn("initial collect", slog.String("err", err.Error()))
	}
	info.DeviceID = cfg.DeviceID
	info.AgentVersion = cfg.AgentVersion
	agent.SetInfo(info)
	log.Info("agent ready",
		slog.String("device_id", cfg.DeviceID),
		slog.String("listen", cfg.ListenAddr),
		slog.String("multicast", cfg.MulticastGroup),
	)

	if err := agent.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("start", slog.String("err", err.Error()))
		os.Exit(1)
	}
	log.Info("agent stopped")
}

// loadConfig reads the TOML config from path. A missing file is not an
// error (caller is expected to apply defaults); malformed TOML is.
func loadConfig(path string) (tomlConfig, error) {
	var c tomlConfig
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, fmt.Errorf("stat %s: %w", path, err)
	}
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return c, fmt.Errorf("decode %s: %w", path, err)
	}
	return c, nil
}

// newLogger returns a slog.Logger that writes JSON to stdout at the
// requested level. Unknown / empty levels default to info.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
