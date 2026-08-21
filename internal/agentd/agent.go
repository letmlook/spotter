package agentd

import (
	"log/slog"
	"sync"

	"github.com/spotter/spotter/internal/protocol"
)

// Config holds the agent's runtime settings.
type Config struct {
	DeviceID       string
	ListenAddr     string
	MulticastGroup string
	AgentVersion   string
}

// Agent owns the cached DeviceInfo and exposes it to the HTTP/UDP layers.
type Agent struct {
	cfg    Config
	logger *slog.Logger
	mu     sync.RWMutex
	info   protocol.DeviceInfo
}

// New constructs an Agent. Returns an error if cfg is missing required fields.
func New(cfg Config, logger *slog.Logger) (*Agent, error) {
	if cfg.DeviceID == "" {
		return nil, errMissing("device_id")
	}
	if cfg.ListenAddr == "" {
		return nil, errMissing("listen_addr")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Agent{cfg: cfg, logger: logger, info: protocol.DeviceInfo{
		SchemaVersion: protocol.SchemaVersion,
		DeviceID:      cfg.DeviceID,
		AgentVersion:  cfg.AgentVersion,
	}}, nil
}

// Config returns the agent's configuration.
func (a *Agent) Config() Config { return a.cfg }

// Logger returns the agent's logger.
func (a *Agent) Logger() *slog.Logger { return a.logger }

// SetInfo atomically replaces the cached DeviceInfo.
func (a *Agent) SetInfo(info protocol.DeviceInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	info.SchemaVersion = protocol.SchemaVersion
	if info.AgentVersion == "" {
		info.AgentVersion = a.cfg.AgentVersion
	}
	a.info = info
}

// Info returns the cached DeviceInfo.
func (a *Agent) Info() protocol.DeviceInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.info
}

type missingFieldError struct{ field string }

func (e *missingFieldError) Error() string { return "agentd: missing field: " + e.field }
func errMissing(f string) error            { return &missingFieldError{f} }
