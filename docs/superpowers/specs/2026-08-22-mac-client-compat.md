# Spotter GUI 客户端 Mac 兼容性

| 项 | 值 |
|---|---|
| 项目名 | **Spotter** |
| 文档日期 | 2026-08-22 |
| 状态 | 设计草案，待用户审核 |
| 范围 | 仅 `spotter-client` GUI 二进制的 Mac 构建与运行 |
| 后端逻辑 | 不变（`internal/**` 全部跨平台） |
| 前端 UI | 不变（保留自定义 TitleBar，不做 Mac 原生外观适配） |

---

## 1. 目标与范围

### 1.1 目标

让 `spotter-client` GUI 客户端**可在 macOS 上构建并启动运行**：
- 在 Mac 开发机上 `make client` 产出可启动的 .app bundle（或裸 Mach-O fallback）
- GUI 行为与 Windows 版保持一致（自定义 TitleBar 不变）
- 不引入新功能、不做 UI 风格改造

### 1.2 非目标（明确排除）

- 代码签名（`codesign`）与公证（`notarize`）— 需要 Apple Developer ID
- DMG 打包
- macOS 原生菜单栏（保留 Wails 默认）
- Cmd 快捷键适配（保留 Ctrl 触发 Wails 默认行为）
- Dock 图标自定义
- Linux GUI 构建产物的端到端验证（仅配置 `linux.Options` 防编译失败）

### 1.3 验收标准

| # | 标准 |
|---|------|
| A | `make test` 在 Mac 上全部通过 |
| B | `make client`（已安装 wails CLI）产出 `build/bin/Spotter.app` 且可启动 |
| C | `make client`（无 wails CLI）走 `go build` fallback，输出 warning，产出裸 `bin/spotter-client` |
| D | GUI 启动后窗口显示，无 macOS 原生标题栏与自定义 TitleBar 叠加 |
| E | 前端按钮（Deploy / Scan / Add / Clear registry）可点击，无 panic |

---

## 2. 现状与障碍

### 2.1 现状

`main.go` 当前只配置 Windows 平台选项：

```go
Windows: &windows.Options{
    WebviewIsTransparent: false,
    DisableWindowIcon:    true,
},
```

`windows` 子包是纯 Go 结构体，可跨平台编译；但**不配置 Mac/Linux 选项时，Wails 会用各自平台的默认设置**——Mac 默认保留系统标题栏，会与自定义 TitleBar 叠加。

### 2.2 Wails v2 跨平台选项机制

Wails v2 的 `options.App` 提供三个独立字段：

```go
Windows *windows.Options  // Windows 专用
Mac     *mac.Options      // macOS 专用
Linux   *linux.Options    // Linux 专用
```

三者都是 `wails/v2/pkg/options/<platform>` 子包下的纯 Go 结构体，无 build tag，跨平台 import 即可编译。运行时 Wails 按 `runtime.GOOS` 选用对应字段。

### 2.3 Makefile 现状

```makefile
client:
    $(GO) build $(GOFLAGS) -o bin/spotter-client .
```

- Windows：产物为 `bin/spotter-client.exe` ✓
- macOS：产物为裸 Mach-O 二进制 `bin/spotter-client`，**不是** .app bundle
- .app bundle 是 macOS 推荐分发/启动形式（含 `Contents/MacOS/<exe>`、`Info.plist`、Dock 图标等）

---

## 3. 设计方案

### 3.1 总体策略

采用**最小改动方案**：
1. 在 `main.go` 中添加 Mac/Linux 平台选项字段（保留现有 Windows 配置）
2. 改造 `Makefile` 的 `client` target：检测 `wails` CLI，存在则用 `wails build`（产出 .app），否则回退 `go build`
3. README 补充 Mac 构建说明

无新文件，无 build tag 拆分，无 frontend 改动。

### 3.2 文件改动清单

