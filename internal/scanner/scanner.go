// Package scanner discovers devices via three sources (registry poll,
// UDP multicast, manual subnet scan) and merges results into a single
// event stream.
package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/spotter/spotter/internal/mdns"
	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/timefmt"
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

// EventDeviceIPDrifted fires when mDNS reports a known device at a
// new IP/Port (e.g. after it migrated to a different subnet). The
// App layer catches this and updates the registry in place, then
// emits EventInfoUpdated so the UI shows the new anchor.
type EventDeviceIPDrifted struct {
	DeviceID string
	OldIP    string
	NewIP    string
	NewPort  int
}

func (EventDeviceIPDrifted) Tag() string { return "device-ip-drifted" }

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
	// AuthToken, when non-empty, is sent as `Authorization: Bearer <token>`
	// on every HTTP request (poll, subnet probe, log stream, power actions).
	// Empty by default; opt-in via WithAuthToken.
	AuthToken string
	// EnableMDNS, when true, starts an mDNS browse loop inside Start().
	// Discovered devices whose IP/Port differ from the registry are
	// reported via EventDeviceIPDrifted so the App layer can re-anchor.
	EnableMDNS bool
}

// WithEnableMDNS opts the scanner into browsing mDNS / DNS-SD
// announcements alongside its poll/mcast loops.
func WithEnableMDNS() func(*Options) {
	return func(o *Options) { o.EnableMDNS = true }
}

// WithAuthToken sets the bearer token used for Authorization headers.
func WithAuthToken(token string) func(*Options) {
	return func(o *Options) { o.AuthToken = token }
}

// newRequest is the single source for HTTP requests issued by the
// scanner; centralising here lets us stamp the Authorization header
// once instead of remembering to do it at every call site. method
// is the HTTP verb; url is the absolute endpoint.
func (s *Scanner) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if s.opts.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.opts.AuthToken)
	}
	return req, nil
}

func (o Options) withDefaults() Options {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 3 * time.Second}
	}
	if o.LogHTTPClient == nil {
		o.LogHTTPClient = &http.Client{Timeout: 0} // 跟随 ctx
	}
	if o.PollInterval == 0 {
		// Cadence so device online/offline transitions surface in
		// the GUI within ~one interval. The agent also emits HELLO
		// every McastInterval, so worst-case online latency is
		// bounded by this value; offline latency is bounded by
		// PollInterval * pollFailures.threshold (currently 3).
		o.PollInterval = protocol.DefaultPollInterval
	}
	if o.McastInterval == 0 {
		// Matches the agent's proactive HELLO cadence. The mcast
		// loop both sends a HELLO (to elicit HELLO_REPLY from
		// agents) and passively listens for agent HELLOs.
		o.McastInterval = protocol.DefaultMcastInterval
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.MulticastGroup == "" {
		o.MulticastGroup = protocol.DefaultMulticastAddr
	}
	if o.DevicePort == 0 {
		o.DevicePort = protocol.DefaultDevicePort
	}
	if o.ClientSenderID == "" {
		// Last-resort fallback for callers that build a Scanner
		// without a settings-backed identity (e.g. spotter-cli
		// smoke tests, ad-hoc probes). The GUI path always sets
		// this via WithClientSenderID(settings.ClientID), so
		// end-users never see this branch.
		o.ClientSenderID = "spotter-client"
	}
	return o
}

// WithOnEvent is a convenience option.
func WithOnEvent(fn func(Event)) func(*Options) {
	return func(o *Options) { o.OnEvent = fn }
}

// WithMulticastGroup overrides the default 239.255.42.42:9999.
func WithMulticastGroup(group string) func(*Options) {
	return func(o *Options) { o.MulticastGroup = group }
}

// WithDevicePort overrides the default 9999.
func WithDevicePort(port int) func(*Options) {
	return func(o *Options) { o.DevicePort = port }
}

// WithClientSenderID stamps every outbound HELLO and X-Spotter-Client
// header with the supplied identifier. Callers are expected to pass a
// stable UUID v4 (see internal/clientconfig.Settings.ClientID) so
// agents can attribute HELLO bursts to the same client across
// reconnects. An empty value is allowed and the scanner will fall
// back to a generic placeholder — see Options.applyDefaults.
func WithClientSenderID(id string) func(*Options) {
	return func(o *Options) { o.ClientSenderID = id }
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
	stop      chan struct{} // closed by Close to terminate watchRegistry
	stopOnce  sync.Once
}

