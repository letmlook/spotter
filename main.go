// spotter-client is the Windows GUI application that discovers
// spotterd instances and displays their info.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/spotter/spotter/internal/deployer"
	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

//go:embed all:frontend/dist
var uiFS embed.FS

// deployUsername is the SSH user assumed when deploying via the GUI.
// MVP: hardcoded — v1.x may prompt per-device.
const deployUsername = "fitow"

// listenPort is the device-side HTTP port the client expects after a
// successful install.sh (the install script writes agent.toml with
// listen_addr = "0.0.0.0:9999").
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

	app := NewApp(reg, logger)

	err = wails.Run(&options.App{
		Title:  "Spotter",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: uiFS,
		},
		OnStartup: app.OnStartup,
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
	reg     *registry.Registry
	logger  *slog.Logger
	scanner *scanner.Scanner
	ctx     context.Context // injected via OnStartup; pre-initialised to a non-nil placeholder so EventsEmit is safe before Wails starts.
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
func NewApp(reg *registry.Registry, logger *slog.Logger) *App {
	app := &App{reg: reg, logger: logger, ctx: context.Background()}
	app.scanner = scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		logger.Info("scanner event", slog.String("tag", e.Tag()))
		wailsruntime.EventsEmit(app.ctx, e.Tag(), e)
	}))
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

// ScanSubnet triggers a manual subnet scan.
func (a *App) ScanSubnet(cidr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return a.scanner.ScanSubnet(ctx, cidr, 30*time.Second)
}

// RefreshNow forces an immediate poll cycle.
func (a *App) RefreshNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return a.scanner.PollOnce(ctx)
}

// DeployDevice SSH-deploys spotterd to the given IP and registers the
// new device locally so the GUI begins polling it. Returns the
// generated DeviceID.
//
// Wails does not inject ctx into bound methods, so we accept it as a
// regular string param (empty = no timeout override) and internally
// derive a context.Background() with a 60s deploy timeout.
func (a *App) DeployDevice(ip string, sshPort int, password string) (string, error) {
	if sshPort == 0 {
		sshPort = 22
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dep := &deployer.Deployer{}
	res, err := dep.Deploy(ctx, deployer.DeployRequest{
		IP:       ip,
		SSHPort:  sshPort,
		Username: deployUsername,
		Password: password,
	})
	if err != nil {
		return "", fmt.Errorf("deploy: %w", err)
	}
	if err := a.reg.Add(registry.Entry{
		DeviceID:   res.DeviceID,
		IP:         ip,
		Port:       listenPort,
		Username:   deployUsername,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
		Online:     false,
	}); err != nil {
		a.logger.Error("register deployed device", slog.String("err", err.Error()))
		return res.DeviceID, fmt.Errorf("deploy succeeded but failed to register: %w", err)
	}
	return res.DeviceID, nil
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

// ClearRegistry removes every entry from the local registry. Used to
// reset the GUI without touching the remote devices (use Uninstall for
// that). Returns the number of entries removed.
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

// UninstallDevice SSH-runs uninstall.sh on the registered device and
// removes its registry entry. Password and username are re-supplied by
// the user per spec §5.3 (SSH credentials are never persisted). If
// username is empty, falls back to whatever the registry stored.
func (a *App) UninstallDevice(deviceID string, username string, password string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	entry, ok := a.reg.Get(deviceID)
	if !ok {
		return fmt.Errorf("device %q not found in registry", deviceID)
	}
	if username == "" {
		username = entry.Username
	}
	if username == "" {
		return fmt.Errorf("device %q has no SSH username recorded; supply one", deviceID)
	}
	// Persist the supplied username for future uninstalls.
	if entry.Username != username {
		_ = a.reg.Update(deviceID, func(e *registry.Entry) { e.Username = username })
	}
	dep := &deployer.Deployer{}
	if err := dep.Uninstall(ctx, deployer.DeployRequest{
		IP:       entry.IP,
		SSHPort:  22,
		Username: username,
		Password: password,
	}); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	if err := a.reg.Remove(deviceID); err != nil {
		return fmt.Errorf("uninstall succeeded on device but registry removal failed: %w", err)
	}
	return nil
}
