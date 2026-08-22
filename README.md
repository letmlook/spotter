# Spotter

LAN device discovery for Linux devices (ARM64 SBCs such as Jetson,
plus AMD64 servers and workstations).

- **Device side**: a single-file Go binary `spotterd` (systemd unit) at
  `cmd/agent/`. Built for Linux on **both `arm64` and `amd64`**.
- **Client side**: `spotter-client`, a Wails desktop GUI with **native
  builds for Windows, macOS and Linux**. The entrypoint lives at the
  project root and embeds the UI from `frontend/` (Vite + React + TS).

The client discovers devices via three sources, all of which feed a
single merge pipeline:

1. **Registry poll** (HTTP GET `/api/v1/info` every 30s)
2. **UDP multicast** (`239.255.42.42:9999`, every 60s)
3. **Manual subnet scan** (TCP probe + `/healthz` + `/api/v1/info`)

## Platform support

| Component       | Linux arm64 | Linux amd64 | Windows | macOS |
|-----------------|:-----------:|:-----------:|:-------:|:-----:|
| `spotterd`      | ✓           | ✓           | —       | —     |
| `spotter-client`| ✓           | ✓           | ✓       | ✓     |

`spotterd` is Linux-only because it depends on `systemd` for service
management; `spotter-client` is a standard Wails app and follows the
Wails matrix exactly.

## Build

The `Makefile` is the project's source of truth for build targets.

```bash
# Unit tests (race detector, full module)
make test

# Device-side binaries for both supported Linux arches
make agent-all

# Single-arch device builds (for staging one arch at a time)
make agent-linux-arm64
make agent-linux-x64

# Cross-platform desktop client (Windows / macOS / Linux)
# Wails selects the active platform from GOOS.
make client
```

`make client` prefers the `wails` CLI when available and falls back to
`go build` otherwise (the fallback path does not produce a macOS
`.app` bundle — see the per-platform notes below).

### Client on macOS

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
make client          # produces build/bin/Spotter.app
open build/bin/Spotter.app
```

### Client on Windows (PowerShell)

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend; npm install; cd ..
make client
# Produces build\bin\spotter-client.exe
```

### Client on Linux

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
make client
# Produces build/bin/spotter-client
```

Wails pulls frontend deps and bundles them. To work on the UI in
isolation, install deps and build the Vite project:

```bash
cd frontend && npm install && npm run build
```

Artifacts are written to `bin/`:

| Artifact                            | Source            | Target        |
|-------------------------------------|-------------------|---------------|
| `bin/spotterd-linux-arm64`          | `cmd/agent/`      | Linux ARM64   |
| `bin/spotterd-linux-x64`            | `cmd/agent/`      | Linux AMD64   |
| `bin/spotterd`                      | `cmd/agent/`      | host GOOS/GOARCH |
| `bin/spotter-client` / `.exe`       | root `main.go`    | host GOOS     |
| `build/bin/Spotter.app`             | `wails build`     | macOS bundle  |

## Deploy to a device

The GUI collects `IP / SSH port / username / password`, picks the
right `spotterd` binary for the target architecture, and runs:

```bash
sftp put bin/spotterd-linux-<arch> /tmp/spotterd
sftp put scripts/install.sh        /tmp/install.sh
sftp put scripts/spotterd.service  /tmp/spotterd.service
ssh bash /tmp/install.sh           # exports SPOTTER_AGENT_VERSION
```

The install script:

1. Installs `spotterd` to `/usr/local/bin/`.
2. Generates a `device_id` (UUID v4) and writes `/etc/spotterd/agent.toml`.
3. Installs the systemd unit and enables it.

Uninstall uses `scripts/uninstall.sh`; full teardown uses
`scripts/cleanup.sh`.

## Known limitations (MVP)

- Linux devices with **systemd** only (Ubuntu / Jetson / Debian / RHEL;
  both `arm64` and `amd64`).
- **No remote command execution** — static info panel only.
- UDP multicast is **L2-only** (same VLAN) unless routers forward.
- HTTP endpoints have **no authentication** — deploy on trusted LANs only.
- SSH credentials are **never persisted** (re-enter per deploy / uninstall).

## Architecture

See [`docs/superpowers/specs/2026-08-21-spotter-design.md`](docs/superpowers/specs/2026-08-21-spotter-design.md).