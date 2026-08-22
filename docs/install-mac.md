# 安装 — macOS（Homebrew）

通过 [官方 tap](https://github.com/spotter/homebrew-tap) 一行命令安装。

```bash
brew install spotter/tap/spotter
```

GUI 启动：`spotter`（位于 `$(brew --prefix)/bin`）。

升级：

```bash
brew update && brew upgrade spotter
```

源码安装（开发版）：

```bash
brew install --HEAD spotter/tap/spotter
```

设备端 (`spotterd`) 是 Linux only；macOS 上无法运行。同名进程互不干扰。
