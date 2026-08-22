# 架构

本文档是对
[`docs/superpowers/specs/2026-08-21-spotter-design.md`](../superpowers/specs/2026-08-21-spotter-design.md)
中设计说明的补充，从运维视角描述两个二进制如何协同。设计取舍与历史理由请看 spec，本文档只解释**结构本身**。

英文版本：[`docs/architecture.en.md`](architecture.en.md)。

## 一图概览

```
┌──────────────────────────────────────────────┐   ┌─────────────────────┐
│ spotter-client（Wails + React GUI）          │   │ spotterd（agent）   │
│ ─────────────────────────────────────────── │   │ ────────────────── │
│ registry（本地 JSON）  ◀── 30 s 轮询 ───────┼──┤  GET /api/v1/info   │
│                       ◀── UDP 组播 ◀─────┼──┤  UDP HELLO @ 60 s    │
│ 手动子网扫描         ─── TCP 探活 + HTTP ─┼──┤  GET /healthz       │
└──────────────────────────────────────────────┘   └─────────────────────┘
        ▲                                              ▲ 以
        │                                              │ systemd 单元
  主机（macOS / Windows / Linux）                     │ 运行
                                                       │
                                                       ▼
                                              Linux（systemd 主机）
                                              （arm64 或 amd64）
```

项目里就只有这两个二进制，其余都是被它们共享的库代码（`internal/`、`cmd/agent/`）。

## 包结构映射

| 路径                            | agent？ | client？ | 用途                                                          |
| ------------------------------- | :-----: | :------: | ------------------------------------------------------------ |
| `cmd/agent/`                    | ✅      | —        | `spotterd` 入口（仅 Linux）。                                |
| `internal/agentd/`              | ✅      | —        | HTTP server、UDP 组播循环、生命周期。                        |
| `internal/collector/`           | ✅      | —        | OS 相关采集器（basic、jetson、network），仅 Linux build tag。 |
| `internal/protocol/`            | ✅      | ✅       | 线协议（`DeviceInfo`）与 UDP 包结构。共享、无副作用。        |
| `internal/registry/`            | —       | ✅       | 客户端本地 JSON 设备注册表，落在 OS 用户配置目录。            |
| `internal/scanner/`             | —       | ✅       | 三源融合：UDP 组播、注册表轮询、手动子网扫描。                |
| `main.go` + `frontend/`         | —       | ✅       | Wails 入口 + React/TypeScript UI。                           |
| `scripts/`                      | 共用     | 共用      | 安装 / 卸载 / 部署 / 跨构建辅助脚本。                        |
| `docs/superpowers/specs/`       | —       | —        | 设计 spec（实施前），不参与构建。                            |
| `docs/superpowers/plans/`       | —       | —        | 实施计划（实施前），不参与构建。                              |

## 发现流程（简版）

1. **UDP 组播** —— 每个 agent 每 60 s 在 `239.255.42.42:9999` 广播一个
   `HELLO` 包。同 L2 广播域的 client 第一次见到某个 `device_id` 时即会
   把它纳入。
2. **注册表轮询** —— 对 client 已知的每一台设备，每 30 s 通过
   `GET http://<ip>:9999/api/v1/info` 拉取一次，`LastInfo` 驱动 UI 卡片。
3. **手动子网扫描** —— 在 GUI 「扫描子网」按钮里启动。scanner 自动检测
   本机 IPv4 子网（RFC1918 优先），逐 IP 探活 TCP `9999`，命中后再去
   `GET /healthz` 与 `/api/v1/info`。

三路结果都写回同一个 `internal/registry.Registry`，以 `LastSeenAt` 后写
优先（last-writer-wins），并把变更以 Wails 事件抛给前端。

## 配置来源

| 用途           | 位置                                                | 归属方    |
| -------------- | --------------------------------------------------- | --------- |
| agent 监听地址 | `/etc/spotterd/agent.toml`                          | 设备端    |
| 组播组地址     | `/etc/spotterd/agent.toml`                          | 设备端    |
| 设备 ID        | `/etc/spotterd/agent.toml`（UUID v4，一次性）        | install.sh |
| client 注册表  | `<UserConfig>/Spotter/devices.json`                 | 客户端    |
| client 日志    | `<UserConfig>/Spotter/logs/spotter.log`             | 客户端    |
| Wails 选项     | `wails.json`                                        | 客户端构建 |

## 为什么要拆成两个二进制

- agent 必须在**无图形栈的 headless Linux** 上运行，所以不能依赖 Wails
  / WebView。把 `spotterd` 做成单一 Go 二进制，意味着它的发布矩阵极小
  （两个架构），可以用 `scp + bash install.sh` 一行命令完成部署。
- client 是**桌面 UX**，Wails 让我们用同一份源码产出 macOS / Windows /
  Linux 各自的原生 webview 二进制。如果合并成一个，要么加 CLI 模式分支，
  要么把一个 agent 用不到的 webview 打包进去。

如果只想要命令行版的发现能力，直接给 `spotterd` 发请求：
`curl http://<device>:9999/api/v1/info` 就是它的协议。

## 为什么不容器化

Spotter 的发布物是两个静态二进制。agent 没有任何运行时依赖（不依赖
systemd socket activation、不依赖 FUSE、不依赖 libssl）；client 只依赖
系统自带的 webview。容器镜像会强迫运维为一个本质只是「两个 Go 二进制」
的东西维护一份基础镜像。

release workflow 已经把两个架构的产物都打好，落到 `dist/`。需要 deb /
rpm / nix / Homebrew tap 的话，自己包就行。