| 文件 | 类型 | 改动摘要 |
|------|------|---------|
| `main.go` | 修改 | 新增 2 个 import；`wails.Run` 入参添加 `Mac`、`Linux` 字段 |
| `Makefile` | 修改 | `client` target 增加 wails CLI 检测与 fallback |
| `README.md` | 修改 | "Build" 段补充 macOS 构建说明 |
| `docs/superpowers/specs/2026-08-22-mac-client-compat.md` | 新增 | 本设计文档 |

### 3.3 不改动的文件

- `internal/**` — SSH/UDP/HTTP/扫描/采集全部跨平台，零改动
- `cmd/agent/main.go` — 已有 `//go:build linux`，Linux-only，不影响
- `frontend/**` — UI 完全跨平台，零改动
- `wails.json` — 无需平台字段

---

## 4. `main.go` 详细改动

### 4.1 imports 新增

```go
import (
    ...
    "github.com/wailsapp/wails/v2/pkg/options/windows"
    "github.com/wailsapp/wails/v2/pkg/options/mac"      // 新增
    "github.com/wailsapp/wails/v2/pkg/options/linux"    // 新增
    ...
)
```

### 4.2 `wails.Run` 入参

```go
opts := &options.App{
    Title:  "Spotter",
    Width:  1200,
    Height: 800,
    AssetServer: &assetserver.Options{Assets: uiFS},
    OnStartup:   app.OnStartup,
    Frameless:   true,
    Bind:        []interface{}{app},

    Windows: &windows.Options{
        WebviewIsTransparent: false,
        DisableWindowIcon:    true,
    },
    Mac: &mac.Options{
        TitleBar: &mac.TitleBar{
            TitlebarAppearsTransparent: true,
            HideTitle:                  true,
            HideTitleBar:               false,
            FullSizeContent:            false,
        },
        Appearance:           mac.NSAppearanceNameDarkAqua,
        WebviewIsTransparent: false,
    },
    Linux: &linux.Options{
        ProgramName: "Spotter",
    },
}
err = wails.Run(opts)
```

### 4.3 字段逐项说明

| 字段 | 作用 |
|------|------|
| `Mac.TitleBar.HideTitle: true` | 隐藏窗口顶部 macOS 原生标题文字，避免与自定义 TitleBar 叠加 |
| `Mac.TitleBar.TitlebarAppearsTransparent: true` | 原生标题栏背景透明，避免出现一条灰色横条 |
| `Mac.TitleBar.HideTitleBar: false` | 故意保持 false——保留原生标题栏区域让 Wails 处理 inset |
| `Mac.TitleBar.FullSizeContent: false` | 不让 webview 内容延伸到标题栏区域 |
| `Mac.Appearance: DarkAqua` | 强制深色模式，与自定义深色 UI 主题一致 |
| `Mac.WebviewIsTransparent: false` | 与 Windows 保持一致 |
| `Linux.ProgramName: "Spotter"` | 设置 WM 下的应用名，默认显示 `wails` |

### 4.4 风险与兼容

- Wails 版本需 ≥ v2.5（项目当前 v2.15，满足）
- `mac.Options`/`linux.Options` 字段集在 v2.15 中稳定
- 所有 options 子包都是纯 Go，跨平台 import 无障碍

---

## 5. `Makefile` 详细改动

### 5.1 新写法

````makefile
WAILS := $(shell command -v wails 2>/dev/null)

client:
ifneq ($(WAILS),)
	$(WAILS) build
else
	@echo "warning: wails CLI not found; falling back to 'go build' (will NOT produce a .app bundle on macOS)" >&2
	$(GO) build $(GOFLAGS) -o bin/spotter-client .
endif
````

> 注意：使用 `ifneq ($(WAILS),)` 而不是 `ifdef WAILS`——后者会把空字符串视为"已定义"，导致执行 ` build` 报错。

### 5.2 行为矩阵

| 环境 | 命令 | 产物 | 启动方式 |
|------|------|------|---------|
| macOS + wails | `make client` | `build/bin/Spotter.app` | `open build/bin/Spotter.app` |
| macOS 无 wails | `make client` | `bin/spotter-client`（裸 Mach-O） | `./bin/spotter-client &` |
| Windows + wails | `make client` | `build/bin/spotter-client.exe` | 双击 |
| Windows 无 wails | `make client` | `bin/spotter-client.exe` | 双击或命令行 |

