# Spotter — 设备发现工具 设计文档

| 项 | 值 |
|---|---|
| 项目名 | **Spotter** |
| 文档日期 | 2026-08-21 |
| 状态 | 设计草案，待实施 |
| 范围 | MVP（首次落地版本） |

---

## 1. 概述

### 1.1 目标
为运维/开发人员提供一套**局域网设备发现与登记工具**：
- 在 Windows 上运行 GUI 客户端，集中查看已部署目标设备
- 通过 SSH 首次推送，向 Linux ARM64 目标设备（含 Jetson）部署一个小型后台服务 `spotterd`
- 客户端**主动**经三种途径发现设备（注册表轮询、UDP 组播、IP 子网扫描）
- 展示设备 IP、端口、用户名、操作系统、内核、架构、Jetson 专属信息（JetPack / L4T / CUDA / cuDNN / TensorRT / Serial）

### 1.2 非目标（Out of Scope, MVP）
- 远程命令执行（仅静态信息展示）
- 多客户端同时管理同一台设备（一对一）
- 实时性能监控（CPU / 内存时序）
- 跨平台客户端（仅 Windows）
- 非 systemd 设备（Yocto / Buildroot 等）
- HTTP 端点认证（仅可信内网使用）
- 国际化（中文优先）

### 1.3 命名约定
| 类别 | 名称 |
|---|---|
| 项目 | `Spotter` |
| 设备端 daemon 二进制 | `spotterd` |
| systemd unit | `spotterd.service` |
| 设备端配置文件 | `/etc/spotterd/agent.toml` |
| 设备端安装目录 | `/etc/spotterd/`、`/usr/local/bin/` |
| 客户端二进制 | `spotter-client.exe`（Wails 打包） |
| 客户端配置目录（Windows） | `%APPDATA%\Spotter\` |
| 客户端注册表文件 | `%APPDATA%\Spotter\devices.json` |
| 客户端日志目录 | `%APPDATA%\Spotter\logs\` |

---

## 2. 网络拓扑与发现策略

### 2.1 典型网络
- 客户端与设备可能位于**同一 LAN 的多个 VLAN / 子网**（跨段可达，无 mDNS/组播转发保证）
- 客户端能 SSH 到目标设备 → 网络层可达 → 端口 9999 也视为可达
- 同 VLAN 内可借助 UDP 组播即时发现新设备

### 2.2 三路并行发现

| 路 | 触发 | 用途 | 覆盖范围 |
|---|---|---|---|
| A 注册表轮询 | 后台常驻，每 30s | 已知设备的健康状态、实时信息 | 注册表中所有设备 |
| B UDP 组播 | 后台常驻，每 60s | 同 VLAN 即插即用自动发现 | 同 VLAN（依赖路由器组播转发） |
| C IP 子网扫描 | 用户手动触发 | 跨 VLAN / 主动探测 | 用户指定 CIDR |

三路结果汇入同一个 merge pipeline，**以 device_id 为 key 去重**，最后通过 Wails 事件总线推送给前端。

### 2.3 组播地址
- **239.255.42.42:9999**（站点本地管理范围，默认不跨路由器）

### 2.4 端口
- TCP **9999**（HTTP，设备端监听）
- UDP **9999**（组播，设备端加入组）

同一端口便于防火墙统一放行。

---

## 3. 整体架构

```
┌──────────────────────────┐                  ┌────────────────────────────────┐
│  spotter-client (Wails)  │                  │  目标设备 (Linux)              │
│                          │   TCP/UDP 主动    │                                │
│  · HTTP 客户端 (轮询)     │ ───────────────► │  · spotterd                    │
│  · UDP 组播客户端        │                  │    · HTTP server :9999         │
│  · TCP 子网扫描器        │                  │    · UDP multicast listener    │
│  · Registry (本地 JSON)  │                  │    · systemd unit              │
│  · Wails Web 前端        │                  │    · /etc/spotterd/agent.toml  │
└──────────────────────────┘                  └────────────────────────────────┘
                                                       ▲
                                                       │  手动 scp + ssh（参考 README）
                                                       │  执行 scripts/install.sh
