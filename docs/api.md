# 协议与 HTTP API 参考

Spotter 用的是一套很小的 JSON-over-HTTP，外加一个小型 UDP 包。两个端
口都通过 `schema_version` 字段做版本控制，将来破坏性变更走
`/api/vN/info`，而不必一次性升所有设备。

英文版本：[`docs/api.en.md`](api.en.md)。

> 权威源是 [`internal/protocol/info.go`](../internal/protocol/info.go)
> 里的 Go 结构。下面贴出的形状对应 `DeviceInfo` 及其嵌套结构；如果
> 哪天出现冲突，以**代码**为准 —— 上线的就是它。

## 通用约定

- 所有时间戳都是 UTC，RFC3339。
- 所有大小都是字节（uint64）。
- 所有时长都是秒（float64）。
- 解码器**容忍**未知 / 新字段；缺失必填字段会报清晰的错误。
- 默认监听端口是 `9999`（HTTP 与 UDP 皆同）。在
  `/etc/spotterd/agent.toml` 的 `listen_addr` 与 `multicast_group` 里
  覆盖。

## `GET /healthz`

存活探针 —— 没有 body。HTTP 循环还在则返回 `200 OK`，否则连接被拒。
client 的手动子网扫描用它过滤候选，然后再请求 `/api/v1/info`。

## `GET /api/v1/info`

发现用的载荷。解码为 `protocol.DeviceInfo` 结构。

### 响应（`200 OK`）

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

| 字段             | 类型     | 说明                                                   |
| ---------------- | -------- | ------------------------------------------------------ |
| `schema_version` | int      | 不向后兼容变更时递增。                                 |
| `device_id`      | string   | UUID v4，来源于 `/etc/spotterd/agent.toml`。           |
| `hostname`       | string   | `hostname` 输出（短形式）。                           |
| `agent_version`  | string   | 与 agent.toml 里的 `agent_version` 一致。              |
| `collected_at`   | string   | 载荷采集时刻，RFC3339。                                |
| `basic`、`network`、`jetson` | object | 可选采集器；非 Linux / 非 Jetson 上会缺失。 |

`extra` 是 `map[string]any`，留给采集器特有的扩展字段，等将来提到一级
字段。

### `basic` 字段

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

值为 Go 零值的字段会**故意**从输出里省略（JSON encoder 行为）。

### `network` 字段

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

采集器会过滤掉 loopback (`lo`) 与 link-local IPv6。

### `jetson` 字段

仅在 Jetson 硬件上出现；其他机器上为 `null`。

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

`jetson` 字段为 nil / 缺失说明这台不是 Jetson。**判别时以字段是否出现
为准，不要按字段名做模式匹配。**

## `GET /api/v1/logs?tail=N`

流式返回设备端软件的执行日志（默认 `journalctl -u spotterd.service`）。

请求：
- Headers：`Accept: application/x-ndjson`（文档化用，agent 不强制校验）。
- Query 参数：
  - `tail=N`（默认 100，上限 1000）—— 回放历史行数。
  - `unit=foo,bar`（逗号分隔多 unit 订阅；空 = 仅默认 unit）。配置的默认 unit 始终叠加在 `?unit=` 之前，所以单独传 `?unit=nginx` 不会"覆盖" agent 自己的日志。
  - `grep=REGEX` 走 `journalctl --grep`，case-sensitive 正则过滤；空 = 不过滤。
  - `since=SPEC` 走 `journalctl --since`，SPEC 是 free-form（`5min ago` / `2026-08-27 12:00:00` / `yesterday`）；空 = 起点不限，靠 `tail` 给历史。
  - `priority=LEVEL` 走 `journalctl --priority`，LEVEL ∈ `emerg|alert|crit|err|warning|notice|info|debug`（journalctl 数字 0-7 也可）；空 = 不限。

响应（200，NDJSON 流）：
- 每行一个 JSON 对象：journalctl `--output=json` 的原始结构（包括 `__REALTIME_TIMESTAMP`、`MESSAGE`、`__CURSOR` 等）。
- 行为：先回放最近 N 行历史，再 follow 新增行；客户端断开后流终止。

响应（403，未启用）：
- 文本 `log streaming disabled`（agent 配置 `enable_log_stream` 缺失或为 false）。

响应（405，非 GET）：文本 `method not allowed`。

## `enable_log_stream` / `log_unit`

`/etc/spotterd/agent.toml`：

```toml
enable_log_stream = true   # 默认 true
log_unit = "spotterd.service"  # 默认
```