// New creates a Scanner and starts a goroutine that mirrors registry
// mutations into its own pollFailures tracker. Without this watcher,
// ClearRegistry would leave stale failure counts: the next PollOnce
// after a clear would still see the device in the registry-poll
// source, but with a count that may already exceed the offline
// threshold.
func New(reg *registry.Registry, optFns ...func(*Options)) *Scanner {
	opts := Options{}.withDefaults()
	for _, fn := range optFns {
		fn(&opts)
	}
	s := &Scanner{
		reg:       reg,
		opts:      opts,
		failTrack: newPollFailures(3),
		stop:      make(chan struct{}),
	}
	go s.watchRegistry(reg.Subscribe())
	return s
}

// Close terminates the registry-watcher goroutine. Idempotent.
// Safe to call multiple times. The Scanner cannot be re-started
// after Close — create a new one if needed.
func (s *Scanner) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// watchRegistry consumes MutationEvents from ch and resets
// pollFailures counts for removed devices. Exits when ch is
// closed (registry.Close) OR when s.stop is closed (Scanner.Close).
func (s *Scanner) watchRegistry(ch <-chan registry.MutationEvent) {
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			switch ev.Op {
			case registry.OpRemove:
				s.failTrack.reset(ev.DeviceID)
				s.opts.Logger.Debug("scan: registry cleared device, reset fail count",
					slog.String("device", ev.DeviceID))
			}
		case <-s.stop:
			return
		}
	}
}

func (s *Scanner) emit(e Event) {
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(e)
	}
}

// timeNowUTC returns the current UTC time in RFC3339 format.
// Thin wrapper around timefmt.NowUTC — kept because every callsite
// used the longer name and we did not want to do a token-by-token
// rename on top of the import in this PR.
func timeNowUTC() string { return timefmt.NowUTC() }

// Start runs all discovery loops until ctx is cancelled.
func (s *Scanner) Start(ctx context.Context) {
	go s.pollLoop(ctx)
	go s.mcastLoop(ctx)
	if s.opts.EnableMDNS {
		go s.watchMdns(ctx)
	}
}

// watchMdns subscribes to mDNS browse results and emits
// EventDeviceIPDrifted for any device in the registry whose IP/Port
// has changed since the last announcement. Unknown devices (those
// not yet in the registry) are silently dropped — the standard
// UnknownDevice flow handles introduction.
func (s *Scanner) watchMdns(ctx context.Context) {
	b, err := mdns.NewBrowser()
	if err != nil {
		s.opts.Logger.Debug("mdns: browser init failed", "err", err.Error())
		return
	}
	type lastSeen struct{ addr string; port int }
	seen := make(map[string]lastSeen)
	if err := b.Start(ctx, func(e mdns.ServiceEntry) {
		cur, ok := s.reg.Get(e.DeviceID)
		if !ok {
			return // unknown device; ignore
		}
		// Coalesce repeated announcements of the same IP/Port.
		prev, dup := seen[e.DeviceID]
		if dup && prev.addr == e.Addr && prev.port == e.Port {
			return
		}
		seen[e.DeviceID] = lastSeen{addr: e.Addr, port: e.Port}
		if cur.IP == e.Addr && cur.Port == e.Port {
			return
		}
		s.emit(EventDeviceIPDrifted{
			DeviceID: e.DeviceID,
			OldIP:    cur.IP,
			NewIP:    e.Addr,
			NewPort:  e.Port,
		})
	}); err != nil {
		s.opts.Logger.Debug("mdns: browse start failed", "err", err.Error())
	}
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

// FetchPowerAuditRecent pulls /api/v1/power/audit/recent?limit=N
// from a device. The endpoint is a small JSON object
// `{entries: [...], count: N}`; we forward `entries` to the
// GUI as []map[string]any so wailsjs can decode it without a
// pre-generated model. Network / non-2xx errors propagate to
// the caller; the Wails layer above maps them to a friendly
// empty list.
func (s *Scanner) FetchPowerAuditRecent(ctx context.Context, ip string, port int, limit int) ([]map[string]any, error) {
	target := fmt.Sprintf("http://%s:%d/api/v1/power/audit/recent", ip, port)
	if limit > 0 {
		target += "?limit=" + strconv.Itoa(limit)
	}
	req, err := s.newRequest(ctx, http.MethodGet, target)
	if err != nil {
		return nil, err
	}
	resp, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		// Agent's audit logger is nil — return empty, not error,
		// so the GUI renders "audit unavailable" gracefully.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audit recent: %s", resp.Status)
	}
	var body struct {
		Entries []map[string]any `json:"entries"`
		Count   int              `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode audit: %w", err)
	}
	if body.Entries == nil {
		return []map[string]any{}, nil
	}
	return body.Entries, nil
}

func (s *Scanner) postPowerAction(ctx context.Context, ip string, port int, action string) error {
	target := fmt.Sprintf("http://%s:%d/api/v1/%s", ip, port, action)
	req, err := s.newRequest(ctx, http.MethodPost, target)
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
	case http.StatusUnauthorized:
		return fmt.Errorf("power action %q: token required", action)
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
