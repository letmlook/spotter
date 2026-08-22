# Spotter Architecture

This document complements the design notes in
[`docs/superpowers/specs/2026-08-21-spotter-design.md`](../superpowers/specs/2026-08-21-spotter-design.md)
with an operator-facing mental model of how the binaries fit together.
Read the design spec for the rationale of the chosen structure.

## Components at a glance

```
┌──────────────────────────────────────────────┐   ┌─────────────────────┐
│ spotter-client (Wails + React GUI)            │   │ spotterd (agent)    │
│ ──────────────────────────────────────────── │   │ ─────────────────── │
│ registry (local JSON)  ◀── poll every 30 s ───┼───┤  GET /api/v1/info   │
│                         ◀── UDP mcast  ◀──────┼───┤  UDP HELLO @60 s    │
│ manual subnet probe    ── TCP probe + HTTP  ──┼───┤  GET /healthz       │
└──────────────────────────────────────────────┘   └─────────────────────┘
        ▲                                              ▲ runs as
        │                                              │ systemd unit
  macOS / Windows / Linux host                         │
                                                       │
                                                       ▼
                                              Linux systemd host
                                              (arm64 or amd64)
```

There are exactly two binaries. Everything else (`internal/`, `cmd/agent/`)
is library code shared between them.

## Package map

| Path                            | Lives in agent? | Lives in client? | Purpose                                                                          |
| ------------------------------- | :-------------: | :--------------: | -------------------------------------------------------------------------------- |
| `cmd/agent/`                    | ✅              | —                | Single-purpose `spotterd` entry point (Linux only).                              |
| `internal/agentd/`              | ✅              | —                | HTTP server, UDP multicast loop, lifecycle.                                       |
| `internal/collector/`           | ✅              | —                | OS-specific collectors (basic, jetson, network). Linux build tags.              |
| `internal/protocol/`            | ✅              | ✅               | Wire format (`DeviceInfo`) and UDP packet shape. Shared, no side effects.        |
| `internal/registry/`            | —               | ✅               | Local JSON-backed device registry persisted under the OS user config dir.       |
| `internal/scanner/`             | —               | ✅               | Three-source merge: UDP mcast, registry poll, manual subnet scan.                |
| `main.go` + `frontend/`         | —               | ✅               | Wails entry point + React/TypeScript UI.                                         |
| `scripts/`                      | both            | both             | Install / uninstall / deploy / cross-build helpers.                              |
| `docs/superpowers/specs/`       | —               | —                | Design specs (pre-implementation) — not part of the build.                       |
| `docs/superpowers/plans/`       | —               | —                | Implementation plans (pre-implementation) — not part of the build.              |

## Discovery flow (high level)

1. **UDP multicast** — every agent announces itself on `239.255.42.42:9999`
   with a `HELLO` packet every 60 s. Clients on the same L2 broadcast
   domain pick these up the first time they see a new `device_id`.
2. **Registry poll** — every device the client already knows about is
   polled every 30 s via `GET http://<ip>:9999/api/v1/info`. The
   `LastInfo` payload drives the UI cards.
3. **Manual subnet scan** — initiated from the GUI's scan button. The
   scanner detects the active IPv4 subnet (RFC1918 first), walks every
   IP, probes TCP `9999`, and on a hit calls `/healthz` and
   `/api/v1/info`.

The three sources all write into the same `internal/registry.Registry`,
which applies last-writer-wins on `LastSeenAt` and emits Wails events to
the frontend.

## Configuration sources

| What                | Where                                               | Owner             |
| ------------------- | --------------------------------------------------- | ----------------- |
| Agent listen addr   | `/etc/spotterd/agent.toml`                          | device's package  |
| Multicast group     | `/etc/spotterd/agent.toml`                          | device's package  |
| Device ID           | `/etc/spotterd/agent.toml` (UUID v4, one-shot)      | install.sh        |
| Client registry     | `<UserConfig>/Spotter/devices.json`                 | client            |
| Client logs         | `<UserConfig>/Spotter/logs/spotter.log`             | client            |
| Wails options       | `wails.json`                                        | client build      |

## Why two binaries, not one

- The agent must run on **headless Linux without a graphical stack**, so
  it cannot depend on Wails / WebView. Keeping `spotterd` as a single Go
  binary means the agent's release matrix is tiny (two architectures) and
  it is straightforward to deploy with `scp + bash install.sh`.
- The client is intentionally **opinionated** about the desktop UX — Wails
  gives us native macOS / Windows / Linux chromium-webview binaries from
  the same source. A combined binary would either need a CLI mode flag, or
  ship a webview for a use case (the agent) that does not benefit from it.

If you only need command-line discovery, `spotterd -h` is intentionally
minimal — the API is the discovery surface; everything else is in the GUI.

## Why not containerise

Spotter is shipped as static binaries. The agent has zero runtime
dependencies (no systemd socket activation, no FUSE, no libssl); the
client only needs the standard webview installed by the OS. A container
image would force operators to maintain a base image for what is, at the
end of the day, two Go binaries.

The release workflow builds artefacts for both architectures and packages
them into `dist/`; bring your own packaging format (deb, rpm, nix,
homebrew tap) if you want to ship that way.
