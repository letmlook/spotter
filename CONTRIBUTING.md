# 参与 Spotter 贡献

感谢你对改进 Spotter 的兴趣。Spotter 有两个面：设备端 agent
（`spotterd`，Go）和跨平台桌面 client（`spotter-client`，Wails +
React）。它们通过 protocol 包共享，但各自有略微不同的迭代节奏。

英文版本：[`CONTRIBUTING.en.md`](CONTRIBUTING.en.md)。

## 基本规则

- 所有互动受我们的 [Code of Conduct](CODE_OF_CONDUCT.md) 约束。
- 一旦提交贡献，就视为同意以本项目的 [MIT 许可证](LICENSE) 授权。
- 非平凡的改动先开 issue 讨论再开 PR。
- 一个 PR 解决一件事或一个特性。合并前整理掉噪音 commit。

## 开发环境

| 工具         | 最低版本 | 用途                                          |
| ------------ | -------- | --------------------------------------------- |
| Go           | 1.25     | `go.mod` 声明 `go 1.25.0`。                   |
| Node.js      | 20 LTS   | Wails / Vite 需要。                           |
| npm          | 10+      | 前端依赖管理。                                |
| Wails CLI    | v2.15+   | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |

构建入口只有 Makefile 这一份。

```bash
git clone https://github.com/spotter/spotter.git
cd spotter

# 跑带 race detector 的完整 Go 测试。
make test

# 编译当前平台的 agent（开发期 `go run` 友好）。
make agent

# 编译当前 GOOS 的桌面 client。
make client
```

要做 UI 迭代不需要重出二进制，跑 Vite dev server，让 Wails serve 它
（`wails dev`）。

## 编码约定

- **Go**：标准 `gofmt`、`go vet` 干净、`race detector` 干净。push 之前
  跑 `golangci-lint run ./...`——见 `.golangci.yml`。
- **Go 测试**：与源代码同包（`foo_test.go`），能写表驱动就写表驱动。
 协议包级别尽量用真实实现而非 mock。**不要**引入依赖时序的断言——复用
 `internal/scanner` 里现成的注入式 `time.Sleep` / backoff helper。
- **TypeScript / React**：strict 模式已经在
  `frontend/tsconfig.json` 打开。组件放 `frontend/src/components/`，
 共享状态放 `frontend/src/state/`，i18n 字串写在
 `frontend/src/i18n/dictionaries.ts`——**不要**就地写用户可见字串。
- **Shell / PowerShell 脚本**：bash 用 `set -euo pipefail`；PowerShell
 用 `$ErrorActionPreference = 'Stop'`；风格跟 `scripts/deploy.sh` /
 `scripts/deploy.ps1` 对齐。

## 目录约定

```
cmd/agent/                 spotterd 入口（仅 Linux）
internal/agentd/           HTTP + UDP 循环
internal/collector/        OS 相关采集器（basic / jetson / network）
internal/protocol/         agent 与 client 共享的协议格式
internal/registry/         客户端本地注册表（磁盘 JSON）
internal/scanner/          三源融合发现（mcast + poll + subnet）
main.go + frontend/        spotter-client（Wails）
scripts/                   install / uninstall / deploy / build-all
docs/                      终端用户文档
docs/superpowers/          内部设计 spec 与计划
```

## 新增采集器

采集器放 `internal/collector/` 下。步骤：

1. 新建 `<name>_<os>.go`，写 `Collect(context.Context) (X, error)` 函数。
2. 在 `internal/collector/collector.go` 的 `Collect(...)` 中通过 build
   tag 或 runtime 守卫注册它，保留交叉编译精简。
3. 如果新载荷要新加字段，扩 `internal/protocol/info.go`——同时升
   `schemaVersion`（`internal/protocol/schema_version.go`），同步更新
   `cmd/agent` 里的滚动解码逻辑。
4. 在 `internal/collector/<name>_<os>_test.go` 里加上单元测试，跑在
   `t.TempDir()` 的 fixture 上。模板看 `basic_linux_test.go`。

## 新增 UI 组件

1. 在 `frontend/src/components/MyThing.tsx` 同目录建配套
   `.module.css`，全局 `.css` 只放真正的全局规则。
2. 复用 `DeviceContext` 拿注册表；`useWailsEvents` 拿实况事件；
   `useDeviceActions` 走后端调用。
3. 所有用户可见字串写到 `frontend/src/i18n/dictionaries.ts`，中英两
   份。dictionary schema 看现有条目。

## 提交信息

遵循 Conventional Commits：

```
feat(agent): 在 /api/v1/info 上加 DHCP 探测到的子网信息
fix(client): 处理空注册表，避免 About 对话框崩溃
docs(readme): 强调 spotterd 需要 systemd 240+
```

常用 scope：`agent`、`client`、`frontend`、`protocol`、`scanner`、`docs`、
`ci`、`scripts`、`build`。工具类改动用 `chore`。

## 提交 PR

1. Fork 仓库，从 `master` 创建 feature 分支。
2. 本地跑 `make test` 与对应 lint，要全绿。
3. 如果是用户可见的改动，把 `CHANGELOG.md` 的「Unreleased」段更新一
   下。
4. 用 [PR 模板](.github/PULL_REQUEST_TEMPLATE.md) 开 PR，关联相关
   issue（`Closes #123`）。
5. 等待 CI 转绿；reviewer 提的修改建议，作为 follow-up commit 加到同一
   分支。

## 报告 bug / 提需求

用 `.github/ISSUE_TEMPLATE/` 里对应的模板。**安全漏洞走
[SECURITY.md](SECURITY.md)，不要开公开 issue。**
