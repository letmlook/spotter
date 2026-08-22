# Spotter

[中文版本](README.md) · [English](README.en.md)

局域网设备发现工具，面向 Linux 设备（ARM64 单板机，例如 Jetson，以及 AMD64 服务器与工作站）。

- **设备端**：单一 Go 二进制 `spotterd`（systemd 单元），源码位于 `cmd/agent/`，可构建 **arm64 与 amd64** 两种架构。
- **客户端**：`spotter-client`，Wails 桌面 GUI，原生支持 **Windows、macOS 与 Linux**。入口位于项目根目录，嵌入 `frontend/`（Vite + React + TS）构建的前端。

客户端通过三条管道发现设备，最终统一汇入合并流水线：

1. **注册表轮询**（HTTP GET `/api/v1/info`，每 30 秒）
2. **UDP 组播**（`239.255.42.42:9999`，每 60 秒）
3. **手动子网扫描**（TCP 探测 + `/healthz` + `/api/v1/info`）

## 平台支持

| 组件              | Linux arm64 | Linux amd64 | Windows | macOS |
|-------------------|:-----------:|:-----------:|:-------:|:-----:|
| `spotterd`        | ✓           | ✓           | —       | —     |
| `spotter-client`  | ✓           | ✓           | ✓       | ✓     |

`spotterd` 仅支持 Linux，因为它依赖 `systemd` 管理服务；`spotter-client` 是标准 Wails 应用，完全遵循 Wails 自身的多平台矩阵。

## 构建

`Makefile` 是构建目标的唯一权威入口。

```bash
# 单元测试（启用竞态检测，覆盖全模块）
make test

# 一次性构建两个支持架构的设备端二进制
make agent-all

# 单架构构建（便于分阶段发布）
make agent-linux-arm64
make agent-linux-x64

# 跨平台桌面客户端（Windows / macOS / Linux）
# Wails 根据当前 GOOS 选择目标平台
make client
```

`make client` 优先调用 `wails` CLI；若不可用则回退到 `go build`（注意：macOS 上的回退路径不会产出 `.app` 包，详见下文各平台说明）。

### 在 macOS 上构建客户端

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
make client          # 产物：build/bin/Spotter.app
open build/bin/Spotter.app
```

### 在 Windows 上构建客户端（PowerShell）

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend; npm install; cd ..
make client
# 产物：build\bin\spotter-client.exe
```

### 在 Linux 上构建客户端

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
make client
# 产物：build/bin/spotter-client
```

Wails 会自动拉取前端依赖并完成打包。如需独立开发 UI，可单独构建 Vite 项目：

```bash
cd frontend && npm install && npm run build
```

构建产物输出到 `bin/`：

| 产物                                | 源码             | 目标平台         |
|-------------------------------------|------------------|------------------|
| `bin/spotterd-linux-arm64`          | `cmd/agent/`     | Linux ARM64      |
| `bin/spotterd-linux-x64`            | `cmd/agent/`     | Linux AMD64      |
| `bin/spotterd`                      | `cmd/agent/`     | 当前 GOOS/GOARCH |
| `bin/spotter-client` / `.exe`       | 根目录 `main.go` | 当前 GOOS        |
| `build/bin/Spotter.app`             | `wails build`    | macOS 应用包     |

## 全平台打包

`scripts/build-all.sh` 提供一键构建并打包脚本，适合发布前在单一主机上产出完整 release 制品：

```bash
./scripts/build-all.sh              # 完整构建（agents + 当前平台客户端）
./scripts/build-all.sh --agent-only # 仅构建设备端二进制
make release                        # 等价入口
```

行为说明：

- **设备端**：始终为 Linux `arm64` 与 `amd64` 交叉编译（Go 交叉编译在所有主机通用）。
- **客户端**：只为当前主机构建 —— macOS 上产出 `Spotter.app`，Windows 上产出 `.exe`，Linux 上产出二进制。
- **打包**：所有产物整齐归入 `dist/`，并自动生成 `SHA256SUMS`。

发布全平台客户端时，只需在每台目标机器上各跑一次脚本，再把各自的 `dist/clients/` 合并即可。

## 手动安装 spotterd 到设备

GUI 客户端只负责发现 / 展示信息；`spotterd` 需要在每台目标设备上手动安装。随发布包分发的 `scripts/install.sh` 是设备端安装脚本；`scripts/deploy.sh` / `scripts/deploy.ps1` 是在开发者机器上一键推送 + 执行的便捷封装。

### 方式一：使用一键部署脚本（推荐）

脚本支持两种认证模式 —— **SSH 公钥**（推荐，公钥已配置时无需密码）或 **密码**（需要 sshpass / PuTTY）。

**macOS / Linux**（开发者机器）：

```bash
# 一次性安装 sshpass（仅密码模式需要；公钥模式不需要）
brew install hudochenkov/sshpass/sshpass   # Debian/Ubuntu: apt install sshpass；Fedora: dnf install sshpass

make agent-linux-arm64                    # 或 agent-linux-x64

# 公钥模式（推荐）：公钥已在 ssh-agent / ~/.ssh 中，目标机接受密钥认证
./scripts/deploy.sh nvidia 10.0.5.23
./scripts/deploy.sh nvidia 10.0.5.23 amd64

# 密码模式：未配公钥时使用，密码经 sshpass 传递
./scripts/deploy.sh nvidia <password> 10.0.5.23
./scripts/deploy.sh nvidia <password> 10.0.5.23 amd64
```

**Windows**（PowerShell）：

```powershell
# 一次性安装 PuTTY（密码模式需要；公钥模式可只装 Pageant）
# https://putty.org  或  choco install putty

make agent-linux-arm64

