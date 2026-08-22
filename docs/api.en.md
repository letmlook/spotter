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
- Query: `tail=N` (default 100, max 1000).

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

## UDP multicast packet (group `239.255.42.42:9999`)

Each agent broadcasts a small JSON packet every 60 s. The packet is the
minimum needed to bootstrap the client before any HTTP round-trip.

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