```

注：客户端**不再**包含 SSH 部署 / 卸载能力 —— spotterd 由用户在目标设备上通过 `scripts/install.sh`（或等价的 `scp`+`ssh` 步骤）手动安装。客户端只负责发现与展示。

---

## 4. 仓库结构

```
device_discovery/
├── cmd/
│ ├── agent/                  # spotterd 入口
│ │ └── main.go
│ └── client/                 # spotter-client (Wails) 入口
│       └── main.go
├── internal/
│ ├── protocol/               # 共享：HTTP/UDP 消息 schema
│ │ ├── info.go
│ │ └── udp.go
│ ├── collector/              # 设备端：信息采集（基础 + Jetson）
│ │ ├── collector.go
│ │ ├── basic_linux.go
│ │ ├── network_linux.go
│ │ └── jetson_linux.go
│ ├── agentd/                 # 设备端：业务编排（HTTP + UDP + collector）
│ │ ├── agent.go
│ │ ├── http.go
│ │ └── udp.go
│ ├── registry/               # 客户端：本地注册表持久化
│ │ └── registry.go
│ └── scanner/                # 客户端：三路发现 + 合并
│ ├── poll.go
│ ├── mcast.go
│ ├── subnet.go
│ └── merge.go
├── scripts/                  # 手动安装 / 卸载 / 回滚时使用的脚本（与发布包同发）
│ ├── install.sh              # 设备端安装脚本（手动 ssh 调用）
│ ├── uninstall.sh            # 设备端卸载脚本（手动 ssh 调用）
│ ├── cleanup.sh              # 失败后手动回滚脚本
│ └── spotterd.service        # systemd unit 文件
├── frontend/                 # Wails 前端 (Vite + React + TS)
│ ├── index.html              # Vite 入口 HTML
│ ├── package.json            # npm 依赖与 scripts
│ ├── vite.config.ts          # Vite 配置
│ ├── tsconfig.json           # TypeScript 配置
│ └── src/                    # 应用源码
│   ├── main.tsx              # React 入口
│   ├── App.tsx               # 根组件
│   ├── styles.css            # 全局样式
│   ├── components/           # UI 组件
│   ├── hooks/                # 自定义 hooks
│   ├── state/                # 状态管理（Context）
│   └── utils/                # 工具函数
├── docs/
│ └── superpowers/
│ └── specs/
│ └── 2026-08-21-spotter-design.md
├── go.mod
├── go.sum
├── wails.json
└── README.md
```

---

## 5. 手动安装与卸载

客户端**不**驱动安装 / 卸载 —— 这是用户的责任。发布包内随附 `scripts/install.sh`、`scripts/uninstall.sh`、`scripts/cleanup.sh` 与 `scripts/spotterd.service` 作为人工流程的脚手架。

### 5.1 手动安装流程

```
[开发者机器]                                  [目标 Linux 设备]
make agent-linux-arm64
       │
       ▼
bin/spotterd-linux-arm64, scripts/install.sh,
scripts/spotterd.service
       │  scp
       ▼
/tmp/spotterd, /tmp/install.sh, /tmp/spotterd.service
       │  ssh
       ▼
SPOTTER_AGENT_VERSION=<x.y.z> sudo bash /tmp/install.sh
   ├─ install -m 0755 /tmp/spotterd /usr/local/bin/spotterd
   ├─ DEVICE_ID="${DEVICE_ID:-$(cat /proc/sys/kernel/random/uuid)}"
   ├─ mkdir -p /etc/spotterd
   ├─ 写 /etc/spotterd/agent.toml { device_id, listen_addr, multicast_group, agent_version }
   ├─ install -m 0644 /tmp/spotterd.service /etc/systemd/system/spotterd.service
   ├─ systemctl daemon-reload
   ├─ systemctl enable --now spotterd
   └─ echo "DEVICE_ID=$DEVICE_ID" 供人工抄录到客户端
```

设备启动后，客户端通过 UDP 组播或子网扫描自动发现，无需在 GUI 上做注册动作。客户端不会保存 SSH 凭据，也不需要任何"deploy"交互。

### 5.2 install.sh

```bash
#!/usr/bin/env bash
set -euo pipefail

install -m 0755 /tmp/spotterd /usr/local/bin/spotterd

DEVICE_ID="${DEVICE_ID:-$(cat /proc/sys/kernel/random/uuid)}"

mkdir -p /etc/spotterd
cat >/etc/spotterd/agent.toml <<EOF
device_id = "$DEVICE_ID"
listen_addr = "0.0.0.0:9999"
multicast_group = "239.255.42.42:9999"
agent_version = "${SPOTTER_AGENT_VERSION:-0.1.0}"
EOF

