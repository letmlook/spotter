# Spotter

[中文版本](README.md) · [English](README.en.md)

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

## Release packaging

`scripts/build-all.sh` orchestrates a one-shot release build on a
single host:

```bash
./scripts/build-all.sh              # full build (agents + host client)
./scripts/build-all.sh --agent-only # agents only, skip client
make release                        # equivalent entry point
```

What it does:

- **Devices**: always cross-compiles Linux `arm64` and `amd64`
  binaries.
- **Client**: builds only for the current host — `Spotter.app` on
  macOS, `.exe` on Windows, plain binary on Linux.
- **Packaging**: stages everything under `dist/` and writes a
  `SHA256SUMS` file.

To ship clients for every platform, run the script once on each
target host and merge the resulting `dist/clients/` directories.

## Manually installing spotterd on a device

The GUI client only discovers and displays devices — `spotterd` itself
must be installed on each target manually. The release bundle ships
`scripts/install.sh` for the device side and `scripts/deploy.sh` /
`scripts/deploy.ps1` for one-shot push-and-install from the developer
machine.

### Option A: one-shot deploy script (recommended)

The script supports two auth modes — **SSH public key** (preferred,
no password when keys are already configured) or **password** (needs
`sshpass` on mac/Linux or PuTTY's `plink` on Windows).

**macOS / Linux** (developer machine):

```bash
# One-time install of sshpass (password mode only; key mode doesn't need it)
brew install hudochenkov/sshpass/sshpass   # Debian/Ubuntu: apt install sshpass; Fedora: dnf install sshpass

make agent-linux-arm64                    # or agent-linux-x64

# Key mode (preferred): the public key is already in ssh-agent / ~/.ssh
./scripts/deploy.sh nvidia 10.0.5.23
./scripts/deploy.sh nvidia 10.0.5.23 amd64

# Password mode: when no key is configured
./scripts/deploy.sh nvidia <password> 10.0.5.23
./scripts/deploy.sh nvidia <password> 10.0.5.23 amd64
```

**Windows** (PowerShell):

```powershell
# One-time install of PuTTY (password mode needs plink/pscp; key mode
# can use Pageant only). https://putty.org  or  choco install putty

make agent-linux-arm64

# Key mode (preferred): Pageant / ssh-agent has the key loaded
.\scripts\deploy.ps1 -User nvidia -Ip 10.0.5.23
.\scripts\deploy.ps1 -User nvidia -Ip 10.0.5.23 -Arch amd64

# Password mode: when no key is configured
.\scripts\deploy.ps1 -User nvidia -Password <password> -Ip 10.0.5.23
.\scripts\deploy.ps1 -User nvidia -Password <password> -Ip 10.0.5.23 -Arch amd64
```

The script scp's `spotterd` + `spotterd.service` + `install.sh`,
runs `sudo bash /tmp/install.sh` over ssh, then `curl /healthz` to
verify.

> The SSH user must be passwordless-sudo on the target, otherwise the
> `sudo bash` inside install.sh will hang on an interactive password
> prompt. In that case fall back to option B and run each step by hand.

### Option B: manual scp + ssh

```bash
# Push the matching arch binary, systemd unit, and install script
scp bin/spotterd-linux-<arch>  user@<device>:/tmp/spotterd
scp scripts/spotterd.service   user@<device>:/tmp/spotterd.service
scp scripts/install.sh         user@<device>:/tmp/install.sh

# Run the installer on the target
ssh user@<device> sudo bash /tmp/install.sh
```

`install.sh` will:

1. Install `spotterd` to `/usr/local/bin/`.
2. Generate a `device_id` (UUID v4) and write `/etc/spotterd/agent.toml`.
3. Install and enable the systemd unit.

Uninstall uses `scripts/uninstall.sh`; full teardown uses
`scripts/cleanup.sh`.

Once the device is up, the GUI discovers it automatically via UDP
multicast (or via the manual subnet scan / "Add by IP" actions) — no
GUI-side registration is required.

## Known limitations (MVP)

- Linux devices with **systemd** only (Ubuntu / Jetson / Debian / RHEL;
  both `arm64` and `amd64`).
- **No remote command execution** — static info panel only.
- UDP multicast is **L2-only** (same VLAN) unless routers forward.
- HTTP endpoints have **no authentication** — deploy on trusted LANs only.

## Architecture

The full design rationale lives in
[`docs/superpowers/specs/2026-08-21-spotter-design.md`](docs/superpowers/specs/2026-08-21-spotter-design.md);
a condensed map of components and packages is in
[`docs/architecture.md`](docs/architecture.md).

## Documentation index

Chinese is the default documentation; English variants use the `.en.md`
suffix. From any English page there is a link to its Chinese counterpart
at the top.

| Document                                          | Audience    | What it covers                                                |
|--------------------------------------------------|-------------|---------------------------------------------------------------|
| [README.md](README.md) / [README.en.md](README.en.md) | Everyone | Project entry, build matrix, deploy scripts.                  |
| [docs/architecture.md](docs/architecture.md) / [.en](docs/architecture.en.md) | Developers  | Component / package map, why two binaries, config sources.    |
| [docs/operations.md](docs/operations.md) / [.en](docs/operations.en.md) | Operators   | File layout on devices and clients, configs, daily tasks, upgrades. |
| [docs/troubleshooting.md](docs/troubleshooting.md) / [.en](docs/troubleshooting.en.md) | On-call     | 10 symptom-driven failure modes and how to recover.           |
| [docs/faq.md](docs/faq.md) / [.en](docs/faq.en.md) | Everyone    | Licensing, networking requirements, build output, etc.        |
| [docs/api.md](docs/api.md) / [.en](docs/api.en.md) | Integrators | Field-level spec of `/api/v1/info`, `/healthz`, and the UDP packet. |
| [CONTRIBUTING.md](CONTRIBUTING.md) / [.en](CONTRIBUTING.en.md) | Contributors | Dev setup, commit conventions, how to add a collector / component. |
| [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) / [.en](CODE_OF_CONDUCT.en.md) | Contributors | Community code of conduct (Contributor Covenant v2.1).        |
| [SECURITY.md](SECURITY.md) / [.en](SECURITY.en.md) | Reporters   | Vulnerability channel, supported versions, hardening checklist. |
| [CHANGELOG.md](CHANGELOG.md) / [.en](CHANGELOG.en.md) | Everyone    | Per-release change log (Keep a Changelog format).             |
| [LICENSE](LICENSE)                                | Everyone    | MIT License (Copyright © 2026 Spotter Dev).                   |

## Project metadata

- `.github/ISSUE_TEMPLATE/` — bug, feature, device/platform support,
  question, and security templates.
- `.github/PULL_REQUEST_TEMPLATE.md` — self-checklist for PR authors.
- `.github/workflows/` — `go.yml` (Go tests + lint), `frontend.yml`
  (frontend build + typecheck), `agent-build.yml` (arm64 / amd64 cross
  compile), `release.yml` (tag-driven GitHub Release).
- `.github/dependabot.yml` — weekly dependency updates for Go, npm,
  GitHub Actions.
- `.golangci.yml` — golangci-lint v1.59+ rule set.