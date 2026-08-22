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

GUI 客户端只负责发现 / 展示信息；`spotterd` 需要在每台目标设备上手动安装。随发布包分发的 `scripts/install.sh` 提供一键安装流程：

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
- **不支持远端命令执行** —— 仅提供静态信息面板。
- UDP 组播仅限 **L2**（同 VLAN），除非路由器主动转发。
- HTTP 端点 **无身份认证** —— 仅限可信局域网内部署。

## 架构设计

详见 [`docs/superpowers/specs/2026-08-21-spotter-design.md`](docs/superpowers/specs/2026-08-21-spotter-design.md)。