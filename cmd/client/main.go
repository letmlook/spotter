// spotter-client is the Windows GUI application that discovers
// spotterd instances and displays their info.
package main

import (
	"context"
	"embed"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/spotter/spotter/internal/registry"
	"github.com/spotter/spotter/internal/scanner"
)

//go:embed all:ui
var uiFS embed.FS

func main() {
	appData, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	dataDir := filepath.Join(appData, "Spotter")
	_ = os.MkdirAll(dataDir, 0755)
	logPath := filepath.Join(dataDir, "logs")
	_ = os.MkdirAll(logPath, 0755)

	logFile, _ := os.OpenFile(filepath.Join(logPath, "spotter.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))

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
}

// NewApp constructs the App, scanner, and event wiring. Scanner
// events are forwarded as Wails runtime events so the frontend can
// listen with EventsOn.
func NewApp(reg *registry.Registry, logger *slog.Logger) *App {
	sc := scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
		logger.Info("scanner event", slog.String("tag", e.Tag()))
		wailsruntime.EventsEmit(nil, e.Tag())
	}))
	return &App{reg: reg, logger: logger, scanner: sc}
}

// StartScanner begins the poll + mcast loops. The frontend calls this
// once at startup. The ctx is canceled when the Wails app exits.
func (a *App) StartScanner(ctx context.Context) {
	a.scanner.Start(ctx)
}

// ListDevices returns the registry snapshot for the UI.
func (a *App) ListDevices() []registry.Entry {
	return a.reg.List()
}

// ScanSubnet triggers a manual subnet scan.
func (a *App) ScanSubnet(ctx context.Context, cidr string) error {
	return a.scanner.ScanSubnet(ctx, cidr, 30*1_000_000_000) // 30s
}

// RefreshNow forces an immediate poll cycle.
func (a *App) RefreshNow(ctx context.Context) error {
	return a.scanner.PollOnce(ctx)
}