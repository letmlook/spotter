//go:build linux

// spotterd is the device-side daemon. It runs as a systemd unit on
// Linux devices (arm64 SBCs such as Jetson, plus amd64 servers and
// workstations) and exposes HTTP+UDP endpoints that the client
// polls/discovers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/spotter/spotter/internal/agentd"
	"github.com/spotter/spotter/internal/collector"
	"github.com/spotter/spotter/internal/mdns"
	"github.com/spotter/spotter/internal/protocol"
)

const defaultAgentVersion = "0.1.0"

type tomlConfig struct {
	DeviceID           string        `toml:"device_id"`
	ListenAddr         string        `toml:"listen_addr"`
	MulticastGroup     string        `toml:"multicast_group"`
	AgentVersion       string        `toml:"agent_version"`
	EnablePowerActions bool          `toml:"enable_power_actions"`
	EnableLogStream    bool          `toml:"enable_log_stream"`
	LogUnit            string        `toml:"log_unit"`
	HelloInterval      time.Duration `toml:"hello_interval"`

	Auth   authSection   `toml:"auth"`
	Server serverSection `toml:"server"`
	Logs   logsSection   `toml:"logs"`
}

type authSection struct {
	Enabled bool   `toml:"enabled"`
	Token   string `toml:"token"`
}

type serverSection struct {
	ReadTimeout     time.Duration `toml:"read_timeout"`
	WriteTimeout    time.Duration `toml:"write_timeout"`
	MaxHeaderBytes  int           `toml:"max_header_bytes"`
	PowerActionRate float64       `toml:"power_action_rate_per_sec"`
	LogStreamRate   float64       `toml:"log_stream_rate_per_sec"`
}

type logsSection struct {
	Unit       string `toml:"unit"`
	DefaultTail int   `toml:"default_tail"`
	MaxTail     int   `toml:"max_tail"`
}

func main() {
	var (
		configPath = flag.String("config", "/etc/spotterd/agent.toml", "path to TOML config")
		logLevel   = flag.String("log-level", "info", "log level (debug/info/warn/error)")
	)
	flag.Parse()

	log := newLogger(*logLevel)
	if code := runAgent(*configPath, log); code != 0 {
		os.Exit(code)
	}
}

// runAgent wires the spotterd agent: load config, build the
// agent, run collectors + HTTP + mDNS until ctx is cancelled.
// Returns 0 on graceful shutdown, 1 on a fatal startup error so
// main() can os.Exit. Splitting this out of main() makes the
// startup sequence testable and lets each step have its own
// helper (applyDefaults / buildAgent / installAudit / announceMDNS).
func runAgent(configPath string, log *slog.Logger) int {
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Error("load config", slog.String("err", err.Error()))
		return 1
	}
	applyConfigDefaults(&cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	agent, err := buildAgent(ctx, cfg, log)
	if err != nil {
		log.Error("create agent", slog.String("err", err.Error()))
		return 1
	}
	installAuditLogger(agent, log)

	// On signal: drain the HTTP server (Close) so in-flight
	// /api/v1/info / power-action requests can finish writing
	// their response before we tear down. Without Close the
	// ctx-cancel inside StartHTTP would slam the listener
	// shut mid-response, dropping the body. Close runs with
	// its own 5s grace timeout; Start's blocking call returns
	// as soon as Shutdown drains (or the grace expires).
	go func() {
		<-ctx.Done()
		log.Info("signal received, draining HTTP")
		if err := agent.Close(); err != nil {
			log.Warn("agent close", slog.String("err", err.Error()))
		}
	}()

	log.Info("agent ready",
		slog.String("device_id", cfg.DeviceID),
		slog.String("listen", cfg.ListenAddr),
		slog.String("multicast", cfg.MulticastGroup),
	)

	if err := agent.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("start", slog.String("err", err.Error()))
		return 1
	}
	announceMDNS(ctx, log, cfg.DeviceID, cfg.ListenAddr)
	log.Info("agent stopped")
	return 0
}

// applyConfigDefaults fills the blank cfg fields with the same
// defaults documented in /etc/spotterd/agent.toml.
func applyConfigDefaults(cfg *tomlConfig) {
	if cfg.DeviceID == "" {
		// No default for device_id — the operator must set it,
		// otherwise New() will return errMissing.
		return
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = protocol.DefaultListenAddr
	}
	if cfg.MulticastGroup == "" {
		cfg.MulticastGroup = protocol.DefaultMulticastAddr
	}
	if cfg.AgentVersion == "" {
		cfg.AgentVersion = defaultAgentVersion
	}
}

