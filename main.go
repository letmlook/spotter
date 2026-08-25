// spotter-client is the cross-platform desktop GUI (Windows, macOS,
// Linux) that discovers spotterd instances and displays their info.
// The Wails options.App below configures all three OS families; the
// active platform is selected at build time via GOOS.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/spotter/spotter/internal/clientconfig"
	"github.com/spotter/spotter/internal/lanscan"
	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
	"github.com/spotter/spotter/internal/timefmt"
)

//go:embed all:frontend/dist
var uiFS embed.FS

// appIcon is the PNG payload used by the macOS "About" dialog. The
// .icns file is generated separately and embedded into the .app bundle
// by Wails via build/darwin/iconfile.icns (see build/darwin/Info.plist).
//
//go:embed build/appicon.png
var appIcon []byte

// Emitter abstracts wailsruntime.EventsEmit so tests can substitute
// a recording fake without spinning up Wails.
type Emitter interface {
	Emit(ctx context.Context, eventName string, data ...interface{})
}

type wailsEmitter struct{}

func (wailsEmitter) Emit(ctx context.Context, eventName string, data ...interface{}) {
	wailsruntime.EventsEmit(ctx, eventName, data...)
}

// listenPort is the device-side HTTP port the agent listens on by
// default (and the port the client expects when polling). Re-exported
// from protocol so call sites that already imported package main keep
// working unchanged.
const listenPort = protocol.DefaultDevicePort

