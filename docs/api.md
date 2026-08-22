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

## `POST /api/v1/reboot`

请求设备重启。**仅在 agent 配置 `enable_power_actions = true` 时生效**。

请求：
- Headers：`Content-Type` 不要求；无需 body。

响应（200/202，命令已派发）：
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
enable_power_actions = true   # 默认 false
```

开启后 agent 接受 `POST /api/v1/reboot` 与 `/api/v1/shutdown`。无身份认证，部署方负责网络隔离。

## UDP 组播包（组地址 `239.255.42.42:9999`）

每个 agent 每 60 s 广播一个小型 JSON 包。这个包只包含 client 首次见到
一个设备需要的最小信息。

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

## 协议演进

- **加字段**：递增 `schema_version` 并发布新 client。client 容忍未知字
  段，旧 client 保持可用。
- **删除 / 重命名字段**：同样递增 `schema_version`，client 与 agent 同
  步发布。
- **字段类型变更**：按重命名处理；千万不要把 `string` 改成 `int` 让老
  client 误读。

`protocol` 包自带一组往返测试（`internal/protocol/info_test.go`）来
锁住 JSON 形状兼容。
