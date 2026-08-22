# 安全策略

## 支持的版本

Spotter 仍在 1.0 之前的开发阶段。下列版本接收安全修复：

| 版本      | 支持状态          |
| --------- | ----------------- |
| `master`  | ✅ 活跃开发        |
| `0.1.x`   | ✅ 尽力修复        |
| `<0.1.0`  | ❌ 已停止维护      |

1.0 之前的 minor 版本**可能**不预告地破坏协议格式
（`/api/v1/info`、`/healthz`、UDP 组播包）。运维人员钉住某个
minor 时，应该预期跨协议版本边界的 client 升级会需要重部署 agent。

英文版本：[`SECURITY.en.md`](SECURITY.en.md)。

## 报告漏洞

**请不要在公开 issue 跟踪器里提安全类 bug。** 改用以下私密渠道之一：

- **邮件**：spotter-security@example.com（上线前请替换为真实维护者邮
  箱）。GPG 公钥等选定后会在此处补登。
- **GitHub Security Advisories**：**仓库 → Security → Advisories →
  「New draft security advisory」**。这是推荐渠道，因为它能把报告与
  修复过程都私有保留到发布之日。

我们承诺在工作日 **3 天内**确认新报告；在有清晰修复路径的前提下，
**30 天内**发布修复或缓解方案。我们遵循 [coordinated disclosure][cd]：
请给我们一段合理的时间窗，再考虑公开。

[cd]: https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure

## 需要包含什么

一份有用的报告应当包含：

1. 受影响的组件（`spotterd` agent、`spotter-client` GUI、UDP 组播包、
   `/api/v1/info` HTTP API、安装 / 部署脚本等）。
2. 具体的复现：设备（`uname -a`、`cat /etc/os-release`），agent 版本
   （`spotterd -V` 或注册表里的 `agent_version`），client 版本（GUI
   「帮助 → 关于」），以及跑过的精确命令。
3. 预期 vs 实际行为，附带任何日志（`journalctl -u spotterd`、
   `~/.config/Spotter/logs/spotter.log`）。
4. 风险面：什么资产受影响（设备本身、LAN、client 主机）。

## 威胁模型与不收的报告范围

Spotter 为**可信 LAN** 设计。下列场景不在范围内，不视为安全漏洞：

- HTTP 端点（`/api/v1/info`、`/healthz`）无需认证即可访问 —— 这是设
  计如此，请只在私网部署。
- LAN 内的其它参与者发现设备 —— 这是设计如此。
- UDP 组播 HELLO 包缺乏签名或完整性保护 —— 这是作为一个加固事项跟踪，
  不是一个 CVE 级别的缺陷。

如果你要把 Spotter 部署到不可信网络，**请不要**。在认证功能加进来之
前，把它放在 VLAN 后面或防火墙之后。

## 运维加固清单

即使在可信 LAN 里，下列基线措施能减小爆炸半径：

1. 通过 systemd 的 `IPAddressAllow=` 与 nftables 把 `spotterd` 的监听
   端口（`/etc/spotterd/agent.toml` 里的 `listen_addr`）限制在管理
   VLAN。
2. 跑 `make release` 产物时核对校验和。每个 `dist/` 制品里的
   `SHA256SUMS` 文件经 release workflow 签名。
3. 订阅 GitHub Releases（**Watch → Custom → Releases**），让公告通
   知能邮件投递给到你。
4. `scripts/deploy.sh` 只走公钥模式（密码模式会让密码残留在 shell 历
   史里）。
