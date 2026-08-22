# Spotter

LAN device discovery for Linux ARM64 targets (Jetson, Ubuntu Server, Debian).

- **Device side**: a single-file Go binary `spotterd` (systemd unit) at `cmd/agent/`.
- **Client side**: a Windows GUI (`spotter-client`) built with Wails; entrypoint
  lives at the project root and embeds the UI from `frontend/` (Vite + React + TS).

The client discovers devices via three sources, all of which feed a single
merge pipeline:

1. **Registry poll** (HTTP GET `/api/v1/info` every 30s)
2. **UDP multicast** (`239.255.42.42:9999`, every 60s)
3. **Manual subnet scan** (TCP probe + `/healthz` + `/api/v1/info`)

## Build

The `Makefile` is the project's source of truth for build targets.

```bash
# Unit tests (race detector, full module)
make test

# Device-side binary for Linux ARM64
make agent-linux-arm64

# Windows client (builds main.go at the project root)
make client

# The Wails build pulls frontend deps and bundles them. To work on the UI
# in isolation, install deps and build the Vite project:
cd frontend && npm install
npm run build

# Or, equivalently, use the Wails CLI directly:
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails build
```

### Build on macOS

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
make client          # produces build/bin/Spotter.app
open build/bin/Spotter.app
```

If `wails` is not on PATH, `make client` falls back to `go build` and
produces a bare `bin/spotter-client` Mach-O binary (no .app bundle).
The `.app` bundle is the recommended way to launch on macOS.

Artifacts are written to `bin/`:

| Artifact                       | Source            |
|--------------------------------|-------------------|
| `bin/spotterd-linux-arm64`     | `cmd/agent/`      |
| `bin/spotterd`                 | `cmd/agent/`      |
| `bin/spotter-client` (Win GUI) | root `main.go`    |

## Deploy to a device

The Windows GUI collects `IP / SSH port / username / password`, runs:

```bash
sftp put bin/spotterd-linux-arm64 /tmp/spotterd
sftp put scripts/install.sh /tmp/install.sh
sftp put scripts/spotterd.service /tmp/spotterd.service
ssh bash /tmp/install.sh   # exports SPOTTER_AGENT_VERSION
```

The install script:

1. Installs `spotterd` to `/usr/local/bin/`.
2. Generates a `device_id` (UUID v4) and writes `/etc/spotterd/agent.toml`.
3. Installs the systemd unit and enables it.

Uninstall uses `scripts/uninstall.sh`; full teardown uses `scripts/cleanup.sh`.

## Known limitations (MVP)

- Linux ARM64 with **systemd** only (Ubuntu / Jetson / Debian / RHEL).
- **Windows client only** (no macOS / Linux client yet).
- **No remote command execution** — static info panel only.
- UDP multicast is **L2-only** (same VLAN) unless routers forward.
- HTTP endpoints have **no authentication** — deploy on trusted LANs only.
- SSH credentials are **never persisted** (re-enter per deploy / uninstall).

## Architecture

See [`docs/superpowers/specs/2026-08-21-spotter-design.md`](docs/superpowers/specs/2026-08-21-spotter-design.md).
