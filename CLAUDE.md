# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概览

局域网设备发现工具，由**两个独立二进制**组成，共享 `internal/` 下的库代码：

- **`spotterd`**（`cmd/agent/`，Linux only）— 设备端守护进程，作为 systemd 单元运行；提供 `GET /api/v1/info`、`GET /healthz`，并以 5s 周期在 `239.255.42.42:9999` 主动发送 UDP HELLO（`internal/agentd/udp.go` 的 `helloInterval`，可由 `Config.HelloInterval` 覆盖）。仅构建 Linux `arm64` + `amd64`。
- **`spotter-client`**（根目录 `main.go`）— Wails 桌面 GUI，源码在 `frontend/`（Vite + React + TS）。原生支持 macOS / Windows / Linux 三端，前端通过 `//go:embed frontend/dist` 嵌入。

为什么拆两个：agent 不能依赖任何 webview / GUI 栈，必须是 headless 静态二进制；client 依赖系统 webview，矩阵由 Wails 处理。合并会让两端都背不必要的依赖。

模块路径：`github.com/spotter/spotter`（Go 1.25）。

## 客户端发现流水线（核心架构）

client 通过三条独立通道发现设备，结果统一写入 `internal/registry.Registry`，**last-writer-wins**（按 `LastSeenAt`），变更以 Wails event 推前端：

| 通道 | 实现 | 周期 | 触发器 |
|------|------|------|--------|
| 注册表轮询 | `internal/scanner/poll.go` | 30s | 对每个已知 device GET `/api/v1/info` |
| UDP 组播 | `internal/scanner/mcast.go` | 60s | 向 `239.255.42.42:9999` 发 HELLO 并在同一 socket 上收 HELLO-REPLY；agent 侧另有 5s 的主动 HELLO 推送 |
| 手动子网扫描 | `internal/scanner/subnet.go` | 用户触发 | 自动探测本机 RFC1918 子网，逐 IP TCP 探活 + `/healthz` + `/api/v1/info` |

> 周期常量的**唯一来源**是 `internal/protocol/defaults.go`（`DefaultPollInterval` = 30s、`DefaultMcastInterval` = 60s），由 `internal/clientconfig` 与 `scanner.Options.withDefaults` 共同引用，并被 `defaults_test.go` 锁定——改文档前先看那里，别反过来。client 的 60s 组播回路只是兜底：上线延迟实际由 agent 的 5s 主动 HELLO 决定。

融合逻辑集中在 `internal/scanner/merge.go` 的 `mergeInfo`——已注册则更新 `LastInfo / LastSeenAt / Online`，未注册则发出 `EventUnknownDeviceDiscovered`，**由 `main.go` 的 OnEvent 回调直接写入注册表（服务端 auto-accept）**。连续失败 3 次的设备在 `pollFailures` 中被标记 offline（`EventOffline`）。

> **设计决策：auto-accept（v0.5 起的现行行为）**。早期版本曾规划「GUI 可选择接受」；目前 Wails EventsOn hook 的 variadic-args 形状在不同 wails 版本间不稳定，且手动 accept 会显著拖累扫描回路的吞吐，故改为服务端静默添加并立即 emit `info-updated`，让 UI reducer 在一次往返内拿到新行。`AcceptUnknownDevice` 绑定仍保留以支持主动用 `ProbeByIP` 后的显式接受。

前端 ↔ Go 边界在 `main.go` 的 `App` 结构体上——所有方法通过 Wails `Bind` 暴露给 JS 侧，事件通过 `wailsruntime.EventsEmit` 推送。`OnStartup` 注入真实 ctx 后才启动 scanner loop；`App.ctx` 用 `context.Background()` 占位以保证 Wails 启动前的早期事件不 panic。

## 内部包结构

| 包 | 端 | 说明 |
| --- | :--: | --- |
| `cmd/agent/` | agent | `spotterd` 入口，`//go:build linux` |
| `internal/agentd/` | agent | HTTP server、UDP 广播循环、生命周期 |
| `internal/collector/` | agent | `basic_linux.go` / `jetson_linux.go` / `network_linux.go`，**全部带 build tag**，仅 Linux |
| `internal/protocol/` | 双端共享 | `DeviceInfo`（`/api/v1/info` 响应）+ UDP 包结构（`udp.go`）+ `schema_version.go` + wire 常量（`DefaultListenAddr` / `DefaultMulticastAddr` / `DefaultDevicePort`）|
| `internal/registry/` | client | 本地 JSON 设备注册表（`<UserConfig>/Spotter/devices.json`），线程安全 |
| `internal/scanner/` | client | 三源融合；`merge.go` 是唯一写入 registry 的入口 |
| `internal/lanscan/` | 双端共享 | `LocalSubnets` + `RFC1918Rank`；被 GUI（main.go）和 CLI（spotter-cli）共享 |
| `cmd/spotter-cli/` | client | 终端客户端：list / scan / show / log；复用 scanner + registry |
| `cmd/spotter-server/` | server | serverd 入口（HTTP + WebSocket hub），跨平台编译（macOS / Windows / Linux）|
| `internal/serverd/` | server | HTTP handler + WebSocket hub + JSON-on-disk store，与 client 的 `internal/registry` 同名但语义独立 |
| `internal/clientconfig/` | client | 用户可调设置（multicast_group / device_port / auth_token / enable_mdns 等）持久化 |
| `internal/mdns/` | 双端共享 | 设备 IP 漂移时通过 mDNS 重新发现地址 |