# 公钥模式（推荐）：Pageant / ssh-agent 中已有密钥时省略 -Password
.\scripts\deploy.ps1 -User nvidia -Ip 10.0.5.23
.\scripts\deploy.ps1 -User nvidia -Ip 10.0.5.23 -Arch amd64

# 密码模式：未配公钥时使用
.\scripts\deploy.ps1 -User nvidia -Password <password> -Ip 10.0.5.23
.\scripts\deploy.ps1 -User nvidia -Password <password> -Ip 10.0.5.23 -Arch amd64
```

脚本会自动：scp `spotterd` + `spotterd.service` + `install.sh` → `ssh` 跑 `sudo bash /tmp/install.sh` → `curl /healthz` 验证。

> 注意：SSH 用户必须能在目标上免密 `sudo`，否则 `install.sh` 中的 `sudo bash` 会卡在交互式密码提示上。这种情况下用方式二手动跑每一步。

### 方式二：手动 scp + ssh

```bash
# 从开发机把对应架构的二进制、systemd 单元、安装脚本推到目标设备
scp bin/spotterd-linux-<arch>  user@<device>:/tmp/spotterd
scp scripts/spotterd.service   user@<device>:/tmp/spotterd.service
scp scripts/install.sh         user@<device>:/tmp/install.sh

# 在目标设备上执行
ssh user@<device> sudo bash /tmp/install.sh
```

`install.sh` 会：

1. 把 `spotterd` 安装到 `/usr/local/bin/`。
2. 生成 `device_id`（UUID v4）并写入 `/etc/spotterd/agent.toml`。
3. 安装并启用 systemd 单元。

卸载使用 `scripts/uninstall.sh`；彻底清理使用 `scripts/cleanup.sh`。

GUI 客户端通过 UDP 组播（或手动子网扫描 / 按 IP 添加）发现该设备后会自动开始轮询，无需在 GUI 上做任何"注册"操作。

## 已知限制（MVP）

- 设备端仅支持运行 **systemd** 的 Linux（Ubuntu / Jetson / Debian / RHEL；`arm64` 与 `amd64` 均可）。
- **不支持任意远端命令执行** —— 仅提供 opt-in 的远程 reboot / shutdown（见 power-actions 设计）与设备端软件执行日志查看（见 execution-log-stream 设计）。不提供 shell 或自定义命令通道。
- UDP 组播仅限 **L2**（同 VLAN），除非路由器主动转发。
- HTTP 端点 **无身份认证** —— 仍仅限可信局域网内部署；启用电源管理等于授予同网段任何客户端触发 root 级别 reboot / poweroff 的权限；启用日志流等于授予读取该 unit 在 systemd journal 中历史与新行的权限。

## 架构设计

完整的设计说明见 [`docs/superpowers/specs/2026-08-21-spotter-design.md`](docs/superpowers/specs/2026-08-21-spotter-design.md)；组件与包结构的精炼总结见 [`docs/architecture.md`](docs/architecture.md)。

## 文档导航

中文版是默认文档，英文版以 `.en.md` 后缀提供。

| 文档                                              | 适用读者     | 内容                                                         |
|--------------------------------------------------|------------|--------------------------------------------------------------|
| [README.md](README.md) / [README.en.md](README.en.md) | 所有人    | 项目入口、构建矩阵、部署脚本。                              |
| [docs/architecture.md](docs/architecture.md) / [.en](docs/architecture.en.md) | 开发者     | 组件与包结构、为什么拆成两个二进制、配置来源一览。          |
| [docs/operations.md](docs/operations.md) / [.en](docs/operations.en.md) | 运维       | 设备与客户端的文件布局、配置项、日常任务、升级流程。         |
| [docs/troubleshooting.md](docs/troubleshooting.md) / [.en](docs/troubleshooting.en.md) | 排障       | 按症状划分的 10 条常见故障与对应处置。                       |
| [docs/faq.md](docs/faq.md) / [.en](docs/faq.en.md) | 所有人     | 许可证选择、网络要求、构建产物等常见问答。                   |
| [docs/api.md](docs/api.md) / [.en](docs/api.en.md) | 集成者     | `/api/v1/info`、`/healthz` 与 UDP 多播包的字段级规范。       |
| [CONTRIBUTING.md](CONTRIBUTING.md) / [.en](CONTRIBUTING.en.md) | 贡献者 | 开发环境、提交规范、新增 collector / 组件的步骤。            |
| [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) / [.en](CODE_OF_CONDUCT.en.md) | 贡献者 | 社区行为准则（Contributor Covenant v2.1）。                   |
| [SECURITY.md](SECURITY.md) / [.en](SECURITY.en.md) | 报告者     | 漏洞报告渠道、版本支持矩阵、加固 checklist。                |
| [CHANGELOG.md](CHANGELOG.md) / [.en](CHANGELOG.en.md) | 所有人   | 版本变更历史（Keep a Changelog 格式）。                     |
| [LICENSE](LICENSE)                                | 所有人     | MIT License（Copyright © 2026 Spotter Dev）。                |

## 项目元数据

- `.github/ISSUE_TEMPLATE/`：Bug 报告、功能请求、设备平台支持、提问、安全报告五种模板。
- `.github/PULL_REQUEST_TEMPLATE.md`：提交 PR 时的自检清单。
- `.github/workflows/`：`go.yml`（Go 测试 + lint）、`frontend.yml`（前端 build + typecheck）、`agent-build.yml`（设备端 arm64 / amd64 交叉编译）、`release.yml`（tag 触发的全量构建与 GitHub Release 发布）。
- `.github/dependabot.yml`：Go / npm / GitHub Actions 依赖每周自动升级策略。
- `.golangci.yml`：golangci-lint v1.59+ 的启用规则集合。