func main() {
	appData, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	dataDir := filepath.Join(appData, "Spotter")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Error("create data directory", slog.String("err", err.Error()))
	}
	logPath := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logPath, 0755); err != nil {
		logger.Error("create log directory", slog.String("err", err.Error()))
	}

	logFile, err := os.OpenFile(filepath.Join(logPath, "spotter.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		logger.Error("open log file", slog.String("err", err.Error()))
	} else {
		defer logFile.Close()
		logger = slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	reg, err := registry.Open(filepath.Join(dataDir, "devices.json"))
	if err != nil {
		logger.Error("open registry", slog.String("err", err.Error()))
		os.Exit(1)
	}
	settings, err := clientconfig.Open(filepath.Join(dataDir, "settings.json"))
	if err != nil {
		logger.Error("open client settings", slog.String("err", err.Error()))
		os.Exit(1)
	}

	app := NewApp(reg, settings, logger, wailsEmitter{})

	err = wails.Run(&options.App{
		Title:  "Spotter",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: uiFS,
		},
		OnStartup: app.OnStartup,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			DisableWindowIcon:    true,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               false,
				FullSizeContent:            false,
			},
			// No explicit Appearance: lets the window follow the macOS
			// system appearance, which also makes the webview's
			// prefers-color-scheme media query reflect the system so
			// the React layer can react to live theme switches.
			WebviewIsTransparent: false,
			About: &mac.AboutInfo{
				Title:   "Spotter",
				Message: "© 2026 Spotter Dev",
				Icon:    appIcon,
			},
		},
		Linux: &linux.Options{
			ProgramName: "Spotter",
		},
		Frameless: true,
		Bind: []interface{}{
			app.settingsApp,
			app.registryApp,
			app.scannerApp,
			app.logStreamApp,
		},
	})
	if err != nil {
		logger.Error("wails run", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

// App is the Wails-bound object. Frontend calls these methods.
// The 14 exposed methods cover four concerns (settings, registry,
// scanner, log-stream); the facade keeps the Wails Bind surface
// flat so the frontend does not have to know about an internal
// split. NewApp wires the dependencies once and the methods stay
// narrow. The eight narrowest methods (GetSettings / SetSettings /
// ListDevices / ClearRegistry / LocalSubnets / RefreshNow /
// StartLogStream / StopLogStream) are delegated to four
// concern-scoped sub-structs; the six methods that need cross-
// concern composition (StartScanner / ScanSubnet / ProbeByIP /
// AcceptUnknownDevice / RebootDevice / ShutdownDevice) stay on
// the facade body — refactoring those into the sub-structs
// would require passing three or more dependencies through the
// same call path for no clarity gain. See PR 5 / PR 8 for the
// per-concern refactors (onScannerEvent dispatch + watchRegistry
// for log-stream lifecycle + Scanner.Close) that already carve
// the largest slices out of this struct.
type App struct {
	settingsApp  *settingsApp
	registryApp  *registryApp
	scannerApp   *scannerApp
	logStreamApp *logStreamApp

	reg     *registry.Registry
	settings *clientconfig.Store
	logger  *slog.Logger
	scanner *scanner.Scanner
	emitter Emitter
	ctx     context.Context // injected via OnStartup; pre-initialised to a non-nil placeholder so EventsEmit is safe before Wails starts.
}

// settingsApp owns the GetSettings / SetSettings surface. No
// mutation outside the user dialog is exposed here.
type settingsApp struct{ s *clientconfig.Store }

func (s *settingsApp) Get() clientconfig.Settings { return s.s.Get() }
func (s *settingsApp) Set(in clientconfig.Settings) error {
	return s.s.Set(in)
}

// registryApp owns the device-registry CRUD surface: List,
// Clear, ProbeByIP, AcceptUnknownDevice, RebootDevice,
// ShutdownDevice. Power actions live here because they need
// the scanner for HTTP calls.
type registryApp struct {
	reg     *registry.Registry
	scanner *scanner.Scanner
	logger  *slog.Logger
}

func (r *registryApp) List() []registry.Entry { return r.reg.List() }
func (r *registryApp) Clear() (int, error) {
	entries := r.reg.List()
	for _, e := range entries {
		if err := r.reg.Remove(e.DeviceID); err != nil {
			return 0, fmt.Errorf("remove %s: %w", e.DeviceID, err)
		}
	}
	r.logger.Info("registry cleared", slog.Int("count", len(entries)))
	return len(entries), nil
}

// powerAction is the shared body of RebootDevice/ShutdownDevice.
func (r *registryApp) powerAction(deviceID, action string) error {
	entry, ok := r.reg.Get(deviceID)
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}
	if !entry.Online {
		return fmt.Errorf("device %s is offline", deviceID)
	}
	port := entry.Port
	if port == 0 {
		port = protocol.DefaultDevicePort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	switch action {
	case "reboot":
		err = r.scanner.RebootDevice(ctx, entry.IP, port)
	case "shutdown":
		err = r.scanner.ShutdownDevice(ctx, entry.IP, port)
	}
	if errors.Is(err, scanner.ErrPowerActionTimeout) {
		r.logger.Info("power action timeout (optimistic success)",
			slog.String("device_id", deviceID),
			slog.String("action", action))
		return nil
	}
	return err
}

// RebootDevice sends a remote reboot command.
func (r *registryApp) RebootDevice(deviceID string) error {
	return r.powerAction(deviceID, "reboot")
}

// ShutdownDevice sends a remote shutdown command.
func (r *registryApp) ShutdownDevice(deviceID string) error {
	return r.powerAction(deviceID, "shutdown")
}

// AcceptUnknownDevice adds a previously-unknown device.
func (r *registryApp) AcceptUnknownDevice(deviceID, ip string, port int, username string) (registry.Entry, error) {
	r.logger.Info("accept-unknown-device called",
		slog.String("device_id", deviceID),
		slog.String("ip", ip),
		slog.Int("port", port),
		slog.String("username", username),
	)
	if deviceID == "" {
		return registry.Entry{}, fmt.Errorf("deviceID required")
	}
	if port == 0 {
		port = protocol.DefaultDevicePort
	}
	e := registry.Entry{
		DeviceID:   deviceID,
		IP:         ip,
		Port:       port,
		Username:   username,
		DeployedAt: timefmt.NowUTC(),
		LastSource: "accept",
		Online:     false,
	}
	created, err := r.reg.Upsert(deviceID, e, nil)
	if err != nil {
		return registry.Entry{}, fmt.Errorf("add to registry: %w", err)
	}
	if !created {
		return registry.Entry{}, fmt.Errorf("device %q already in registry", deviceID)
	}
	// Best-effort immediate poll so the row shows up populated quickly.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := r.scanner.PollOnce(ctx); err != nil {
			r.logger.Debug("immediate poll after accept failed",
				slog.String("err", err.Error()))
		}
	}()
	return e, nil
}