install -m 0644 /tmp/spotterd.service /etc/systemd/system/spotterd.service
systemctl daemon-reload
systemctl enable --now spotterd

echo "DEVICE_ID=$DEVICE_ID"
```

### 5.3 手动卸载流程

```bash
ssh user@<device> sudo bash /tmp/uninstall.sh
```

`scripts/uninstall.sh` 在设备本地完成：

```
systemctl stop spotterd
systemctl disable spotterd
rm /etc/systemd/system/spotterd.service
rm /usr/local/bin/spotterd
rm -rf /etc/spotterd
systemctl daemon-reload
```

卸载完成后，客户端在下次轮询超时（≈90s）后自动将设备标记 offline，但**不会**自动从注册表删除条目 —— 用户可在客户端用 "Clear" 按钮或 `ProbeByIP` / `AcceptUnknownDevice` 的反向操作手动管理注册表。

### 5.4 install.sh 失败回滚

| 失败阶段 | 用户动作 |
|---|---|
| scp 失败 | 重新 scp，残留 `/tmp` 文件下次覆盖 |
| install.sh 中途失败 | ssh 进设备执行 `bash /tmp/cleanup.sh`（stop+disable+rm 已写入的部分） |
| install.sh 成功但客户端未发现 | 检查网络连通性；用客户端 "Add by IP" 输入 `<ip>:9999` 手动注册 |

---

## 6. 协议设计

### 6.1 HTTP 端点（设备端 :9999）

#### `GET /healthz`
```
200 OK
Content-Type: text/plain

ok
```
用途：TCP 探测 / 子网扫描快速判断"端口在听"。

#### `GET /api/v1/info`
```
200 OK
Content-Type: application/json
```

Body：
```json
{
  "schema_version": 1,
  "device_id": "5f3a1c9b-...-...",
  "collected_at": "2026-08-21T10:23:45Z",
  "agent_version": "0.1.0",
  "basic": {
    "hostname": "jetson-01",
    "username": "nvidia",
    "os": {
      "pretty_name": "Ubuntu 22.04.4 LTS",
      "id": "ubuntu",
      "version_id": "22.04"
    },
    "kernel": "5.15.122-tegra",
    "arch": "aarch64",
    "uptime_seconds": 1234567
  },
  "network": {
    "primary_ip": "10.0.5.23",
    "interfaces": [
      {"name": "eth0", "mac": "aa:bb:cc:dd:ee:ff", "addrs": ["10.0.5.23/24"]},
      {"name": "wlan0", "mac": "11:22:33:44:55:66", "addrs": ["192.168.1.10/24"]}
    ]
  },
  "jetson": null
}
```

`jetson` 字段在非 Jetson 或探测失败时为 `null`，**绝不阻塞响应**。

### 6.2 UDP 组播消息

**HELLO**（客户端 → 组播 239.255.42.42:9999）
```json
{"type":"hello","sender_id":"<client_uuid>","ts":"2026-08-21T10:00:00Z"}
```

**HELLO-REPLY**（设备 → 客户端单播，发送到 HELLO 来源 IP:port）
```json
{
  "type":"hello_reply",
  "device_id":"5f3a1c9b-...-...",
  "info": { ... 同一份 DeviceInfo ... }
}
```

### 6.3 版本演进
- 顶层 `schema_version`（当前 1）
- 客户端按版本路由解析逻辑
- 字段新增向后兼容；删除/重命名需 bump major version

---

## 7. 组件接口

### 7.1 `internal/protocol`
零依赖纯类型包。导出 `DeviceInfo / BasicInfo / OSInfo / NetworkInfo / Interface / JetsonInfo / HelloPacket / HelloReply / SchemaVersion`。

### 7.2 `internal/collector`
```go
type Collector struct { /* ... */ }
func New() *Collector
func (c *Collector) Collect(ctx context.Context) (protocol.DeviceInfo, error)

type JetsonProbe interface {
    Name() string
    Probe(ctx context.Context) (*protocol.JetsonInfo, error)
}
```
实现要点：
- **基础**：`/etc/os-release`、hostname、`uname -r/-m`、`uptime`、`whoami`
- **网络**：枚举 `/sys/class/net/`，过滤 lo，解析 `/proc/net/fib_trie` 或 `ip route` 取主 IP
- **Jetson 信息**：以下步骤**独立叠加**，每步成功就填充对应字段，失败跳过：
  1. `jetson_release -v` → model / jetpack / l4t / cuda / cudnn / tensorrt / python
  2. `/etc/nv_tegra_release`（L4T 兜底）+ `/proc/device-tree/model`（model 兜底）
  3. serial 从 `/sys/firmware/devicetree/base/serial-number`
  4. CUDA/cuDNN/TensorRT：从 `/usr/local/` 版本文件独立探测
- 失败兜底：Jetson 字段可能仅含部分信息（如只有 serial 没有 model），绝不阻塞基础信息

### 7.3 `internal/agentd`
```go
type Config struct {
    DeviceID       string
    ListenAddr     string
    MulticastGroup string
    AgentVersion   string
}

