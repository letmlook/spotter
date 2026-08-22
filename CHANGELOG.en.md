# Changelog

All notable changes to Spotter are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- _nothing yet_

### Changed
- _nothing yet_

### Removed
- _nothing yet_

### Fixed
- _nothing yet_

### Security
- _nothing yet_

## [0.1.0] — 2026-08-22

The inaugural public release of Spotter. Both binaries ship from this
version. The agent targets Linux on `arm64` and `amd64`; the client runs on
Windows, macOS, and Linux.

### Added

#### Agent (`spotterd`)
- systemd service unit (`scripts/spotterd.service`) and a one-shot
  installer (`scripts/install.sh`) that generates a `device_id` UUID and
  writes `/etc/spotterd/agent.toml`.
- UDP multicast beacon (`239.255.42.42:9999`, 60-second cadence) so the
  desktop client can discover devices on the same L2 broadcast domain.
- HTTP endpoints `GET /healthz` (liveness) and `GET /api/v1/info`
  (device info snapshot in JSON).
- Collectors for basic Linux host info, Jetson-specific telemetry (tegra
  SOC, jetson_clocks, nvpmodel), and network interface descriptors.
- `scripts/deploy.sh` (macOS/Linux) and `scripts/deploy.ps1` (Windows) to
  SCP + SSH push the matching binary to a target device. Both support SSH
  public-key and `sshpass` / PuTTY password modes.
- `scripts/uninstall.sh` and `scripts/cleanup.sh` for graceful and
  best-effort removal.

#### Client (`spotter-client`)
- Wails desktop GUI for Windows, macOS, and Linux.
- Three-way discovery pipeline — registry poll (`/api/v1/info` every 30 s),
  UDP multicast join, and manual subnet scan (TCP probe + `/healthz` +
  `/api/v1/info`).
- Per-device cards (basic, network, Jetson), detail panel, and a sidebar
  list with online/offline status and "last source" attribution.
- i18n dictionaries (en + zh-CN) — all UI strings routed through
  `frontend/src/i18n/dictionaries.ts`.
- Setup guide modal that walks new operators through `make agent` and
  one of the deploy scripts.

#### Build / CI
- Makefile as the single authoritative build entry point.
- `scripts/build-all.sh` cross-compiles both agent architectures, builds
  the active-platform client, and packages everything into `dist/` with a
  `SHA256SUMS` file.
- Race-detector unit tests across `internal/agentd`, `internal/collector`,
  `internal/protocol`, `internal/registry`, and `internal/scanner`.

### Notes
- The wire protocol carries `schema_version: 1`. Future breaking changes
  will bump this and ship a versioned endpoint (`/api/v2/info`).
- Known limitations tracked in the top-level README ("Known limitations
  (MVP)"): no remote command execution, UDP multicast stays on L2, no
  authentication.

[Unreleased]: https://github.com/spotter/spotter/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/spotter/spotter/releases/tag/v0.1.0