开启后 agent 暴露 `GET /api/v1/logs`。无身份认证，部署方负责网络隔离；日志内容可能含敏感信息，开启前评估数据敏感性。

## `POST /api/v1/reboot`

请求设备重启。**仅在 agent 配置 `enable_power_actions = true` 时生效**。

请求：
- Headers：`Content-Type` 不要求；无需 body。

响应（202，命令已派发）：
```json
{
  "status": "scheduled",
  "action": "reboot"
}
```

响应（403，未启用）：
```json
{
  "error": "power actions disabled"
}
```

响应（405，非 POST）：文本 `method not allowed`。

## `POST /api/v1/shutdown`

同 reboot，但调用 `systemctl poweroff`。**该操作不可逆**，需手动上电才能恢复。

## `enable_power_actions`

`/etc/spotterd/agent.toml`：

```toml
enable_power_actions = true   # 默认 true
```

开启后 agent 接受 `POST /api/v1/reboot` 与 `/api/v1/shutdown`。无身份认证，部署方负责网络隔离。如需关闭，显式设置 `enable_power_actions = false`。

## `POST /api/v1/power`

统一电源操作端点（v0.5 新增）。支持立即执行、延迟执行与 dry-run，由 `request_id` 关联审计与取消。**仅在 `enable_power_actions = true` 时生效**。

请求（`Content-Type: application/json`）：

```json
{
  "action": "reboot",
  "dry_run": false,
  "delay_minutes": 0,
  "request_id": "ops-2026-08-27-001"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `action` | string | 是 | `reboot` 或 `shutdown`。 |
| `dry_run` | bool | 否 | `true` 时只回显 `would_execute`，不调度任何命令。 |
| `delay_minutes` | int | 否 | 0 立即；1–1440 表示延后分钟数（最长 24 小时）。 |
| `request_id` | string | 否 | 客户端生成的去重键；同一 ID 多次提交时由调用方保证幂等。`cancel` 端点按此 ID 关联。 |

响应（202，命令已派发或已调度）：

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

| `status` | 触发条件 |
| --- | --- |
| `scheduled` | 已调用 `systemctl`，或 `delay_minutes > 0` 已入队延迟执行。 |
| `would_execute` | `dry_run=true`，不实际派发。 |
| `running` | 内部状态（保留），由审计日志使用。 |
| `cancelled` | 由 `cancel` 端点取消后的延迟任务（仅 v0.6 实现后产出）。 |

`delay_minutes > 0` 时响应额外带 `execute_at`（RFC3339 UTC，ISO 格式），表示计划执行时刻。

错误响应：
- 400：`action` 非 `reboot/shutdown`；`delay_minutes` 越界（< 0 或 > 1440）；body 解析失败。
- 403：`power actions disabled`（`enable_power_actions = false`）。
- 405：非 POST。

## `GET /api/v1/power/audit`

返回 agent 进程内的电源操作审计日志。`Content-Type: application/x-ndjson`，每行一个 JSON 对象（TSV 化的内部审计行 NDJSON 化导出），按时间顺序追加。

- 200：NDJSON 流（`io.Copy` 32 KB chunk，文件为空时 body 为空）。
- 405：非 GET。
- 503：`audit log unavailable` / `audit not open`（审计日志未初始化或被关闭，理论上启动时已就绪）。

## `GET /api/v1/power/audit/recent`

返回最近 N 条审计记录作为 JSON 数组（区别于 `/audit` 的 NDJSON 流）。GUI 在 DetailPanel 渲染"最近电源活动"列表时用这个端点。

请求：
- Query：`limit=N`（默认 50，上限 200，>200 自动 cap 到 200）。

响应（200）：

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

错误响应：
- 405：非 GET。
- 503：`audit log unavailable`（agent 未初始化审计 logger）。

> 内部格式：审计文件仍是 TSV（`/var/log/spotterd/audit.tsv`），字段顺序 `timestamp \t action \t dry_run=... \t req=... \t ip=... \t status=...`。解码器跳过格式错误的行（避免单行损坏让整页空白），但不会修改文件。

## `POST /api/v1/power/cancel`

按 `request_id` 取消一个**已调度但尚未执行**的延迟电源操作。取消走 agent 进程内的 cancel channel map（`Agent.pending`，`request_id` → `chan struct{}`），立即关闭 channel 让 `delayExec` 退出 select 而不调用 `systemctl`。

请求（`Content-Type: application/json`）：

```json
{
  "request_id": "ops-2026-08-27-001"
}
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `request_id` | string | 是 | 与 dispatch 时使用的 request_id 一致。 |

响应（202，已取消）：

