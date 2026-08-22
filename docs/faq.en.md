# Frequently Asked Questions

Quick answers to the questions we hear most. If yours isn't here, file
a question issue (`.github/ISSUE_TEMPLATE/question.yml`) or check
[`troubleshooting.md`](troubleshooting.md).

## General

### What is Spotter?

A two-binary LAN device discovery tool. The `spotterd` agent (Go,
systemd-only Linux, arm64 + amd64) announces itself via UDP multicast
and serves basic host info over HTTP. The `spotter-client` GUI (Wails,
Windows + macOS + Linux) auto-discovers those agents and presents a
paneled UI.

### Why is it split into two binaries, not one?

`spotterd` must run on headless Linux with no graphical stack, so it
cannot embed the webview that `spotter-client` needs. Keeping them
separate also keeps each release matrix small — the agent ships for two
architectures; the client for three OS families. See
[`architecture.md`](architecture.md).

### Does Spotter work over the internet?

**No.** UDP multicast stays on L2 unless your routers forward it
(IGMP/PIM). `/api/v1/info` is unauthenticated and meant for trusted LANs
only. If you put it on the public internet, anyone can reach the agent
and read host info.

### Does Spotter support authentication?

Not in 0.1.0. The agent's HTTP endpoints and UDP packets are open by
design (trusted-LAN deployment). Tracking for a future milestone; see
issue tracker / `docs/superpowers/specs/`.

### Why MIT license?

Short, permissive, compatible with most other code. If you want a
different license, see [`LICENSE`](../../LICENSE) and reach out before
embedding Spotter in a commercial product.

## Agent (`spotterd`)

### Why is `spotterd` Linux-only?

It registers as a systemd unit. systemd is the only init Spotter
explicitly supports in 0.1.0. Other init systems are a clear area for
community PRs (OpenRC / runit scripts).

### Can I run `spotterd` without systemd?

Yes, the binary is self-contained — `go run ./cmd/agent` is the
development path. For production, the install script assumes systemd
because that's how the unit file is deployed.

### How is the `device_id` generated?

`scripts/install.sh` calls `cat /proc/sys/kernel/random/uuid` once and
writes it into `/etc/spotterd/agent.toml`. The ID is stable across
reboots — to regenerate, stop the service, delete the config dir, and
re-run `install.sh`.

### Why a single static IP per row in the client?

Each entry remembers one IP. If a device moves networks, `Add Device by
IP` (or a manual subnet scan + `Accept`) re-anchors it. Tracking the
moving IP via mDNS is on the roadmap.

### What's the default port?

`9999` for both HTTP and UDP. Override via `agent.toml`'s `listen_addr`
field and the matching `multicast_group`.

### Why `CGO_ENABLED=0` for the agent build?

A static binary means a single artefact runs on any glibc/musl variant.
Cross-compile from macOS to Linux arm64 without libc version skew. The
Wails-enabled client cannot avoid CGO on Windows (it needs to embed
WebView2) but the agent does not need it.

## Client (`spotter-client`)

### Where is my data stored?

The device registry and log files live under `<UserConfig>/Spotter/`.
See [`operations.md`](operations.md) for per-OS paths.

### Can multiple operators share a registry?

Not in 0.1.0. The registry is a local JSON file under your home
directory; sharing requires forwarding the file via Syncthing, NFS, or
similar. Expect this to land in a later release as an opt-in central
backend.

### Why do new devices appear "offline" briefly?

The scanner discovers via multicast first, then does a probe so it can
populate the detail cards. There's a small window (~1 s) where the row
exists but `LastInfo` is empty. This is by design; the merge pipeline
fills the row in asynchronously.

### Can I use the client headlessly?

No, the Wails app is GUI-only. If you want a CLI discovery tool, the
recommended path is to:

1. Run `spotterd` on a target device.
2. Poll `http://<device>:9999/api/v1/info` with `curl` from any
   operator box.

The wire format is documented in [`api.md`](api.md).

### Will there be an Android / iOS client?

Not planned in 0.x. Both platforms require very different Wails flavors
and aren't a good fit for "single-binary desktop" packaging.

## Networking & firewalls

### Which ports does the agent open?

- `9999/udp` — multicast listener (the agent is in the group, not a
  server).
- `9999/tcp` — HTTP server (`/healthz`, `/api/v1/info`).

### Which ports does the client open?

Just outbound: `9999/udp` (mcast join) and `9999/tcp` (poll / probe).
No inbound listening.

### UDP multicast address — can I change it?

Yes, via `multicast_group` in `agent.toml`. The same change on the
client side requires rebuild — there's no UI for it in 0.1.0. Pick a
site-local group (239.x.x.x) so it doesn't leak out of your network.

## Build / release

### Where do binaries land?

| Build command                       | Output                              |
| ----------------------------------- | ----------------------------------- |
| `make agent-linux-arm64`            | `bin/spotterd-linux-arm64`          |
| `make agent-linux-x64`              | `bin/spotterd-linux-x64`            |
| `make client` (macOS)               | `build/bin/Spotter.app`             |
| `make client` (Linux)               | `build/bin/spotter-client`          |
| `make client` (Windows)             | `build\bin\spotter-client.exe`      |
| `make release` (= `scripts/build-all.sh`) | `dist/`                       |

### Does the release build sign binaries?

Yes — the GitHub Actions release workflow writes a `SHA256SUMS` file
into `dist/`. A future PR may add `cosign` signing.

### Can I build cross-platform from any host?

- **Agent**: yes; Go cross-compiles from any host (Linux / macOS /
  Windows) to Linux arm64 + amd64.
- **Client**: no — Wails builds for the **active GOOS**. Run
  `scripts/build-all.sh` on a macOS host to get a `Spotter.app`, on
  Linux to get the Linux binary, on Windows to get the `.exe`. The
  README has per-OS instructions.

### Where does the version come from?

Top of the chain: `git tag vX.Y.Z`. The release workflow uses that tag
as both the GitHub release name and the value set into
`SPOTTER_AGENT_VERSION` at install time. During local dev builds,
`agent_version` defaults to `0.1.0` from `wails.json`.
