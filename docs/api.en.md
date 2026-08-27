# Wire protocol & HTTP API reference

Spotter uses a tiny JSON-over-HTTP surface and a small UDP packet. Both
are versioned through `schema_version` so future breaking changes can
ship at a new `/api/vN/info` endpoint without upgrading every device.

> The authoritative source is the Go struct in
> [`internal/protocol/info.go`](../internal/protocol/info.go). The shapes
> below match `DeviceInfo` plus its embedded sections; if the two ever
> disagree, trust the code — it is the artifact that ships.

## Common conventions

- All times are RFC3339 in UTC.
- All sizes are bytes (uint64).
- All durations are seconds (float64).
- Unknown / future fields are tolerated by the decoder; missing required
  fields fail with a clear error.
- The default listen port is `9999` for both HTTP and UDP. Override via
  `listen_addr` and `multicast_group` in `/etc/spotterd/agent.toml`.

## `GET /healthz`

Liveness probe — no body. Returns `200 OK` if the agent's HTTP loop is
running, otherwise the connection refuses. Used by the client's manual
subnet scan to filter candidates before issuing `/api/v1/info`.

## `GET /api/v1/info`

The discovery payload. Decoded into the `protocol.DeviceInfo` struct.

### Response (`200 OK`)

```json
{
  "schema_version": 1,
  "device_id": "9d1f2c5e-7a3b-4f00-b112-9c1a5d4e3e21",
  "hostname": "nvidia-orin-1",
  "agent_version": "0.1.0",
  "collected_at": "2026-08-22T08:00:00Z",
  "basic": { ... },
  "network": { ... },
  "jetson": { ... },
  "extra": { ... }
}
```

| Field             | Type     | Notes                                            |
| ----------------- | -------- | ------------------------------------------------ |
| `schema_version`  | int      | Bumped on backwards-incompatible changes.        |
| `device_id`       | string   | UUID v4 from `/etc/spotterd/agent.toml`.         |
| `hostname`        | string   | Output of `hostname` (short).                    |
| `agent_version`   | string   | Matches `agent_version` in agent.toml.           |
| `collected_at`    | string   | RFC3339 timestamp of the payload's collection.   |
| `basic`, `network`, `jetson` | object | Optional collectors; absent on non-Linux / non-Jetson. |

`extra` is a free-form map (`map[string]any`) for collector-specific
extension data not yet promoted to a first-class field.

### `basic` section

```json
{
  "os": "linux",
  "distribution": "Ubuntu 22.04.3 LTS",
  "kernel": "5.15.0-1050-jetson",
  "arch": "aarch64",
  "uptime_sec": 36000.0,
  "loadavg": { "1": 0.12, "5": 0.08, "15": 0.05 },
  "cpu": { "model": "ARMv8 ...", "cores": 8, "threads": 8 },
  "memory": {
    "total_bytes": 16777216000,
    "available_bytes": 12421812224,
    "used_bytes": 4355403776
  },
  "disks": [
    { "mountpoint": "/", "total_bytes": 250000000000, "used_bytes": 92000000000, "fstype": "ext4" }
  ]
}
```

Fields with `null` value are intentionally absent from the wire (Go
zero-values are elided by the JSON encoder).

### `network` section

```json
{
  "interfaces": [
    {
      "name": "eth0",
      "mac": "aa:bb:cc:dd:ee:ff",
      "ipv4": ["10.0.5.23/24"],
      "ipv6": ["fe80::aabb:ccff:fedd:eeff/64"],
      "mtu": 1500,
      "state": "up"
    }
  ],
  "default_route": "10.0.5.1",
  "dns_servers": ["10.0.0.10", "1.1.1.1"]
}
```

Loopback (`lo`) and link-local IPv6 are filtered out by the collector.

### `jetson` section

Only present on Jetson hardware; absent (`null`) elsewhere.