### 5.3 对其他 target 的影响

- `test`、`agent`、`agent-linux-arm64`、`build`、`clean`：零影响
- `build: agent client`：仍工作

---

## 6. README 补充

在现有 "Build" 段末尾追加：

````markdown
### Build on macOS

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
make client          # produces build/bin/Spotter.app
open build/bin/Spotter.app
```

If `wails` is not on PATH, `make client` falls back to `go build` and
produces a bare `bin/spotter-client` Mach-O binary (no .app bundle).
The .app bundle is the recommended way to launch on macOS.
````

---

## 7. 验证步骤

### 7.1 一次性环境准备

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd frontend && npm install && cd ..
```

### 7.2 构建与启动

```bash
# 1. 单元测试（验证 Go 代码不回归）
make test

# 2. 静态检查
go vet ./...

# 3. 构建 .app
make client

# 4. 验证产物存在
ls build/bin/Spotter.app/Contents/MacOS/

# 5. 启动
open build/bin/Spotter.app

# 或带日志启动（便于排查）
./build/bin/Spotter.app/Contents/MacOS/Spotter
```

### 7.3 验收清单

| # | 检查项 | 期望 | 验证方式 |
|---|--------|------|---------|
| 1 | `make test` | 全部通过 | 退出码 0 |
| 2 | `go vet ./...` | 干净 | 退出码 0 |
| 3 | `make client` | `build/bin/Spotter.app/Contents/MacOS/Spotter` 存在 | `ls` |
| 4 | GUI 启动 | 窗口出现，自定义 TitleBar 单一显示 | 肉眼 |
| 5 | 前端交互 | Deploy/Scan/Add/Clear 按钮可点击 | 手动点击 |
| 6 | Fallback | 临时 PATH 去掉 wails 后 `make client` 走 fallback 并输出 warning | 手动验证 |
| 7 | 进程清理 | 关闭窗口后进程退出 | `pgrep -f Spotter` |

---

## 8. 测试策略

### 8.1 单元测试

**不新增**。理由：
- `main.go` 改动仅是 Wails 配置项，无业务逻辑
- 跨平台编译性由 §7.3 第 3 条覆盖
- `make test` 已能确保 `internal/**` 不回归

### 8.2 集成验证

由 §7.3 验收清单覆盖，本任务在 Mac 上完成端到端构建 + 启动验证。

### 8.3 不引入 CI

本任务无 CI 改动。后续若需要自动化 Mac 构建验证，可单独建 spec。

---

## 9. 风险与回滚

### 9.1 风险

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| Wails 字段名误用导致编译失败 | 低 | 中 | `wails build` 立即暴露 |
| `HideTitle: true` 在不同 macOS 版本表现不一 | 低 | 低 | macOS 10.13+ API 稳定 |
| Linux GUI 端到端未验证 | 中 | 低 | 仅加 `linux.Options` 防编译失败；端到端验证留给后续 |
| wails CLI 安装失败 | 低 | 低 | fallback 走 `go build` 仍能产出可执行文件 |

### 9.2 回滚

如需回滚，三个改动都是独立 commit：
1. revert `main.go` import 与 options 改动
2. revert `Makefile` 的 wails 检测块
3. revert `README.md` 新增段

---

## 10. 不在范围（明确）

- ❌ 代码签名与公证
- ❌ DMG 打包
- ❌ 原生菜单栏
- ❌ Cmd 快捷键适配
- ❌ Dock 图标自定义
- ❌ Linux GUI 端到端验证
- ❌ 新增单元测试

---

## 11. 后续工作（follow-up，本任务不做）

- 拆分 `internal/scanner` 接收 ctx 的方式（已有 FIXME 注释提到 ssh.Dial 不接受 context）
- 引入 GitHub Actions macOS runner 跑 `wails build` 做 CI
- 给 Mac .app 加 Apple Developer ID 签名（需账户）
- 适配 Cmd 快捷键 + macOS 菜单栏
