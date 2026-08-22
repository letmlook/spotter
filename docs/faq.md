# 常见问答（FAQ）

最常被问到的问题汇总。找不到答案的可以开 question issue
（`.github/ISSUE_TEMPLATE/question.yml`），或者先看
[`troubleshooting.md`](troubleshooting.md)。

英文版本：[`docs/faq.en.md`](faq.en.md)。

## 总览

### Spotter 是什么？

一个二进制的局域网设备发现工具。`spotterd` agent（Go、只 Linux 用
systemd、arm64 + amd64）通过 UDP 组播宣告自己，并对外提供 HTTP 主机信
息。`spotter-client` GUI（Wails、Windows + macOS + Linux）自动发现这些
agent 并以分栏 UI 展示。

### 为什么拆成两个二进制，不做一个？

`spotterd` 必须能在无图形栈的 headless Linux 上运行，因此不能内嵌
client 需要的 webview。把它们分开，每个发布矩阵都很小：agent 两个架构
就够了，client 三个 OS 家族就够了。详见 [`architecture.md`](architecture.md)。

### Spotter 能跨互联网用吗？

**不能。** UDP 组播默认不跨路由器（除非配 IGMP/PIM）。`/api/v1/info`
无认证，按可信 LAN 部署设计。如果放到公网上，任何人都能连到 agent 读
主机信息。

### 有认证吗？

0.1.0 没有。agent 的 HTTP 端点和 UDP 包默认就是开放的（可信 LAN 部署模
式）。未来 milestone 上路线图，详见 issue tracker /
`docs/superpowers/specs/`。

### 为什么用 MIT 许可证？

简短、宽松、几乎兼容所有代码。如果要别的协议，见
[`LICENSE`](../LICENSE)。

## Agent (`spotterd`)

### 为什么 `spotterd` 只支持 Linux？

它以 systemd 单元方式注册。0.1.0 只显式支持 systemd。其它 init 系统是明
确的社区 PR 方向（OpenRC / runit 脚本）。

### 没有 systemd 能跑 `spotterd` 吗？

可以。二进制本身自包含 —— 开发时就是 `go run ./cmd/agent`。生产环境
的安装脚本假设 systemd，因为 unit 文件就是这样部署的。

### `device_id` 怎么生成的？

`scripts/install.sh` 跑一次 `cat /proc/sys/kernel/random/uuid`，写入
`/etc/spotterd/agent.toml`。ID 在重启后保持不变；要重新生成，停止服务、
删除配置目录、重跑 `install.sh`。

### 为什么一行只记一个 IP？

每个 entry 记一个 IP。如果设备换了网络，「按 IP 添加」（或手扫子网再
Accept）重新锚定即可。通过 mDNS 跟随 IP 是后续路线图。

### 默认端口？

HTTP 和 UDP 都是 `9999`。通过 `agent.toml` 的 `listen_addr` 与
`multicast_group` 字段覆盖。

### 为什么 agent 用 `CGO_ENABLED=0` 编译？

静态二进制意味着同一份产物能在任何 glibc / musl 变体上跑。从 macOS
交叉编译到 Linux arm64 也不需要担心 libc 兼容性。Windows 端的 client 由
于 WebView2 不能完全避开 CGO，但 agent 端不需要。

## Client (`spotter-client`)

### 数据存在哪？

设备注册表和日志落在 `<UserConfig>/Spotter/`。各 OS 路径见
[`operations.md`](operations.md)。

### 多个运维人员能共享同一个注册表吗？

0.1.0 不能。注册表是 home 目录下的本地 JSON，共享要靠 Syncthing / NFS 之
类的外部手段。后续版本大概率会引入可选的中心后端。

### 新设备为什么短暂地显示 offline？

scanner 先通过组播发现，再发起一次探测把详情卡片填上。这中间有大约 1 s
的窗口期：行已经存在但 `LastInfo` 还空。这是设计如此，merge pipeline 会
异步把行补齐。

### 不开 GUI 能用 client 吗？

不能，Wails 应用是 GUI-only。如果想要 CLI 版的发现工具，建议：

1. 在目标设备上跑 `spotterd`。
2. 用 `curl` 从任意运维机轮询 `http://<device>:9999/api/v1/info`。

协议字段在 [`api.md`](api.md) 里。

### 未来会有 Android / iOS 客户端吗？

0.x 没有计划。两个平台需要的 Wails 形态差异很大，不适合「单二进制桌
面」这种打包方式。

## 网络与防火墙

### agent 监听哪些端口？

- `9999/udp` —— 组播监听（agent 在组里，不在等服务）。
- `9999/tcp` —— HTTP server（`/healthz`、`/api/v1/info`）。

### client 监听哪些端口？

只有出站：`9999/udp`（加入组播组）、`9999/tcp`（轮询 / 探测）。没有入
站监听。

### 组播地址可以改吗？

可以，通过 `agent.toml` 的 `multicast_group` 字段。client 端要改需要重
新构建 —— 0.1.0 没有 UI 入口。**强烈建议**选 site-local 段
（239.x.x.x），避免泄漏。

## 构建 / 发布

### 产物落在哪？

| 构建命令                           | 产物                              |
| ---------------------------------- | --------------------------------- |
| `make agent-linux-arm64`           | `bin/spotterd-linux-arm64`        |
| `make agent-linux-x64`             | `bin/spotterd-linux-x64`          |
| `make client`（macOS）             | `build/bin/Spotter.app`           |
| `make client`（Linux）             | `build/bin/spotter-client`        |
| `make client`（Windows）           | `build\bin\spotter-client.exe`    |
| `make release` (= `scripts/build-all.sh`) | `dist/`                    |

### release 构建会签名二进制吗？

会 —— GitHub Actions release workflow 把 `SHA256SUMS` 写到 `dist/`。后
续 PR 会加 `cosign` 签名。

### 能在任意主机上交叉编译所有平台吗？

- **Agent**：可以。Go 交叉编译从任何主机（Linux / macOS / Windows）到
  Linux arm64 + amd64 都支持。
- **Client**：**不能**。Wails 只为**当前 GOOS** 构建。在 macOS 上跑
  `scripts/build-all.sh` 拿到 `Spotter.app`，在 Linux 上拿 Linux 二进
  制，在 Windows 上拿 `.exe`。README 里有逐 OS 说明。