// ProbeByIP fetches /api/v1/info from ip:port and upserts.
func (r *registryApp) ProbeByIP(ip string, port int, username string) (registry.Entry, error) {
	if port == 0 {
		port = protocol.DefaultDevicePort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/v1/info", ip, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return registry.Entry{}, err
	}
	resp, err := r.scanner.HTTPClient().Do(req)
	if err != nil {
		return registry.Entry{}, fmt.Errorf("probe: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return registry.Entry{}, fmt.Errorf("probe: HTTP %d", resp.StatusCode)
	}
	var info protocol.DeviceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return registry.Entry{}, fmt.Errorf("decode: %w", err)
	}
	if info.DeviceID == "" {
		return registry.Entry{}, fmt.Errorf("probe: response missing device_id")
	}
	newEntry := registry.Entry{
		DeviceID:   info.DeviceID,
		IP:         ip,
		Port:       port,
		Username:   username,
		DeployedAt: timefmt.NowUTC(),
		LastSeenAt: timefmt.NowUTC(),
		LastSource: "manual-probe",
		Online:     true,
		LastInfo:   &info,
	}
	created, err := r.reg.Upsert(info.DeviceID, newEntry, func(e *registry.Entry) {
		e.IP = ip
		e.Port = port
		e.LastSeenAt = timefmt.NowUTC()
		e.LastSource = "manual-probe"
		e.Online = true
		e.LastInfo = &info
	})
	if err != nil {
		return registry.Entry{}, fmt.Errorf("upsert: %w", err)
	}
	if created {
		return newEntry, nil
	}
	existing, _ := r.reg.Get(info.DeviceID)
	return existing, nil
}

// scannerApp owns the scan triggers + enumeration: LocalSubnets,
// RefreshNow, StartScanner, ScanSubnet.
type scannerApp struct {
	scanner *scanner.Scanner
	logger  *slog.Logger
}

func (s *scannerApp) LocalSubnets() []string {
	subs := lanscan.LocalSubnets()
	if subs == nil {
		s.logger.Error("list interfaces")
	}
	return subs
}

func (s *scannerApp) RefreshNow(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.scanner.PollOnce(ctx)
}

// StartScanner kicks off the scanner's poll/mcast loops until
// ctx is cancelled. (OnStartup also does this — kept for
// frontends that want explicit lifecycle control.)
func (s *scannerApp) StartScanner(ctx context.Context) {
	s.scanner.Start(ctx)
}