type Agent struct { /* ... */ }

func New(cfg Config, log *slog.Logger) (*Agent, error)
func (a *Agent) Start(ctx context.Context) error  // 阻塞至 ctx 取消
func (a *Agent) Info() protocol.DeviceInfo          // 同步获取当前缓存
```
- `Start` 内部：`http.Server` + `udp.Conn` 加入组播 + goroutine 调 collector 填充 info
- HTTP handler 直接返回缓存（避免每次跑 collector）
- UDP handler：解析 HELLO → 单播 REPLY（含 device_id + info）

### 7.4 `cmd/agent/main.go`
flags：`--config` / `--device-id` / `--listen-addr` / `--multicast` / `--log-level`
逻辑：flags → toml → `agentd.New` → `agent.Start(ctx)`，监听 SIGTERM/SIGINT 优雅退出。

### 7.5 `internal/registry`
```go
type Entry struct {
    DeviceID    string             `json:"device_id"`
    IP          string             `json:"ip"`
    Port        int                `json:"port"`
    Username    string             `json:"username"`
    DeployedAt  string             `json:"deployed_at"`
    LastSeenAt  string             `json:"last_seen_at"`
    LastSource  string             `json:"last_source"`
    Online      bool               `json:"online"`
    LastInfo    *protocol.DeviceInfo `json:"last_info,omitempty"`
}

type Registry struct { /* ... */ }

func Open(path string) (*Registry, error)
func (r *Registry) Add(e Entry) error
func (r *Registry) Remove(deviceID string) error
func (r *Registry) Update(deviceID string, mut func(*Entry)) error
func (r *Registry) Get(deviceID string) (*Entry, bool)
func (r *Registry) List() []*Entry
func (r *Registry) FindByIP(ip string, port int) (*Entry, bool)
```
- 文件：`<path>/devices.json`
- 每次 mutation 后写整个 JSON 文件
- 损坏处理：备份 `devices.json.corrupt-<timestamp>` → 启动空表

### 7.6 `internal/scanner`
```go
type Scanner struct {
    reg       *registry.Registry
    httpClient *http.Client
    log       *slog.Logger
    onEvent   func(Event)
}

type Event interface { eventTag() string }

type EventInfoUpdated struct { Entry *registry.Entry }
type EventOffline      struct { DeviceID string }
type EventUnknownDeviceDiscovered struct { Info protocol.DeviceInfo }

func New(reg *registry.Registry, log *slog.Logger) *Scanner
func (s *Scanner) Start(ctx context.Context)
func (s *Scanner) ScanSubnet(ctx context.Context, cidr string) error
func (s *Scanner) RefreshNow(ctx context.Context)
```
- `pollLoop`：每 30s 遍历注册表 → 并发 HTTP GET
- `mcastLoop`：每 60s 发 HELLO + 收集 REPLY
- `merge()`：以 device_id 为 key 合并 → 调 reg.Update + 发 Event

### 7.7 `cmd/client/main.go` (Wails)
导出给前端 API（Wails 自动绑定，方法名首字母大写）：
- `ListDevices() []registry.Entry`
- `ScanSubnet(cidr string) error`
- `RefreshNow() error`
- `ProbeByIP(ip string, port int, username string) (registry.Entry, error)`
- `AcceptUnknownDevice(deviceID, ip string, port int, username string) (registry.Entry, error)`
- `ClearRegistry() (int, error)`

事件推送使用 Wails 框架：`scanner` 调用 `runtime.EventsEmit(ctx, "device:info-updated", payload)`，前端通过 `EventsOn('device:info-updated', handler)` 订阅；不暴露额外 Go API。

---

## 8. 前端 UI（Wails）

栈：Vanilla HTML + ES Modules + CSS，不引入前端框架。

### 8.1 布局
- 顶部工具栏：`+ 部署设备` `扫描子网` `刷新` `最近错误 N`
- 左：设备列表（表格 / 卡片）
- 右：详情面板（选中设备的 DeviceInfo 渲染）
- 底部：状态栏（在线数 / 总数 / 最后扫描时间）

### 8.2 事件订阅
Wails `EventsOn('device-event', handler)` 监听后端推过来的 `EventXxx`。

### 8.3 详情面板字段
- 基本信息：hostname / username / OS / kernel / arch / uptime
- 网络：primary_ip + interfaces 列表
- Jetson（若有）：model / jetpack / l4t / cuda / cudnn / tensorrt / python / serial
- 设备 ID / 上次见到时间 / 来源（轮询/组播/子网）

---

## 9. systemd unit

```ini
[Unit]
Description=Spotter Device Discovery Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/spotterd --config /etc/spotterd/agent.toml
Restart=on-failure
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60

