# 更新日志

Spotter 的所有值得注意的变更都在此记录。格式参考
[Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵
循 [Semantic Versioning](https://semver.org/lang/zh-CN/spec/v2.0.0.html)。

英文版本：[`CHANGELOG.en.md`](CHANGELOG.en.md)。

## [Unreleased]

暂无变更。

## [0.1.0] — 2026-08-22

Spotter 第一个公开发布版本。两个二进制都从此版本发布。agent 支持
Linux `arm64` 与 `amd64`；client 支持 Windows、macOS 与 Linux。

### 新增

#### Agent (`spotterd`)
- systemd 服务单元（`scripts/spotterd.service`）与一次性安装脚本
  （`scripts/install.sh`），后者会生成 `device_id` UUID，写入
  `/etc/spotterd/agent.toml`。
- UDP 组播信标（`239.255.42.42:9999`，60 秒一次），让桌面 client 能
  在同一个 L2 广播域内发现设备。
- HTTP 端点 `GET /healthz`（存活）与 `GET /api/v1/info`（JSON 设备
  信息快照）。
- 基础 Linux 主机信息、Jetson 专属遥测（tegra SOC、jetson_clocks、
  nvpmodel）、网络接口描述三类采集器。
- `scripts/deploy.sh`（macOS / Linux）与 `scripts/deploy.ps1`
 （Windows）把匹配架构的二进制 scp + ssh 推到目标设备。两者都支持
  SSH 公钥与 `sshpass` / PuTTY 密码两种模式。
- `scripts/uninstall.sh` 与 `scripts/cleanup.sh` 用于优雅卸载与尽力彻
  底清理。

#### Client (`spotter-client`)
- Wails 桌面 GUI，跨 Windows、macOS、Linux。
- 三路发现 pipeline：注册表轮询（每 30 s 拉 `/api/v1/info`）、UDP
  组播加入、手动子网扫描（TCP 探活 + `/healthz` + `/api/v1/info`）。
- 单设备卡片（基础、网络、Jetson）、详情面板、侧边栏列表，含
  online / offline 状态与「last source」归属。
- i18n 字串（en + zh-CN）—— 所有 UI 字串都过
  `frontend/src/i18n/dictionaries.ts`。
- 安装向导模态，引导新运维人员过一遍 `make agent` 加上一个 deploy
  脚本。

#### 构建 / CI
- Makefile 作为构建入口的唯一权威。
- `scripts/build-all.sh` 交叉编译两个 agent 架构，编译当前平台的
  client，把一切打到 `dist/`，生成 `SHA256SUMS`。
- 跨 `internal/agentd`、`internal/collector`、`internal/protocol`、
  `internal/registry`、`internal/scanner` 的 race detector 单元测试。

### 备注
- 协议载荷携带 `schema_version: 1`。将来破坏性变更会递增它，并发布
  版本化的端点（`/api/v2/info`）。
- 已知限制列在 README「Known limitations (MVP)」：不提供远端命令执
  行、UDP 组播只在 L2、无认证。

[Unreleased]: https://github.com/letmlook/spotter/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/letmlook/spotter/releases/tag/v0.1.0