```json
{
  "model": "NVIDIA Jetson Orin Nano",
  "tegra": { "soc": "tegra234", "chip_id": "...", "cvm_hash": "..." },
  "jetpack": "6.0",
  "l4t": "36.3.0",
  "nvpmodel": { "current": "MODE_15W", "available": ["MODE_15W", "MODE_25W"] },
  "jetson_clocks": { "running": false, "supported": true },
  "nvpower": {
    "current_w": 7.4,
    "voltage_v": 19.0,
    "input_current_a": 0.4,
    "fan_pwm": 0
  }
}
```

A nil / missing `jetson` field signals the agent is not on a Jetson
board. Don't pattern-match the field name to decide; pattern-match
its presence.

## `GET /api/v1/logs?tail=N`

Streams the device's execution log (default `journalctl -u spotterd.service`).

Request:
- Headers: `Accept: application/x-ndjson` (informational; agent does not enforce).
- Query parameters:
  - `tail=N` (default 100, max 1000) — history lines to replay before following.
  - `unit=foo,bar` (comma-separated; empty = default unit only). The configured default unit is always prepended to `?unit=`, so `?unit=nginx` alone does NOT replace the agent's own log — the agent's unit still appears in the stream.
  - `grep=REGEX` — `journalctl --grep`, case-sensitive regex; empty = no filter.
  - `since=SPEC` — `journalctl --since`, free-form (`5min ago` / `2026-08-27 12:00:00` / `yesterday`); empty = no start bound (use `tail` for history).
  - `priority=LEVEL` — `journalctl --priority`, LEVEL ∈ `emerg|alert|crit|err|warning|notice|info|debug` (numeric 0-7 also accepted); empty = no limit.

Response (200, NDJSON stream):
- One JSON object per line: journalctl `--output=json` raw record (including `__REALTIME_TIMESTAMP`, `MESSAGE`, `__CURSOR`, etc.).
- Behaviour: replays the most recent N lines, then follows new output; the stream terminates when the client disconnects.

Response (403, disabled):
- Plain text `log streaming disabled` (when `enable_log_stream` is missing or false).

Response (405, non-GET): plain text `method not allowed`.

## `enable_log_stream` / `log_unit`

`/etc/spotterd/agent.toml`:

```toml
enable_log_stream = true   # default true
log_unit = "spotterd.service"  # default
```

When enabled, the agent exposes `GET /api/v1/logs`. Unauthenticated; deployer is responsible for network isolation. Log content may contain sensitive information; evaluate before enabling.

## `POST /api/v1/reboot`

Requests the device to reboot. Only effective when `enable_power_actions = true` in the agent config.

Request:
- Headers: no body required.

Response (202, scheduled):
```json
{
  "status": "scheduled",
  "action": "reboot"
}
```

Response (403, disabled):
```json
{
  "error": "power actions disabled"
}
```

Response (405, non-POST): plain text `method not allowed`.

## `POST /api/v1/shutdown`

Same as reboot, but invokes `systemctl poweroff`. **Irreversible** — the device requires manual power-on.

## `enable_power_actions`

`/etc/spotterd/agent.toml`:

```toml
enable_power_actions = true   # default true
```

When enabled, the agent accepts `POST /api/v1/reboot` and `/api/v1/shutdown`. Unauthenticated; deployer is responsible for network isolation. To turn it off, explicitly set `enable_power_actions = false`.

## `POST /api/v1/power`

Unified power action endpoint (new in v0.5). Supports immediate execution, delayed execution, and dry-run. Audit and cancel are linked through `request_id`. **Only effective when `enable_power_actions = true`.**

Request (`Content-Type: application/json`):

```json
{
  "action": "reboot",
  "dry_run": false,
  "delay_minutes": 0,
  "request_id": "ops-2026-08-27-001"
}
```

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `action` | string | yes | `reboot` or `shutdown`. |
| `dry_run` | bool | no | When `true`, the endpoint echoes `would_execute` and does not schedule anything. |
| `delay_minutes` | int | no | 0 = immediate; 1–1440 = delay in minutes (max 24h). |
| `request_id` | string | no | Client-generated dedup key. Caller is responsible for idempotency. The `cancel` endpoint correlates by this ID. |

