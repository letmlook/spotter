// Package scanner discovers devices via three sources (registry poll,
// UDP multicast, manual subnet scan) and merges results into a single
// event stream.
package scanner

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)

// Event is the union of all scanner-produced events.
type Event interface{ Tag() string }

// EventInfoUpdated fires when a device's info has been refreshed.
type EventInfoUpdated struct{ Entry registry.Entry }

func (EventInfoUpdated) Tag() string { return "info-updated" }

// EventOffline fires when a device has been offline for >= threshold.
type EventOffline struct{ DeviceID string }

func (EventOffline) Tag() string { return "offline" }

// EventUnknownDeviceDiscovered fires when a /info or HELLO-REPLY
// arrives for a device not in the local registry.
type EventUnknownDeviceDiscovered struct{ Info protocol.DeviceInfo }

func (EventUnknownDeviceDiscovered) Tag() string { return "unknown-device" }

// Options for configuring a Scanner.
type Options struct {
	HTTPClient     *http.Client
	PollInterval   time.Duration
	McastInterval  time.Duration
	OnEvent        func(Event)
	Logger         *slog.Logger
	MulticastGroup string
	ClientSenderID string
}

func (o Options) withDefaults() Options {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 3 * time.Second}
	}
	if o.PollInterval == 0 {
		o.PollInterval = 30 * time.Second
	}
	if o.McastInterval == 0 {
		o.McastInterval = 60 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.MulticastGroup == "" {
		o.MulticastGroup = "239.255.42.42:9999"
	}
	if o.ClientSenderID == "" {
		o.ClientSenderID = "spotter-client"
	}
	return o
}

// WithOnEvent is a convenience option.
func WithOnEvent(fn func(Event)) func(*Options) {
	return func(o *Options) { o.OnEvent = fn }
}

// Scanner runs the three discovery loops.
type Scanner struct {
	reg       *registry.Registry
	opts      Options
	failTrack *pollFailures
}

// New creates a Scanner.
func New(reg *registry.Registry, optFns ...func(*Options)) *Scanner {
	opts := Options{}.withDefaults()
	for _, fn := range optFns {
		fn(&opts)
	}
	return &Scanner{reg: reg, opts: opts, failTrack: newPollFailures(3)}
}

func (s *Scanner) emit(e Event) {
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(e)
	}
}

// timeNowUTC returns the current UTC time in RFC3339 format.
func timeNowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