// buildAgent constructs the *agentd.Agent from cfg, performing the
// initial collector pass and wiring SetCollector for live
// re-collects on every /api/v1/info request.
func buildAgent(ctx context.Context, cfg tomlConfig, log *slog.Logger) (*agentd.Agent, error) {
	agent, err := agentd.New(agentd.Config{
		DeviceID:           cfg.DeviceID,
		ListenAddr:         cfg.ListenAddr,
		MulticastGroup:     cfg.MulticastGroup,
		AgentVersion:       cfg.AgentVersion,
		EnablePowerActions: cfg.EnablePowerActions,
		EnableLogStream:    cfg.EnableLogStream,
		LogUnit:            cfg.LogUnit,
		HelloInterval:      cfg.HelloInterval,
		Auth: agentd.AuthConfig{
			Enabled: cfg.Auth.Enabled,
			Token:   cfg.Auth.Token,
		},
		Server: agentd.ServerConfig{
			ReadTimeout:         cfg.Server.ReadTimeout,
			WriteTimeout:        cfg.Server.WriteTimeout,
			MaxHeaderBytes:      cfg.Server.MaxHeaderBytes,
			PowerActionRatePerS: cfg.Server.PowerActionRate,
			LogStreamRatePerS:   cfg.Server.LogStreamRate,
		},
		Logs: agentd.LogsConfig{
			Unit:        cfg.Logs.Unit,
			DefaultTail: cfg.Logs.DefaultTail,
			MaxTail:     cfg.Logs.MaxTail,
		},
	}, log)
	if err != nil {
		return nil, err
	}

	c := collector.New()
	info, err := c.Collect(ctx)
	if err != nil {
		log.Warn("initial collect", slog.String("err", err.Error()))
	}
	info.DeviceID = cfg.DeviceID
	info.AgentVersion = cfg.AgentVersion
	agent.SetInfo(info)
	// Live-recollect on every /api/v1/info so the GUI sees fresh
	// collected_at and uptime_seconds after a manual refresh.
	agent.SetCollector(c.Collect)
	return agent, nil
}

// installAuditLogger opens the operator-facing audit TSV and wires
// it into the agent. Failure is non-fatal — the agent still runs
// without audit traceability — but the warning is loud so operators
// notice missing disk or wrong permissions.
func installAuditLogger(agent *agentd.Agent, log *slog.Logger) {
	const auditPath = "/var/log/spotterd/audit.tsv"
	if a, err := agentd.NewAuditLogger(auditPath); err == nil {
		agent.SetAuditLogger(a)
		log.Info("audit log open", slog.String("path", auditPath))
	} else {
		log.Warn("audit log unavailable", slog.String("err", err.Error()))
	}
}

// announceMDNS registers the agent under _spotter._tcp via mDNS so
// clients can re-anchor when the device migrates to a different
// subnet. Skipped silently if the listen_addr port can't be parsed.
func announceMDNS(ctx context.Context, log *slog.Logger, deviceID, listenAddr string) {
	port := listenPortFromAddr(listenAddr)
	if port <= 0 {
		log.Warn("skip mDNS announce: invalid listen_addr", slog.String("addr", listenAddr))
		return
	}
	announceAndShutdown(ctx, log, deviceID, port)
}

// listenPortFromAddr extracts the port from "host:port". Defaults to
// 0 on parse error so the caller can skip mDNS cleanly.
func listenPortFromAddr(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	var p int
	if _, err := fmt.Sscanf(portStr, "%d", &p); err != nil {
		return 0
	}
	return p
}

// announceAndShutdown is fire-and-forget; failures are logged at Warn
// rather than fatal so a misconfigured network doesn't prevent the
// agent from running (multicast / poll paths still work).
func announceAndShutdown(ctx context.Context, log *slog.Logger, deviceID string, port int) {
	ann, err := mdns.NewAnnouncer(deviceID, port)
	if err != nil {
		log.Warn("mDNS announce failed", slog.String("err", err.Error()))
		return
	}
	log.Info("mDNS announcing", slog.String("device_id", deviceID), slog.Int("port", port))
	go func() {
		<-ctx.Done()
		ann.Shutdown()
	}()
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
