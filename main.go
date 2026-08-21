// spotter-client is the Windows GUI application that discovers
// spotterd instances and displays their info.
package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/spotter/spotter/internal/deployer"
	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

//go:embed all:ui
var uiFS embed.FS

// deployUsername is the SSH user assumed when deploying via the GUI.
// MVP: hardcoded "root" — v1.x may prompt per-device.
const deployUsername = "root"

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
		wailsruntime.EventsEmit(app.ctx, e.Tag())
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
func (a *App) ScanSubnet(ctx context.Context, cidr string) error {
	return a.scanner.ScanSubnet(ctx, cidr, 30*time.Second)
}

// RefreshNow forces an immediate poll cycle.
func (a *App) RefreshNow(ctx context.Context) error {
	return a.scanner.PollOnce(ctx)
}

// DeployDevice SSH-deploys spotterd to the given IP and registers the
// new device locally so the GUI begins polling it. Returns the
// generated DeviceID.
func (a *App) DeployDevice(ctx context.Context, ip string, sshPort int, password string) (string, error) {
	if sshPort == 0 {
		sshPort = 22
	}
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

// UninstallDevice SSH-runs uninstall.sh on the registered device and
// removes its registry entry. Password is re-supplied by the user per
// spec §5.3 (SSH credentials are never persisted).
func (a *App) UninstallDevice(ctx context.Context, deviceID string, password string) error {
	entry, ok := a.reg.Get(deviceID)
	if !ok {
		return fmt.Errorf("device %q not found in registry", deviceID)
	}
	dep := &deployer.Deployer{}
	if err := dep.Uninstall(ctx, deployer.DeployRequest{
		IP:       entry.IP,
		SSHPort:  22,
		Username: entry.Username,
		Password: password,
	}); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	if err := a.reg.Remove(deviceID); err != nil {
		return fmt.Errorf("uninstall succeeded on device but registry removal failed: %w", err)
	}
	return nil
}
