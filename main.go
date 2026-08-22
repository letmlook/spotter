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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
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
// default (and the port the client expects when polling).
const listenPort = 9999

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

	app := NewApp(reg, logger, wailsEmitter{})

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
			Appearance:           mac.NSAppearanceNameDarkAqua,
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
			app,
		},
	})
	if err != nil {
		logger.Error("wails run", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

// App is the Wails-bound object. Frontend calls these methods.
type App struct {
	reg          *registry.Registry
	logger       *slog.Logger
	scanner      *scanner.Scanner
	emitter      Emitter
	ctx          context.Context // injected via OnStartup; pre-initialised to a non-nil placeholder so EventsEmit is safe before Wails starts.
	logStreams   map[string]context.CancelFunc
	logStreamsMu sync.Mutex
	// streamFn is the body of StartLogStream; injected for tests.
	streamFn func(ctx context.Context, ip string, port int, onLine func(scanner.LogLine)) error
}

// OnStartup is called by Wails; replaces the placeholder ctx with the
// real Wails-managed one and kicks off the scanner poll/mcast loops.
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx
	a.scanner.Start(ctx)
}

// NewApp constructs the App, scanner, and event wiring. Scanner
// events are forwarded as Wails runtime events so the frontend can
// listen with EventsOn. app.ctx is seeded with context.Background()
// so the very first event from the scanner (which may fire before
// OnStartup runs) does not dereference a nil ctx.
func NewApp(reg *registry.Registry, logger *slog.Logger, emitter Emitter) *App {
	if emitter == nil {
		emitter = wailsEmitter{}
	}
	app := &App{
		reg: reg, logger: logger, emitter: emitter,
		ctx:        context.Background(),
		logStreams: map[string]context.CancelFunc{},
	}
	app.scanner = scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		logger.Info("scanner event", slog.String("tag", e.Tag()))
		app.emitter.Emit(app.ctx, e.Tag(), e)
	}))
	// streamFn 默认指向 scanner 实现；测试可覆盖。
	app.streamFn = app.scanner.StreamDeviceLogs
	return app
}

// StartScanner begins the poll + mcast loops. The frontend calls this
// once at startup. The ctx is canceled when the Wails app exits.
// (OnStartup already starts the scanner; this binding is kept for
// frontends that want to control the lifecycle explicitly.)
func (a *App) StartScanner(ctx context.Context) {
	a.scanner.Start(ctx)
}

// ListDevices returns the registry snapshot for the UI.
func (a *App) ListDevices() []registry.Entry {
	return a.reg.List()
}

// ScanSubnet triggers a manual subnet scan. If cidr is empty, the
// first non-loopback IPv4 subnet is auto-detected from the host's
// network interfaces (RFC1918 ranges preferred). Pass an explicit
// CIDR to scan something else.
func (a *App) ScanSubnet(cidr string) error {
	if cidr == "" {
		subnets := a.LocalSubnets()
		if len(subnets) == 0 {
			return fmt.Errorf("no local subnet detected; pass an explicit CIDR")
		}
		cidr = subnets[0]
		a.logger.Info("auto-detected local subnet", slog.String("cidr", cidr))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return a.scanner.ScanSubnet(ctx, cidr, 30*time.Second)
}

// LocalSubnets returns the CIDRs of all non-loopback, up IPv4
// interfaces on the host, ordered with RFC1918 ranges first
// (10/8, 172.16/12, 192.168/16) so the most likely LAN segment is
// at index 0. Link-local 169.254/16 is filtered out. The GUI uses
// this to pre-fill / auto-trigger subnet scans.
func (a *App) LocalSubnets() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		a.logger.Error("list interfaces", slog.String("err", err.Error()))
		return nil
	}
	var cidrs []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			ip := ipnet.IP.To4()
			// Skip link-local (169.254/16).
			if ip[0] == 169 && ip[1] == 254 {
				continue
			}
			ones, _ := ipnet.Mask.Size()
			cidrs = append(cidrs, fmt.Sprintf("%s/%d", ip.Mask(ipnet.Mask), ones))
		}
	}
	sort.SliceStable(cidrs, func(i, j int) bool {
		return rfc1918Rank(cidrs[i]) < rfc1918Rank(cidrs[j])
	})
	return cidrs
}

// rfc1918Rank returns 0 for RFC1918 (LAN), 1 for everything else.
// Used by LocalSubnets to put the LAN subnet first.
func rfc1918Rank(cidr string) int {
	ip := net.ParseIP(cidr[:strings.IndexByte(cidr, '/')])
	if ip == nil {
		return 1
	}
	switch {
	case ip[0] == 10:
		return 0
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return 0
	case ip[0] == 192 && ip[1] == 168:
		return 0
	}
	return 1
}

// RefreshNow forces an immediate poll cycle.
func (a *App) RefreshNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return a.scanner.PollOnce(ctx)
}

// ProbeByIP fetches /api/v1/info from ip:port and, if the device is
// not already registered, adds it to the registry. Returns the new
// entry. Useful for known-IP setups where UDP multicast is blocked.
func (a *App) ProbeByIP(ip string, port int, username string) (registry.Entry, error) {
	if port == 0 {
		port = listenPort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/v1/info", ip, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return registry.Entry{}, err
	}
	resp, err := a.scanner.HTTPClient().Do(req)
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
	// Already registered? Just refresh.
	if existing, ok := a.reg.Get(info.DeviceID); ok {
		a.reg.Update(info.DeviceID, func(e *registry.Entry) {
			e.IP = ip
			e.Port = port
			e.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
			e.LastSource = "manual-probe"
			e.Online = true
			e.LastInfo = &info
		})
		return existing, nil
	}
	e := registry.Entry{
		DeviceID:   info.DeviceID,
		IP:         ip,
		Port:       port,
		Username:   username,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
		LastSeenAt: time.Now().UTC().Format(time.RFC3339),
		LastSource: "manual-probe",
		Online:     true,
		LastInfo:   &info,
	}
	if err := a.reg.Add(e); err != nil {
		return registry.Entry{}, fmt.Errorf("add: %w", err)
	}
	return e, nil
}