```json
{
  "status": "cancelled",
  "request_id": "ops-2026-08-27-001"
}
```

错误响应：
- 400：`request_id` 缺失。
- 403：`power actions disabled`（`enable_power_actions = false`）。
- 404：`no pending action with that request_id` —— 该 ID 不存在、已执行、或 agent 重启后内存 map 清空。GUI 仍可乐观显示「已取消」（v0.6 计划持久化到 pid-file 跨重启）。
- 405：非 POST。

> **v0.6 计划**：将 in-memory map 升级为 `/var/run/spotterd/power-pending/<request_id>.json`（含 `pid`），cancel 端点发 SIGINT 给目标 PID，跨 agent 重启仍能取消。当前实现为 v0.5 in-memory 版，已能覆盖「同进程内」所有取消场景。

## UDP 组播包（组地址 `239.255.42.42:9999`）

每个 agent 周期性地（默认 5s，由 `hello_interval` 控制）在组播组上广播一个小型 JSON 包 —— 既响应 client 主动发来的 HELLO（HELLO_REPLY），也**主动**发自己的 HELLO 让 client 在没有反向请求的情况下也能立即识别上线。这个包只包含 client 首次见到一个设备需要的最小信息。

```
{
  "tag": "hello",
  "device_id": "9d1f2c5e-…",
  "host": "nvidia-orin-1",
  "listen_port": 9999,
  "agent_version": "0.1.0"
}
```

| 字段           | 类型   | 说明                                                    |
| -------------- | ------ | ------------------------------------------------------- |
| `tag`          | string | `hello`。预留给将来 `ping`、`goodbye` 等。              |
| `device_id`    | string | 与 HTTP 载荷里的 UUID 一致。                            |
| `host`         | string | 短主机名。                                              |
| `listen_port`  | int    | HTTP 端口（`/api/v1/info` 与 `/healthz`）。             |
| `agent_version`| string | 与 agent.toml 的 `agent_version` 一致。                 |

包没有认证、没有加密、也没有 padding。**请只在和 `ntpdate -d` 同等级别
可信任的网络里部署 agent。**

## `GET /api/v1/metrics/recent`

返回 agent 端**最近 5 分钟的 CPU / 内存 / 温度时序样本**（环形缓冲，60 槽 × 5s 间隔）。GUI 用作 sparkline / mini chart 数据源。

> 写入：`internal/agentd/metrics.go` 的 sampler goroutine 在 agent 启动时通过 `SetLifecycleContext` 注入；与 UDP/heartbeat loop 同一 ctx，SIGTERM 驱动干净退出。

响应（200）：

```json
{
  "interval_seconds": 5,
  "samples": [
    { "at": "2026-08-27T22:00:00Z", "cpu_percent": 12.3, "mem_percent": 41.5, "mem_used_bytes": 6816972800, "temp_celsius": 48.0 },
    { "at": "2026-08-27T22:00:05Z", "cpu_percent": 15.1, "mem_percent": 41.6, "mem_used_bytes": 6826213376, "temp_celsius": 49.0 }
  ]
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `interval_seconds` | int | 采样间隔（秒）。 |
| `samples[].at` | string (RFC3339 UTC) | 采样时刻。 |
| `samples[].cpu_percent` | float | 区间平均 CPU 占用百分比，0–100，0.1 精度。第一次采样为 0（无 prior tick）。 |
| `samples[].mem_percent` | float | 当前内存占用百分比（基于 MemTotal - MemAvailable），0.1 精度。 |
| `samples[].mem_used_bytes` | uint64 | 占用字节数。 |
| `samples[].temp_celsius` | float | 第一个 `thermal_zone*` 的温度（℃），无 sensor 时字段缺省（`omitempty`）。 |

错误响应：
- 405：非 GET。
- 503：`metrics not started`（agent 启动尚未完成 init 路径，理论上 1s 内自动恢复）。

> **资源开销**：sampler 每次 collect 读 3 个 procfs/sysfs 文件 + 1 次时间格式化，< 50µs。环形缓冲固定 60 × ~80B ≈ 5KB 内存。

## 协议演进

- **加字段**：递增 `schema_version` 并发布新 client。client 容忍未知字
  段，旧 client 保持可用。
- **删除 / 重命名字段**：同样递增 `schema_version`，client 与 agent 同
  步发布。
- **字段类型变更**：按重命名处理；千万不要把 `string` 改成 `int` 让老
  client 误读。

`protocol` 包自带一组往返测试（`internal/protocol/info_test.go`）来
锁住 JSON 形状兼容。
