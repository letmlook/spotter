# 故障排查

按症状分类的处理手册。每个章节按出现频率列出原因，参考自 0.1.x 开发期
间已经踩过的坑。

英文版本：[`docs/troubleshooting.en.md`](troubleshooting.en.md)。

## 1. 客户端里完全看不到设备

最常见的原因：**agent 与 client 不在同一 L2 广播域**。UDP 组播默认不
跨路由器，除非路由器显式启用 IGMP/PIM。

验证方法：

```bash
# 在设备端：
ip addr show | grep inet          # 记下 IPv4 子网
journalctl -u spotterd --no-pager | tail -n 20

# 在运维机器（任何有 tcpdump / nping 的系统）：
tcpdump -n -i <iface> 'udp port 9999'    # 观察 60 s 是否出现 HELLO 包
```

如果你这边 `tcpdump` 看不到东西，但 `journalctl` 里 agent 在正常每 60 s
广播，**路由器或防火墙在中间悄悄丢掉了组播**。修网络 —— Spotter 解决不
了这个。

修网络之前的临时绕行：

- 在 GUI 里用 **按 IP 添加** 把设备手动注册。
- 或者 **工具 → 扫描子网**。

## 2. 设备出现后又消失

client 每 30 s 对每条 entry `GET /api/v1/info`。连续 3 次失败的轮询，
这一行会切到 offline。常见原因：

- **agent 自身重启** —— client 在下一次轮询自然恢复，无需操作。
- **网络抖动** —— 同上；网络回来后 90 s 内恢复绿色。
- **设备 `listen_addr` 配错** —— `journalctl -u spotterd` 里会出现
  `bind: address already in use` 或 `bind: cannot assign requested
  address`，说明配置的接口已经不在。改 `/etc/spotterd/agent.toml` 后
  `systemctl restart spotterd`。
- **TTL 过期** —— 部分家用路由器会老化长期 UDP 缓存条目。Switch 到子
  网扫描模式可绕开。

## 3. 子网扫描没有结果

scanner 会自动探测本机所在子网。如果运维机器有多个网卡（Wi-Fi + 有线、
VPN + LAN），它挑第一个 RFC1918 的。可以手工覆盖：

- **工具 → 扫描子网 → 指定 CIDR** → 例 `192.168.1.0/24`。
- VPN 子网通常在 RFC1918 之外，必须手工指定 CIDR。

TCP 探活用的是 Go 的 `net.DialTimeout`，每个 IP 500 ms 超时。Wi-Fi 抖
动场景下建议**扫两遍** —— 第二遍能把第一遍漏的设备补回来。

## 4. install.sh 卡在 sudo 提示

> `sudo bash /tmp/install.sh` 然后什么都不发生。

`sudo` 在等 TTY 输入密码。SSH 非交互默认不分配 TTY。三种修复，按推荐
度排序：

1. 给部署用户在目标机配置免密 `sudo`（推荐路径）。
2. `ssh -t user@device` 强制分配 TTY，然后跑脚本。
3. 在 `/etc/sudoers.d/` 写一条 `NOPASSWD: ALL`。

`scripts/deploy.sh` 在不传密码时走的是**公钥模式**，这是最稳的姿势。

## 5. systemctl 报「Unit spotterd.service not found」

`spotterd.service` 没有拷到 `/etc/systemd/system/`。重跑部署脚本。如果
你是手动 scp 的：

```bash
sudo install -m 0644 /tmp/spotterd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now spotterd
```

## 6. UI 白屏 / 一直转圈

多半是启动期 JS 异常。客户端日志落到
`<UserConfig>/Spotter/logs/spotter.log`。如果文件是空的，说明异常发生
在 slog handler 之前；macOS 也请检查 `~/Library/Logs/Spotter/` 下的系统
崩溃报告。

快速诊断：

```bash
# macOS
open -a Console.app   # 筛选 "Spotter"
# Linux
journalctl --user -u spotter-client.service || tail -n 200 ~/.config/Spotter/logs/spotter.log
```

常见原因：

- **`frontend/dist` 残留** —— 删 `build/bin/Spotter.app` 然后
  `make client` 一次。
- **协议 `device_id` 不兼容** —— 只有 agent 版本远高于 client 时才会
  出。升 client。

## 7. 「清空注册表」没生效

**工具 → 清空注册表** 只删除本地 JSON 里所有 entry，但 **scanner 在内存
里仍持有这些设备**。0.1.0 没有注册表的 hot reload —— 直接退出重启 GUI。

## 8. schema_version 不匹配

agent 返回 `schema_version: 2` 而 client 只懂 `schema_version: 1` 时，
GUI 日志里会出现 `decode: schema version 2 not supported`。

修法：让 `agent_version` 与 client 锁的版本对齐。协议设计是**只能向前
滚动**，没有自动降级路径。

## 9. 「按 IP 添加」返回 probe: HTTP 4xx

「按 IP 添加」会真实发 `GET /api/v1/info`。如果设备端返 401/403，说明
agent 配置错了（0.1.0 不该出现这种情况；只有未来版本加上认证后才会有）。
其余情况：

- 端口错 —— `spotterd` 默认 9999。client 自动填，只有当你手工改过
  `/etc/spotterd/agent.toml` 时才需要手动指定。
- 设备端有防火墙 —— 在运维机上 `curl http://<ip>:9999/api/v1/info`
  直接验证。

## 10. 提一个好用的 bug 报告

如果上述条款都对不上，请用 Bug Report 模板
（`.github/ISSUE_TEMPLATE/bug_report.yml`）开 issue，至少包含：

- 版本：`uname -a`、`cat /etc/os-release`、agent `agent_version`、
  client 「帮助 → 关于」里的版本字符串。
- 设备 IP 与运维主机 IP，是否在同一个 L2 域。
- 设备端 `journalctl -u spotterd --no-pager -n 200`，以及 client 的
  `spotter.log`。

安全问题**不要**走公开 tracker，见 [`SECURITY.md`](../SECURITY.md)。