Response (202, dispatched or scheduled):

```json
{
  "status": "scheduled",
  "action": "reboot",
  "dry_run": false,
  "delay_minutes": 0,
  "would_execute": false,
  "request_id": "ops-2026-08-27-001"
}
```

| `status` | Trigger |
| --- | --- |
| `scheduled` | `systemctl` invoked, or `delay_minutes > 0` queued for later execution. |
| `would_execute` | `dry_run=true`; nothing actually dispatched. |
| `running` | Internal status (reserved for audit log use). |
| `cancelled` | Delayed job cancelled via the `cancel` endpoint (only after v0.6 ships). |

When `delay_minutes > 0`, the response also includes `execute_at` (RFC3339 UTC) indicating the planned execution time.

Error responses:
- 400: `action` is not `reboot`/`shutdown`; `delay_minutes` out of range (< 0 or > 1440); body parse failure.
- 403: `power actions disabled` (when `enable_power_actions = false`).
- 405: not POST.

## `GET /api/v1/power/audit`

Returns the in-process power action audit log. `Content-Type: application/x-ndjson`; one JSON object per line (the internal TSV audit row, exported as NDJSON), appended in time order.

- 200: NDJSON stream (`io.Copy` 32 KB chunks; empty body when the file is empty).
- 405: not GET.
- 503: `audit log unavailable` / `audit not open` (audit log not initialised or already closed; in practice it is ready from startup).

## `GET /api/v1/power/audit/recent`

