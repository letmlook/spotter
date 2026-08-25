// spotter-server is a small HTTP + WebSocket hub that aggregates
// spotterd device registrations and heartbeats so multiple
// spotter-client users can share a fleet. See
// docs/superpowers/specs/2026-08-22-registry-server-design.md for
// the protocol and roadmap.
//
// Build tag was previously `//go:build linux`, but the underlying
// internal/serverd package uses only net/http, gorilla/websocket,
// and the standard library — nothing platform-specific — so the
// server now compiles on macOS and Windows too.

package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spotter/spotter/internal/serverd"
)

func main() {
	var (
		listen = flag.String("listen", ":9998", "HTTP listen address")
		data   = flag.String("data", "", "data directory (defaults to ~/.config/spotter-server)")
	)
	flag.Parse()

	if *data == "" {
		home, err := os.UserConfigDir()
		if err != nil {
			slog.Error("user config dir", slog.String("err", err.Error()))
			os.Exit(1)
		}
		*data = filepath.Join(home, "spotter-server")
	}
	if err := os.MkdirAll(*data, 0755); err != nil {
		slog.Error("create data dir", slog.String("err", err.Error()))
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	store, err := 	serverd.Open(filepath.Join(*data, "server.json"))
	if err != nil {
		log.Error("open store", slog.String("err", err.Error()))
		os.Exit(1)
	}
	hub := 	serverd.NewHub()
	srv := &http.Server{
		Addr:              *listen,
		Handler:           	serverd.NewHandler(store, hub),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		log.Info("spotter-server listening", slog.String("addr", *listen))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	case err := <-errCh:
		if err != nil {
			log.Error("listen", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}
}
