// Package scanner discovers devices via three sources (registry poll,
// UDP multicast, manual subnet scan) and merges results into a single
// event stream.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)

// ErrPowerActionTimeout is returned by RebootDevice/ShutdownDevice when
// the HTTP client timed out. Callers (e.g. the Wails App) treat it as
// "the command may have been sent" and surface an optimistic success.
var ErrPowerActionTimeout = errors.New("scanner: power action timed out, device may have responded")

// Event is the union of all scanner-produced events.
type Event interface{ Tag() string }

// EventInfoUpdated fires when a device's info has been refreshed.
type EventInfoUpdated struct{ Entry registry.Entry }

func (EventInfoUpdated) Tag() string { return "info-updated" }

// EventOffline fires when a device has been offline for >= threshold.
type EventOffline struct{ DeviceID string }

func (EventOffline) Tag() string { return "offline" }

// EventUnknownDeviceDiscovered fires when a /info or HELLO-REPLY
// arrives for a device not in the local registry. IP and Port carry
// the source address (multicast HELLO source IP / subnet probe IP);
// they may be zero if the discovery path didn't supply them.
type EventUnknownDeviceDiscovered struct {
	Info protocol.DeviceInfo
	IP   string
	Port int
}

func (EventUnknownDeviceDiscovered) Tag() string { return "unknown-device" }

// Options for configuring a Scanner.
type Options struct {
	HTTPClient    *http.Client
	LogHTTPClient *http.Client // 独立于 HTTPClient（无 read timeout）
	PollInterval  time.Duration
	McastInterval time.Duration
	OnEvent       func(Event)
	Logger        *slog.Logger
	// DevicePort is the spotterd port the subnet scanner probes. Defaults to 9999.
	DevicePort     int
	MulticastGroup string
	ClientSenderID string
}

func (o Options) withDefaults() Options {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 3 * time.Second}
	}
	if o.LogHTTPClient == nil {
		o.LogHTTPClient = &http.Client{Timeout: 0} // 跟随 ctx
	}
	if o.PollInterval == 0 {
		// Short cadence so device online/offline transitions surface in
		// the GUI within ~one interval. The agent also emits HELLO every
		// 5s (see internal/agentd/udp.go), so the worst-case online
		// latency is bounded by this value; offline latency is bounded
		// by PollInterval * pollFailures.threshold (currently 3).
		o.PollInterval = 5 * time.Second
	}
	if o.McastInterval == 0 {
		// Matches the agent's proactive HELLO cadence. The mcast loop
		// both sends a HELLO (to elicit HELLO_REPLY from agents) and
		// passively listens for agent HELLOs.
		o.McastInterval = 5 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.MulticastGroup == "" {
		o.MulticastGroup = "239.255.42.42:9999"
	}
	if o.DevicePort == 0 {
		o.DevicePort = 9999
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

// WithHTTPClient overrides the default HTTP client (used by Scanner.RebootDevice etc.).
func WithHTTPClient(c *http.Client) func(*Options) {
	return func(o *Options) { o.HTTPClient = c }
}

// WithLogHTTPClient overrides the streaming client used by
// Scanner.StreamDeviceLogs. The streaming client should have no read
// timeout so long-lived log streams are not cut off prematurely.
func WithLogHTTPClient(c *http.Client) func(*Options) {
	return func(o *Options) { o.LogHTTPClient = c }
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

// Start runs all discovery loops until ctx is cancelled.
func (s *Scanner) Start(ctx context.Context) {
	go s.pollLoop(ctx)
	go s.mcastLoop(ctx)
}

// MergeForTest exposes the merge pipeline for tests.
func (s *Scanner) MergeForTest(src, ip string, port int, info protocol.DeviceInfo) {
	s.mergeInfo(src, ip, port, info)
}

// McastOnceForTest triggers a single mcast HELLO/REPLY cycle.
func (s *Scanner) McastOnceForTest(ctx context.Context) {
	s.mcastOnce(ctx)
}

// HTTPClient exposes the configured HTTP client for one-off probes
// (e.g. manual IP add from the UI when multicast is blocked).
func (s *Scanner) HTTPClient() *http.Client { return s.opts.HTTPClient }

// RebootDevice POSTs to /api/v1/reboot on the device. Returns
// ErrPowerActionTimeout on client-side timeout (treated as "may have
// succeeded" by callers); other errors are terminal.
func (s *Scanner) RebootDevice(ctx context.Context, ip string, port int) error {
	return s.postPowerAction(ctx, ip, port, "reboot")
}

// ShutdownDevice POSTs to /api/v1/shutdown. Same semantics as RebootDevice.
func (s *Scanner) ShutdownDevice(ctx context.Context, ip string, port int) error {
	return s.postPowerAction(ctx, ip, port, "shutdown")
}

func (s *Scanner) postPowerAction(ctx context.Context, ip string, port int, action string) error {
	target := fmt.Sprintf("http://%s:%d/api/v1/%s", ip, port, action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
	if err != nil {
		return err
	}
	resp, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isHTTPClientTimeout(err) {
			return ErrPowerActionTimeout
		}
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("power actions disabled")
	default:
		return fmt.Errorf("power action %q: unexpected status %d", action, resp.StatusCode)
	}
}

// isHTTPClientTimeout detects the http.Client's own timeout (not
// context-driven). http.Client surfaces it as a *url.Error whose
// Timeout() method returns true.
func isHTTPClientTimeout(err error) bool {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	return false
}