[Install]
WantedBy=multi-user.target
```

---

## 10. 可靠性 & 错误处理

### 10.1 设备端

| 场景 | 处理 |
|---|---|
| 读 agent.toml 失败 | 退出码非 0，systemd 重启 |
| 端口 :9999 占用 | 启动失败，systemd 退避重启 10s |
| 加入组播失败 | HTTP 仍起；仅日志 WARN |
| collector 部分失败 | info 字段尽力填充，不返回 error |
| HTTP handler panic | recover middleware + 500 + 日志 ERROR |
| UDP malformed HELLO | 静默丢弃，日志 DEBUG |
| SIGTERM | http.Shutdown + UDP close + ctx 取消 |

### 10.2 客户端 scanner

| 场景 | 处理 |
|---|---|
| HTTP 超时 >3s | 不立刻 offline |
| TCP refused | 记一次 fail |
| HTTP 4xx | 标记 `incompatible`（不当 offline） |
| HTTP 5xx | 记一次 fail，连续 3 次 → offline |
| 路由不可达 | 立即 offline |
| 离线判定 | **连续 3 轮失败**（≈90s） |
| 恢复判定 | offline 状态下任一轮询成功 → online |
| UDP 无 reply | 正常（可能 VLAN 无设备） |
| reply device_id 未注册 | 推 `EventUnknownDeviceDiscovered` |
| CIDR 解析错 | UI 报错 |
| CIDR >4096 IP | 拒绝，要求缩小范围 |
| 单 IP TCP 超时 500ms | 跳过，标记 unreachable |
| 进度推送 | 每扫 64 个 IP 推一次 |

### 10.3 注册表文件损坏
- 加载失败 → 备份为 `devices.json.corrupt-<timestamp>` → 启动空表 → 日志 WARN

### 10.4 install.sh 鲁棒性
- scp 失败：用户重新 scp；`/tmp` 残留文件下次覆盖即可
- install.sh 中途失败：用户 ssh 进设备执行 `scripts/cleanup.sh`（stop+disable+rm 已写入的部分）
- install.sh 成功但客户端未发现：用户用客户端 "Add by IP" 输入 `<ip>:9999` 手动注册

### 10.5 可观测性

**设备端**：slog JSON → stdout → journal
- 级别：`--log-level`（默认 info）
- 关键事件：`started`、`http-listening`、`mcast-joined`、`hello-reply-sent`、`shutdown`

**客户端**：slog JSON → `%APPDATA%\Spotter\logs\spotter.log`（每日 rotate）
- UI 角标显示"最近错误 N"
- 关键事件：`deploy-start/success/fail`、`device-online`、`device-offline`、`scan-start/end`

---

## 11. 安全模型

### 11.1 MVP 默认状态

| 项 | 状态 |
|---|---|
| HTTP 端点 | 无认证 |
| UDP 组播 | 无认证 |
| agent.toml | 明文 |
| spotter-client | 单机使用 |

**风险接受**：MVP 部署在内网/可信环境，敏感信息（serial、IP 拓扑）泄露可控。客户端不再接触 SSH 凭据 —— spotterd 由用户手动安装，SSH 凭据只存在于用户本地 `scp` / `ssh` 调用链中，客户端不缓存。

### 11.2 MVP 升级路径（v1.x 留口子，不实现）
- HTTP `Authorization: Bearer <token>`，token 由用户首次手动写入 agent.toml
- token 通过 SSH 安全通道首次分发

### 11.3 攻击面缓解（MVP）
| 攻击面 | 缓解 |
|---|---|
| 任意客户端读 :9999 | 部署内网 + 防火墙限制源 IP |
| 任意 HELLO | 设备端按来源 IP 限速（10 req/s） |
| agent.toml 篡改 | v1.x 加 token 认证 |
| 手动安装流程注入 | `install.sh` 不解析用户输入，仅用客户端生成的 DEVICE_ID（v1.x）或首次运行时生成的 DEVICE_ID（MVP） |

---

## 12. 测试策略

### 12.1 分层
- **Unit**（Go test，无外部依赖）：protocol / collector / registry / agentd handler
- **Integration**（dockertest）：scanner（httptest + loopback UDP）；install.sh 在 Ubuntu 容器内的 smoke test
- **E2E**（手动）：真机 Jetson / arm64 Ubuntu VM 验收

### 12.2 关键用例

| 包 | 用例 |
|---|---|
| `protocol` | JSON round-trip；Jetson nil vs object；schema_version |
| `collector` | mock `/etc/os-release`；Jetson 4 路径覆盖 |
| `registry` | Add/Update/Remove；持久化 round-trip；损坏文件恢复 |
| `agentd` | HTTP `/info` 返回缓存；UDP HELLO → REPLY；端口冲突退出码非 0 |
| `scanner` | mock HTTP timeout/5xx/OK；mock UDP reply；merge 去重 |
| `scripts/install.sh` | dockertest Ubuntu 容器内：调用 → service active → `uninstall.sh` → gone |

### 12.3 手动验收
1. 手动安装真机 → 30s 内 online
2. UI 显示完整字段
3. 同 VLAN 设备 60s 内出现在 UI
4. 子网扫描命中 + 未注册设备均显示
5. 杀 spotterd → 90s 内 UI offline；重启 → UI online
6. 客户端 "Add by IP" 手动注册已知设备并显示

---

## 13. 已知限制（写入 README）

- 不支持非 systemd 设备（Yocto / Buildroot）
- 不做远程命令执行
- 同 VLAN UDP 组播需路由器允许组播转发（默认多数不通）
- HTTP 端点无认证，仅适合可信内网
- 客户端不部署 / 不卸载 spotterd —— 由用户在设备上手动执行 `scripts/install.sh` / `scripts/uninstall.sh`

---

## 14. 实现阶段（粗略路线）

1. **协议与共享类型**：`internal/protocol`
2. **设备端基础**：`collector` + `agentd` + `cmd/agent` + `scripts/install.sh` + `scripts/spotterd.service`
3. **客户端基础**：`registry` + `scanner` + `cmd/client` 最小骨架（Wails 占位）
4. **scanner 三路发现**：`scanner/poll.go` + `scanner/mcast.go` + `scanner/subnet.go` + `scanner/merge.go`
5. **Wails 前端 UI**：`frontend/` (Vite + React + TS)
6. **集成测试**：dockertest scanner 端到端 + install.sh 在 Ubuntu 容器内的 smoke
7. **手动验收**：真机 Jetson

---

## 附录 A：完整目录树（实施后预期）

```
device_discovery/
├── cmd/
│ ├── agent/main.go
│ └── client/main.go
├── internal/
│ ├── protocol/{info.go,udp.go,info_test.go,udp_test.go}
│ ├── collector/{collector.go,basic_linux.go,network_linux.go,jetson_linux.go,*_test.go}
│ ├── agentd/{agent.go,http.go,udp.go,*_test.go}
│ ├── registry/registry.go,registry_test.go
│ └── scanner/{poll.go,mcast.go,subnet.go,merge.go,*_test.go}
├── scripts/{install.sh,uninstall.sh,cleanup.sh,spotterd.service}
├── ui/{index.html,app.js,styles.css}  ← 已迁移至 frontend/ (Vite + React + TS)
├── frontend/                 # Vite 项目根
│ ├── index.html              # Vite 入口 HTML
│ ├── package.json            # npm 依赖与 scripts
│ ├── vite.config.ts          # Vite 配置
│ ├── tsconfig.json           # TypeScript 配置
│ └── src/                    # 应用源码
│   ├── main.tsx              # React 入口
│   ├── App.tsx               # 根组件
│   ├── styles.css            # 全局样式
│   ├── components/           # UI 组件
│   ├── hooks/                # 自定义 hooks
│   ├── state/                # 状态管理（Context）
│   └── utils/                # 工具函数
├── docs/superpowers/specs/2026-08-21-spotter-design.md
├── go.mod, go.sum, wails.json, README.md
```