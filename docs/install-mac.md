# 安装 — macOS（Homebrew）

通过 [官方 tap](https://github.com/spotter/homebrew-tap) 一行命令安装。

```bash
brew install spotter/tap/spotter
```

GUI 启动：`spotter`（位于 `$(brew --prefix)/bin`）。首次启动按提示授予「本地网络」权限（macOS 15+ 需要）。

升级：

```bash
brew update && brew upgrade spotter
```

源码安装（开发版）：

```bash
brew install --HEAD spotter/tap/spotter
```

## 设备端 (`spotterd`) 安装

`spotterd` 是 Linux only；macOS 上无法运行。同名进程互不干扰，但 host 上的 GUI 只能发现网络里的 Linux 设备，不会尝试本机。

要从 macOS 把 `spotterd` 推到远端 Linux 设备，用项目根的 `scripts/deploy.sh`（详见 [README](README.md#手动安装-spotterd-到设备)）。

## 故障排查

**`brew install` 卡在 `Cloning spotter/tap`**

Tap 仓库首次访问需要拉取，少数网络环境下慢。等 1–2 分钟；如 5 分钟仍卡，检查 `git ls-remote https://github.com/spotter/homebrew-tap` 是否能通。

**启动后看不到任何设备**

1. macOS「系统设置 → 隐私与安全性 → 本地网络」是否授予了 Spotter 权限。
2. 设备与 Mac 是否在同一 VLAN（UDP 组播 `239.255.42.42` 不跨路由器）。
3. 设备端 `systemctl status spotterd` 是否 active。

**Tap 公式 `sha256` 校验失败（罕见）**

通常发生在 release 刚发布后 GitHub CDN 还在 propagate。运行：

```bash
brew update && brew install --force spotter/tap/spotter
```

仍失败则在 [spotter/homebrew-tap](https://github.com/spotter/homebrew-tap) 提 issue。

## 维护流程

每次打 release tag 后，`.github/workflows/release.yml` 的 `homebrew-tap` job 会自动用 `peter-evans/create-pull-request` 跨仓库开 PR，patch `Formula/spotter.rb` 的 `url` 和 `sha256`。该 job 需要 `HOMEBREW_TAP_TOKEN` secret（PAT，`repo` scope，作用于 `spotter/homebrew-tap`）。

未配置 secret 时 job 静默跳过，operator 可手动跑：

```bash
GITHUB_REPO=spotter/spotter VERSION=vX.Y.Z ./scripts/update-homebrew-sha.sh
```

把输出替换到 `Formula/spotter.rb` 后手开 PR。