`protocol` 是**两端唯一共享的包**，修改前请确认字段向后兼容（schema_version）。

## 常用命令（Makefile）

```bash
# 测试（启用竞态检测，覆盖全模块）
make test
# 单包测试
go test ./internal/scanner/... -race -count=1 -run TestMerge
# 单个测试函数
go test ./internal/registry/ -run TestRegistry_Remove -count=1 -v

# 覆盖率（生成 coverage.out + 打印 total %）
make coverage

# SPEC ↔ code 不变式（脚本基于 SPEC_REVIEW.md）
make spec-check

# 设备端（agent）
make agent                # 当前 GOOS/GOARCH
make agent-linux-arm64    # 交叉编译
make agent-linux-x64
make agent-all            # 两个架构一次性构建

# 客户端（GUI）
make client               # 优先 wails CLI，缺失则回退到 go build（macOS 上不会产 .app 包）

# 清理
make clean                # 删除 bin/
```

发布：`make release` 调用 `scripts/build-all.sh`，产出落到 `dist/` 并自动生成 `SHA256SUMS`。

## Lint 与 CI

- `.golangci.yml` v2 配置，启用 bodyclose / errcheck / gofmt / goimports / gocritic / govet / ineffassign / misspell / nolintlint / prealloc / revive / staticcheck / unconvert / unused。
- goimports `local-prefixes: github.com/spotter/spotter`——新 import 自动归组。
- 测试文件豁免 `gosimple` 与 `gomnd`。
- CI（`.github/workflows/`）：`go.yml`（test + lint + coverage + spec-check）、`frontend.yml`（前端 build + typecheck）、`agent-build.yml`（交叉编译两个架构）、`client-cross-compile.yml`（macOS / Windows 客户端 + codesign/DMG 集成）、`release.yml`（tag 触发全量发布 + cosign 签名 + SLSA provenance）。dependabot 周升级。
- 当前 release workflow 中 golangci-lint 固定到 v2.5.0。
- **覆盖率门槛（v0.5）**：55%。`internal/*` 大部分 70%+（clientconfig 88、timefmt 100、jsonstore 86、registry 82、serverd 76），根包 22%（main.go Wails binding 不可单测）。每 PR 推高门槛。

## 当前状态（v0.5 末）

- **v0.5 已落地**（参考 `.claude/iteration-tracker.md` 完整列表）：时序 metrics、log stream 增强、电源审计 + cancel 真实实现、SQLite WAL 持久化、OpenRC/runit init、cosign+SLSA、跨平台 CI matrix、macOS codesign 框架、agent `/admin` Web、客户端告警引擎。
- **C-8 客户端自更新** 留 v1.1（需 Apple Developer 账号 + diff 更新 + 跨平台签名链）；v1.0 用"GUI 检测按钮 → 浏览器下载"fallback。
- **v0.5 性能 / 可观测性** 已配：电源操作 5s 内可发现、`/admin` HTML 单页 < 300 KB、时序数据 5s 采样 × 60 槽 = 5min 历史。
- **客户端 ID** 持久化为 UUID v4（`internal/clientconfig.Settings.ClientID`），跨重启稳定。

## 配置来源

| 用途 | 位置 | 归属 |
| --- | --- | --- |
| agent 监听/组播/device_id | `/etc/spotterd/agent.toml`（缺失文件非错，仅在字段为空时用默认） | 设备端 |
| client 注册表 | `<UserConfig>/Spotter/devices.json` | 客户端 |
| client 日志 | `<UserConfig>/Spotter/logs/spotter.log` | 客户端 |
| Wails 打包选项 | `wails.json` | 客户端构建 |

## 扩展点

- **新增采集器**（agent）：在 `internal/collector/` 加 `xxx_linux.go`（必须带 `_linux.go` 后缀以匹配 build tag），实现 `Collector` 接口，由 `collector.New()` 组合。参考 `jetson_linux.go`。
- **新增发现源**（client）：在 `internal/scanner/` 加新 loop 文件，由 `Scanner.Start` 启动 goroutine；新结果统一走 `merge.go` 写注册表 + 触发事件——**不要绕过 `mergeInfo` 直接写 registry**。
- **新增协议字段**：先改 `DeviceInfo` 再改 `schema_version.go`；保证向后兼容。

## 提交规范

遵循 `.cursor/rules/git-commit-author.mdc`：

- 作者固定 `letmlook <letmlook@aliyun.com>`——通过 `git commit --author=...` 指定，**不要修改 git config**。
- 中文 commit message，格式 `[类型] 简短描述`（如 `feat: 添加 jetson 内存采集`）。
- 仅在用户明确要求时提交；不要主动 commit。
- 不 force push master；不 skip hooks。

## 文档

- 设计 spec（实施前的完整设计文档）：`docs/superpowers/specs/2026-08-21-spotter-design.md`
- 运维 + 排障 + API + FAQ：`docs/` 下中英双语（默认中文，`.en.md` 为英文）
- 中文 README 是项目入口：`README.md`