// ScanSubnet triggers a manual subnet scan. If cidr is empty,
// the first non-loopback IPv4 subnet is auto-detected.
func (s *scannerApp) ScanSubnet(cidr string) error {
	if cidr == "" {
		subs := lanscan.LocalSubnets()
		if len(subs) == 0 {
			return fmt.Errorf("no local subnet detected; pass an explicit CIDR")
		}
		cidr = subs[0]
		s.logger.Info("auto-detected local subnet", slog.String("cidr", cidr))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return s.scanner.ScanSubnet(ctx, cidr, 30*time.Second)
}

// logStreamApp owns the per-device log-stream goroutines.
// Holds the registry too because StartLogStream needs to look up
// the entry's IP/port and check Online before launching a stream.
type logStreamApp struct {
	reg     *registry.Registry
	scanner *scanner.Scanner
	logger  *slog.Logger
	emitter func(ctx context.Context, name string, data ...interface{})

	mu      sync.Mutex
	streams map[string]context.CancelFunc
	// streamFn is the body of StartLogStream; injected for tests.
	streamFn func(ctx context.Context, ip string, port int, onLine func(scanner.LogLine)) error
}

// StartLogStream launches a streaming goroutine for deviceID.
// Idempotent: a second call for the same deviceID returns nil
// without spawning a second goroutine.
func (l *logStreamApp) StartLogStream(deviceID string) error {
	entry, ok := l.reg.Get(deviceID)
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}
	if !entry.Online {
		return fmt.Errorf("device %s is offline", deviceID)
	}
	port := entry.Port
	if port == 0 {
		port = protocol.DefaultDevicePort
	}
	l.mu.Lock()
	if _, exists := l.streams[deviceID]; exists {
		l.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.streams[deviceID] = cancel
	l.mu.Unlock()
	go l.run(ctx, deviceID, entry.IP, port)
	return nil
}

// StopLogStream cancels the active stream for deviceID.
func (l *logStreamApp) StopLogStream(deviceID string) error {
	l.stop(deviceID)
	return nil
}

// stop cancels (if any) the active stream for deviceID.
func (l *logStreamApp) stop(deviceID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if cancel, ok := l.streams[deviceID]; ok {
		cancel()
		delete(l.streams, deviceID)
	}
}

// cancelOnRemove is the watcher callback — same as stop but
// kept named for the call-site intent.
func (l *logStreamApp) cancelOnRemove(deviceID string) { l.stop(deviceID) }

// run reads from streamFn, emits each line, and on exit removes
// the stream entry + emits "device-log-end:{id}".
func (l *logStreamApp) run(ctx context.Context, deviceID, ip string, port int) {
	defer func() {
		l.mu.Lock()
		if c, ok := l.streams[deviceID]; ok {
			c()
			delete(l.streams, deviceID)
		}
		l.mu.Unlock()
		l.emitter(ctx, "device-log-end:"+deviceID, true)
	}()
	err := l.streamFn(ctx, ip, port, func(line scanner.LogLine) {
		l.emitter(ctx, "device-log:"+deviceID, line)
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		l.logger.Warn("log stream ended",
			slog.String("device_id", deviceID),
			slog.String("err", err.Error()))
		l.emitter(ctx, "device-log-error:"+deviceID, err.Error())
	}
}

// OnStartup is called by Wails; replaces the placeholder ctx with the
// real Wails-managed one and kicks off the scanner poll/mcast loops.
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	a.scanner.Start(ctx)
	// Close the scanner's registry-watcher when Wails shuts the
	// ctx down. Without this the goroutine outlives the App and
	// only exits when the registry itself is closed.
	go func() {
		<-ctx.Done()
		a.scanner.Close()
	}()
}

// NewApp constructs the App, scanner, and event wiring. Scanner
// events are forwarded as Wails runtime events so the frontend can
// listen with EventsOn. app.ctx is seeded with context.Background()
// so the very first event from the scanner (which may fire before
// OnStartup runs) does not dereference a nil ctx.
func NewApp(reg *registry.Registry, settings *clientconfig.Store, logger *slog.Logger, emitter Emitter) *App {
	if emitter == nil {
		emitter = wailsEmitter{}
	}
	app := &App{
		reg: reg, settings: settings, logger: logger, emitter: emitter,
		ctx: context.Background(),
		settingsApp: &settingsApp{s: settings},
		registryApp: &registryApp{reg: reg, logger: logger},
		scannerApp:  &scannerApp{scanner: nil, logger: logger}, // wired after scanner.New below
		logStreamApp: &logStreamApp{
			reg:     reg,
			scanner: nil, // wired after scanner.New below
			logger:  logger,
			emitter: emitter.Emit,
			streams: map[string]context.CancelFunc{},
			streamFn: nil, // wired after scanner.New below
		},
	}
	s := settings.Get()
	opts := []func(*scanner.Options){
		scanner.WithOnEvent(app.onScannerEvent),
		scanner.WithMulticastGroup(s.MulticastGroup),
		scanner.WithDevicePort(s.DevicePort),
	}
	if s.AuthToken != "" {
		opts = append(opts, scanner.WithAuthToken(s.AuthToken))
	}
	// mDNS browse enables runtime IP-drift handling. Defaults to
	// off; SettingsDialog surfaces a checkbox so operators on
	// networks that block mDNS multicast can keep zeroconf
	// traffic off the wire.
	if s.EnableMDNS {
		opts = append(opts, scanner.WithEnableMDNS())
	}

	tokenSet := "no"
	if s.AuthToken != "" {
		tokenSet = "set"
	}
	logger.Info("scanner config applied",
		slog.String("multicast", s.MulticastGroup),
		slog.Int("port", s.DevicePort),
		slog.String("auth_token", tokenSet),
	)
	app.scanner = scanner.New(reg, opts...)
	app.scannerApp.scanner = app.scanner
	app.logStreamApp.scanner = app.scanner
	// streamFn 默认指向 scanner 实现；测试可覆盖。
	app.logStreamApp.streamFn = func(ctx context.Context, ip string, port int, onLine func(scanner.LogLine)) error {
		return app.scanner.StreamDeviceLogs(ctx, ip, port, "", onLine)
	}

	// Subscribe to registry mutations so logStreams can clean up
	// automatically when a device is removed.
	go app.watchRegistry(reg.Subscribe())
	return app
}

// watchRegistry listens on the registry mutation channel and
// cancels any active log stream whose device is removed.
func (a *App) watchRegistry(ch <-chan registry.MutationEvent) {
	for ev := range ch {
		if ev.Op != registry.OpRemove {
			continue
		}
		a.logStreamApp.cancelOnRemove(ev.DeviceID)
	}
}

// onScannerEvent is the dispatch entry for every scanner event.
// It logs the event and routes to a per-type policy method
// (onDeviceIPDrifted / onUnknownDevice) plus a fallthrough
// forward-to-frontend. Previously a 70-line closure inlined all
// three concerns (state mutation, policy, event forwarding) and
// was hard to unit-test.
func (a *App) onScannerEvent(e scanner.Event) {
	a.logger.Info("scanner event", slog.String("tag", e.Tag()))
	switch ev := e.(type) {
	case scanner.EventDeviceIPDrifted:
		a.onDeviceIPDrifted(ev)
		return
	case scanner.EventUnknownDeviceDiscovered:
		a.onUnknownDevice(ev)
		a.forwardToFrontend(e)
		return
	}
	a.forwardToFrontend(e)
}

// forwardToFrontend emits the event onto the Wails event bus so
// the TS subscriber can react. See frontend/src/hooks/wailsEvents.ts
// for the variadic-args contract — TS unwraps args[0].
func (a *App) forwardToFrontend(e scanner.Event) {
	a.emitter.Emit(a.ctx, e.Tag(), e)
}

// emitPayload is a typed convenience over Emitter.Emit so the
// call site reads as "emit exactly this struct as the event
// payload" instead of "pass it as the third variadic arg".
// The single-payload contract is what makes
// frontend/src/hooks/wailsEvents.ts:subscribe<T>() work.
func emitPayload[T any](a *App, name string, payload T) {
	a.emitter.Emit(a.ctx, name, payload)
}

// onDeviceIPDrifted re-anchors a registered device whose IP
// changed (mDNS-driven). No-ops on empty NewIP (an mDNS browse
// that lost the entry doesn't carry one).
func (a *App) onDeviceIPDrifted(ev scanner.EventDeviceIPDrifted) {
	if ev.NewIP == "" {
		return
	}
	err := a.reg.Update(ev.DeviceID, func(en *registry.Entry) {
		if ev.NewIP != "" {
			en.IP = ev.NewIP
		}
		if ev.NewPort > 0 {
			en.Port = ev.NewPort
		}
		en.LastSource = "mdns"
	})
	if err != nil {
		a.logger.Warn("mdns drift: registry update failed",
			slog.String("device", ev.DeviceID),
			slog.String("err", err.Error()))
		return
	}
	fresh, _ := a.reg.Get(ev.DeviceID)
	a.logger.Info("mdns drift re-anchored",
		slog.String("device", ev.DeviceID),
		slog.String("from", ev.OldIP),
		slog.String("to", ev.NewIP))
	a.emitter.Emit(a.ctx, "info-updated", scanner.EventInfoUpdated{Entry: fresh})
}

// onUnknownDevice upserts an auto-accepted entry for a device
// the registry hasn't seen before (mDNS or subnet scan). On
// create, emit info-updated so the UI's reducer sees a fresh row.
func (a *App) onUnknownDevice(u scanner.EventUnknownDeviceDiscovered) {
	id := u.Info.DeviceID
	if id == "" {
		return
	}
	ip := u.IP
	if ip == "" && u.Info.Network.PrimaryIP != "" {
		ip = u.Info.Network.PrimaryIP
	}
	port := u.Port
	if port == 0 {
		port = listenPort
	}
	entry := registry.Entry{
		DeviceID:   id,
		IP:         ip,
		Port:       port,
		Username:   "",
		DeployedAt: timefmt.NowUTC(),
		LastSource: "accept",
		Online:     true,
	}
	created, err := a.reg.Upsert(id, entry, func(en *registry.Entry) {
		en.IP = ip
		en.Port = port
		en.LastSource = "accept"
		en.Online = true
	})
	if err != nil {
		a.logger.Warn("auto-accept: registry upsert failed",
			slog.String("device", id),
			slog.String("err", err.Error()))
		return
	}
	if created {
		a.logger.Info("auto-accepted new device",
			slog.String("device", id),
			slog.String("ip", ip))
		fresh, _ := a.reg.Get(id)
		a.emitter.Emit(a.ctx, "info-updated", scanner.EventInfoUpdated{Entry: fresh})
	}
}