// AcceptUnknownDevice adds a previously-unknown device (discovered
// via UDP multicast HELLO or subnet scan) to the local registry, so
// subsequent polls start tracking it. Returns the new entry.
func (a *App) AcceptUnknownDevice(deviceID string, ip string, port int, username string) (registry.Entry, error) {
	if deviceID == "" {
		return registry.Entry{}, fmt.Errorf("deviceID required")
	}
	if port == 0 {
		port = listenPort
	}
	if _, ok := a.reg.Get(deviceID); ok {
		return registry.Entry{}, fmt.Errorf("device %q already in registry", deviceID)
	}
	e := registry.Entry{
		DeviceID:   deviceID,
		IP:         ip,
		Port:       port,
		Username:   username,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
		LastSource: "accept",
		Online:     false,
	}
	if err := a.reg.Add(e); err != nil {
		return registry.Entry{}, fmt.Errorf("add to registry: %w", err)
	}
	// Best-effort immediate poll so the row shows up populated quickly.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.scanner.PollOnce(ctx)
	}()
	return e, nil
}

// ClearRegistry removes every entry from the local registry. Returns
// the number of entries removed.
func (a *App) ClearRegistry() (int, error) {
	entries := a.reg.List()
	for _, e := range entries {
		if err := a.reg.Remove(e.DeviceID); err != nil {
			return 0, fmt.Errorf("remove %s: %w", e.DeviceID, err)
		}
	}
	a.logger.Info("registry cleared", slog.Int("count", len(entries)))
	return len(entries), nil
}

// RebootDevice sends a remote reboot command to the device identified
// by deviceID. Returns an error if the device is not in the registry
// or is marked offline. A client-side HTTP timeout is treated as
// success — the command may have been dispatched before the agent's
// connection hung up during reboot.
func (a *App) RebootDevice(deviceID string) error {
	return a.powerAction(deviceID, "reboot")
}

// ShutdownDevice sends a remote shutdown command. Same semantics as
// RebootDevice. Note: there is no remote power-on; the device must be
// physically powered back on.
func (a *App) ShutdownDevice(deviceID string) error {
	return a.powerAction(deviceID, "shutdown")
}

// powerAction is the shared body of RebootDevice/ShutdownDevice.
func (a *App) powerAction(deviceID string, action string) error {
	entry, ok := a.reg.Get(deviceID)
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}
	if !entry.Online {
		return fmt.Errorf("device %s is offline", deviceID)
	}
	port := entry.Port
	if port == 0 {
		port = listenPort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	switch action {
	case "reboot":
		err = a.scanner.RebootDevice(ctx, entry.IP, port)
	case "shutdown":
		err = a.scanner.ShutdownDevice(ctx, entry.IP, port)
	}
	if errors.Is(err, scanner.ErrPowerActionTimeout) {
		a.logger.Info("power action timeout (optimistic success)",
			slog.String("device_id", deviceID),
			slog.String("action", action))
		return nil
	}
	return err
}

// StartLogStream begins streaming the device's execution log. Each
// NDJSON record is emitted as "device-log:{deviceID}" with payload
// scanner.LogLine. Idempotent for the same deviceID: a second call
// while a stream is active returns nil and does NOT spawn another
// goroutine. Errors:
//
//   - device not in registry
//   - device marked offline
func (a *App) StartLogStream(deviceID string) error {
	entry, ok := a.reg.Get(deviceID)
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}
	if !entry.Online {
		return fmt.Errorf("device %s is offline", deviceID)
	}
	port := entry.Port
	if port == 0 {
		port = listenPort
	}

	a.logStreamsMu.Lock()
	if _, exists := a.logStreams[deviceID]; exists {
		a.logStreamsMu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.logStreams[deviceID] = cancel
	a.logStreamsMu.Unlock()

	go a.runLogStream(ctx, deviceID, entry.IP, port)
	return nil
}

// StopLogStream cancels the active log stream for deviceID. Returns
// nil even if no stream is active.
func (a *App) StopLogStream(deviceID string) error {
	a.logStreamsMu.Lock()
	defer a.logStreamsMu.Unlock()
	if cancel, ok := a.logStreams[deviceID]; ok {
		cancel()
		delete(a.logStreams, deviceID)
	}
	return nil
}

// runLogStream reads from streamFn and emits each line via the
// Emitter. On exit (ctx cancel or stream error) it removes the
// stream from the map and emits "device-log-end:{id}".
func (a *App) runLogStream(ctx context.Context, deviceID, ip string, port int) {
	defer func() {
		a.logStreamsMu.Lock()
		if c, ok := a.logStreams[deviceID]; ok {
			c()
			delete(a.logStreams, deviceID)
		}
		a.logStreamsMu.Unlock()
		a.emitter.Emit(a.ctx, "device-log-end:"+deviceID, true)
	}()
	err := a.streamFn(ctx, ip, port, func(line scanner.LogLine) {
		a.emitter.Emit(a.ctx, "device-log:"+deviceID, line)
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		a.logger.Warn("log stream ended",
			slog.String("device_id", deviceID),
			slog.String("err", err.Error()))
		a.emitter.Emit(a.ctx, "device-log-error:"+deviceID, err.Error())
	}
}
