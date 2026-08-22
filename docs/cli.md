# CLI 客户端（headless）

适合 SSH-only 服务器或自动化场景。复用 scanner 库，不需要 Wails。

子命令：

| 命令 | 作用 |
|---|---|
| `spotter-cli list` | 列出本地注册表中的设备（online/offline） |
| `spotter-cli scan [--cidr=x.x.x.x/24]` | 扫描子网（默认自动挑选 RFC1918） |
| `spotter-cli info <device_id>` | 打印某设备的 /api/v1/info（cached） |
| `spotter-cli version` | 版本信息 |

数据路径与 GUI 客户端共享 `<UserConfigDir>/Spotter/devices.json`，因此 headless 扫描的成果在 GUI 上也可见。

源码路径：`cmd/spotter-cli/`；构建：`go build -o bin/spotter-cli ./cmd/spotter-cli`。
