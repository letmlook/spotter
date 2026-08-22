# 运维指南

本文档覆盖 Spotter 部署**之后**的操作：生命周期、配置、日志位置、日常维
护。安装步骤见 `README.md`。

英文版本：[`docs/operations.en.md`](operations.en.md)。

## 组件一览

| 组件              | 进程                          | 所在位置                  |
| ----------------- | ----------------------------- | ------------------------- |
| `spotterd`        | systemd 单元 (`spotterd.service`) | 每台被管理的 Linux 设备 |
| `spotter-client`  | Wails 桌面应用                | 运维人员的 macOS / Windows / Linux |
| 注册表            | `<UserConfig>/Spotter/devices.json` | 仅 client 主机        |
| agent 配置        | `/etc/spotterd/agent.toml`    | 每台设备                  |

## 设备上的目录布局

```
/usr/local/bin/spotterd               # agent 二进制，权限 0755
/etc/systemd/system/spotterd.service  # systemd 单元，权限 0644
/etc/spotterd/                        # agent 配置目录，权限 0755
    └── agent.toml                    # device_id、listen_addr、multicast
/var/log/journal/                     # spotterd 日志（journald）
```

`ssh user@device sudo systemctl status spotterd` 是最常用的健康检查。
要查看实时日志：

```bash
ssh user@device journalctl -u spotterd -f
```

## 客户端主机的目录布局

| 操作系统 | 路径                                                       |
| -------- | ---------------------------------------------------------- |
| macOS    | `~/Library/Application Support/Spotter/devices.json`      |
|          | `~/Library/Application Support/Spotter/logs/spotter.log`   |
| Linux    | `~/.config/Spotter/devices.json`                           |
|          | `~/.config/Spotter/logs/spotter.log`                       |
| Windows  | `%APPDATA%\Spotter\devices.json`                           |
|          | `%APPDATA%\Spotter\logs\spotter.log`                       |

## 配置

### Agent (`/etc/spotterd/agent.toml`)

```toml
device_id        = "9d1f2c5e-…"     # UUID v4，由 install.sh 生成
listen_addr      = "0.0.0.0:9999"   # 监听接口与端口
multicast_group  = "239.255.42.42:9999"  # 发现组
agent_version    = "0.1.0"          # 由 install.sh 写入
```

修改监听端口：

```bash
ssh user@device sudo sed -i 's/listen_addr = "0.0.0.0:9999"/listen_addr = "0.0.0.0:9100"/' /etc/spotterd/agent.toml
ssh user@device sudo systemctl restart spotterd
```

组播组在 install 时确定。改了它需要同步改 systemd unit，更省事的做法是
带新的 `MULTICAST_GROUP` 环境变量重跑 `scripts/install.sh`。

### 客户端注册表（`devices.json`）

纯 JSON，client 关闭时也可以直接看。其中两类编辑是安全的：

- 删除某个 entry，下一次 30 s 轮询会自然把它重建（强制重新发现一台设备）。
- 把整个文件挪走 → 清零状态。GUI 会从下一次的组播 / 探活重新构建。

不要手改 `LastInfo`，每次轮询都会覆盖。

## 日常任务

### 设备平滑重启

```bash
ssh user@device sudo systemctl restart spotterd
```

client 会先把这一行标成 offline（~30 s），再在下一次成功的轮询里绿回来。
这是预期的行为，不是 bug。

### 设备换网络（迁到新子网）

1. 在 client 里把过期的 entry 从注册表删掉。
2. 接下来 30 s 轮询会失败，行变 offline。
3. 用「按 IP 添加」重新注册一次 —— client 优先信任最近一次探活的条目。

或者直接重跑 `scripts/deploy.sh`，把设备那台机器的
`/etc/spotterd/agent.toml` 整个换掉，GUI 会按新的 IP 重新发现。

### 升协议版本

协议字段变化通过 `internal/protocol/info.go` 里的 `schema_version`
字段升主版本号。同步更新 `wails.json`（client）与
`scripts/install.sh`（agent）里写入的 `agent_version`。`protocol` 包自己
有一组测试用来锁住向后兼容解码行为。

### 卸载

设备端：

```bash
sudo bash /tmp/uninstall.sh        # 只停 + 删服务
sudo bash /tmp/cleanup.sh         # 顺带清配置和 /tmp 残留文件
```

客户端：**工具 → 清空注册表**，或者退出后删掉 `<UserConfig>/Spotter/`。

## 升级策略

Spotter 还在 1.0 之前。minor 版本可能会引入协议不兼容变更。设备与
client 应当同步升级：

1. 打 release tag（`git tag v0.2.0`）。
2. GitHub Actions release workflow 自动构建 Linux arm64/amd64 二进制。
3. 用 `scripts/deploy.sh` 逐台设备推送新二进制。
4. 更新 client（自动更新还没做 —— 用户从 GitHub Release 下载新版本）。

对多 VLAN 部署，按 (3) 一台台推，把爆炸半径压到最小。

## 可观测性

0.1.0 没有集中聚合器。最简单的做法是把 `journalctl -u spotterd` 接到你已
有的日志收集（Promtail / Vector / fluent-bit）。agent 用 `slog` 把 JSON
写到 stderr，journald 把每条记录作为一行捕获。