Returns the most recent N audit entries as a JSON array (vs `/audit`'s NDJSON stream over the whole file). The GUI uses this to render a "recent activity" list in the detail panel.

Request:
- Query: `limit=N` (default 50, max 200; values >200 are silently capped).

Response (200):

```json
{
  "count": 2,
  "entries": [
    {
      "at": "2026-08-27T22:00:00Z",
      "action": "reboot",
      "dry_run": false,
      "request_id": "ops-2026-08-27-001",
      "remote_addr": "10.0.0.42:54321",
      "result": "scheduled"
    },
    {
      "at": "2026-08-27T22:00:05Z",
      "action": "shutdown",
      "dry_run": false,
      "request_id": "ops-2026-08-27-002",
      "remote_addr": "10.0.0.42:54321",
      "result": "scheduled"
    }
  ]
}
```

Error responses:
- 405: not GET.
- 503: `audit log unavailable` (audit logger not initialised).

> Internal format: the audit file is still TSV (`/var/log/spotterd/audit.tsv`) with field order `timestamp \t action \t dry_run=... \t req=... \t ip=... \t status=...`. The decoder skips malformed rows (so one corrupt line doesn't blank the page) but never modifies the file.

## `POST /api/v1/power/cancel`

Cancels a **scheduled but not yet executed** delayed power action by `request_id`. Cancellation goes through an in-process map (`Agent.pending`, `request_id` → `chan struct{}`); closing the channel makes `delayExec` exit its select without invoking `systemctl`.

Request (`Content-Type: application/json`):

```json
{
  "request_id": "ops-2026-08-27-001"
}
```

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `request_id` | string | yes | Must match the `request_id` the dispatch endpoint was given. |

Response (202, cancelled):

```json
{
  "status": "cancelled",
  "request_id": "ops-2026-08-27-001"
}
```

Error responses:
- 400: `request_id` missing.
- 403: `power actions disabled` (when `enable_power_actions = false`).
- 404: `no pending action with that request_id` — the ID is unknown, already executed, or lost across an agent restart. The GUI may still optimistically show "cancelled" (a v0.6 plan persists the map to pid-files for cross-restart coverage).
- 405: not POST.

> **v0.6 plan**: upgrade the in-memory map to `/var/run/spotterd/power-pending/<request_id>.json` (carrying a `pid`); the cancel endpoint then sends SIGINT to the recorded PID, surviving an agent restart. Today's v0.5 in-memory implementation already covers every cancel scenario within a single process.

## `GET /api/v1/metrics/recent`

Returns the agent's **last 5 minutes of CPU / memory / temperature time-series samples** (60-slot ring buffer, 5s interval). The GUI uses this as the data source for sparklines and mini charts.

> Writer: the sampler goroutine in `internal/agentd/metrics.go` is wired via `SetLifecycleContext` at agent startup. It shares the lifecycle context with the UDP/heartbeat loop, so SIGTERM-driven shutdown tears it down cleanly.

Response (200):

```json
{
  "interval_seconds": 5,
  "samples": [
    { "at": "2026-08-27T22:00:00Z", "cpu_percent": 12.3, "mem_percent": 41.5, "mem_used_bytes": 6816972800, "temp_celsius": 48.0 },
    { "at": "2026-08-27T22:00:05Z", "cpu_percent": 15.1, "mem_percent": 41.6, "mem_used_bytes": 6826213376, "temp_celsius": 49.0 }
  ]
}
```

| Field | Type | Description |
| --- | --- | --- |
| `interval_seconds` | int | Sample interval in seconds. |
| `samples[].at` | string (RFC3339 UTC) | Sampling instant. |
| `samples[].cpu_percent` | float | Interval-average CPU usage percent, 0–100, 0.1 precision. The first sample is 0 (no prior tick to diff against). |
| `samples[].mem_percent` | float | Current memory percent (MemTotal - MemAvailable), 0.1 precision. |
| `samples[].mem_used_bytes` | uint64 | Used bytes. |
| `samples[].temp_celsius` | float | First `thermal_zone*` reading in ℃. Field is omitted when no sensor is present (`omitempty`). |

Error responses:
- 405: not GET.
- 503: `metrics not started` (agent startup hasn't completed; resolves within 1s in practice).

> **Resource cost**: each sample reads 3 procfs/sysfs files plus a time format, < 50µs. Ring buffer is 60 × ~80B ≈ 5KB resident.

## UDP multicast packet (group `239.255.42.42:9999`)

Each agent periodically (default 5s, controlled by `hello_interval`)
broadcasts a small JSON packet on the multicast group — both
responding to a client-originated HELLO (HELLO_REPLY) and **proactively
emitting** its own HELLO so the client can detect an online transition
without first sending a request. The packet is the minimum needed to
bootstrap the client before any HTTP round-trip.

```
{
  "tag": "hello",
  "device_id": "9d1f2c5e-…",
  "host": "nvidia-orin-1",
  "listen_port": 9999,
  "agent_version": "0.1.0"
}
```

| Field            | Type   | Notes                                                  |
| ---------------- | ------ | ------------------------------------------------------ |
| `tag`            | string | `hello`. Reserved for future `ping`, `goodbye`, etc.   |
| `device_id`      | string | Same UUID as in the HTTP payload.                      |
| `host`           | string | Short hostname.                                        |
| `listen_port`    | int    | HTTP port for `/api/v1/info` and `/healthz`.           |
| `agent_version`  | string | Matches `agent_version` in agent.toml.                 |

Packets are not authenticated, not encrypted, and not padded. Do not
deploy agents on networks you wouldn't allow `ntpdate -d` to run on.

## Schema evolution

- **Adding a field**: bump `schema_version` and ship a new client. The
  client tolerates unknown fields, so older clients keep working.
- **Removing or renaming a field**: also bump `schema_version` and ship
  the new client in lockstep with the agent.
- **Changing a field type**: same as renaming; never change a `string`
  to an `int` in a way an older client could misread.

The protocol package ships round-trip tests in
`internal/protocol/info_test.go` to lock in JSON shape compatibility.
