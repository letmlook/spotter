# Spec 偏差记录（SPEC DEVIATIONS）

> 当实现的默认值、字段或行为与既有 spec 不一致时，本文件记录变更理由 + spec 修订 PR 链接。spec 是 source of truth，但实施进度快于 spec 时需透明披露，避免后人按旧 spec 决策。

## v0.2.0 — 治理引入

### DEVIATION-001 默认 `enable_power_actions = true`

- **Spec 来源**：[power-actions-design.md:116](../specs/2026-08-22-power-actions-design.md)（早期版本）
- **现实**：自 0.1 GA 起 `scripts/install.sh` 写 `enable_power_actions = true`；`operations.md` 与 `CHANGELOG.md` 同步声明默认 `true`。
- **原因**：opt-in（默认 false）会让 0.1 的 GUI 按钮看似无响应；调研发现 LAN 部署用户期望"装上就能重启"。`SECURITY.md` 已加注：禁用方式是显式 `enable_power_actions = false`。
- **处理**：spec 第 116 行改写为"默认 `true`（opt-out 设为 `false`）"，并在 `SECURITY.md` 加入"如何显式关闭"的操作指引。

### DEVIATION-002 默认 `enable_log_stream = true`

- **Spec 来源**：[execution-log-stream-design.md:140](../specs/2026-08-22-execution-log-stream-design.md)（早期版本）
- **现实**：自 0.1 GA 起 `enable_log_stream = true` 为默认。
- **原因**：与 DEVIATION-001 一致——LAN 部署期望 GUI「日志 tab」默认有数据。`SECURITY.md` 同时加注。
- **处理**：spec 第 140 行改写为"默认 `true`（opt-out via false）"。

### DEVIATION-003 poll 30s / mcast 60s → 5s / 5s

- **Spec 来源**：[spotter-design.md:56](../specs/2026-08-21-spotter-design.md) §"Discovery cadence"
- **现实**：自 0.1.1 起，client poll 调到 5s，agent HELLO 调到 5s，mcast listen 调到 5s。
- **原因**：30s 太长，UI 上 online/offline 转换肉眼可见；5s 与心跳对齐。`api.md:197`、`operations.md:54`、`README.md` 都已声明 5s。
- **处理**：spec §"Discovery cadence" 章节更新，落地于 commit [0674e98](https://github.com/spotter/spotter/commit/0674e98)（`feat(agent): agent 周期 HELLO + client 加快检测 (5s)`）。

### DEVIATION-004 跨平台（macOS / Linux）从非目标移入目标

- **Spec 来源**：[spotter-design.md:25](../specs/2026-08-21-spotter-design.md) §1.2
- **现实**：当前 `spotter-client` 三端原生支持。
- **处理**：§1.2 中"跨平台客户端（仅 Windows）"改为"v0.2 起三端支持"。

### DEVIATION-005 i18n 与主题部分实装

- **Spec 来源**：[ui-redesign.md:399](../specs/2026-08-21-spotter-ui-redesign.md) §"不在范围内"
- **现实**：已实现中英双语 + "跟随系统"主题；侧栏宽度未实装拖拽。
- **处理**：spec §不在范围保留"拖拽侧栏宽度"，新增"i18n（已双语）"为"已完成"。

### DEVIATION-006 §13「客户端不部署 spotterd」入历史

- **Spec 来源**：[spotter-design.md:582](../specs/2026-08-21-spotter-design.md) §13
- **现实**：plan commit `13598a4 refactor: drop deploy/uninstall-from-GUI` 已经推翻该限制；该 spec 段与历史 plan 一致作废。
- **处理**：在 `docs/superpowers/specs/.history/` 归档旧设计，§13 整段标注"已过时"。

---

## 流程说明

任何后续偏差追加到本文件，CI `spec-check` job 比对本表 + spec + 代码三处默认值。详见 [SPEC_REVIEW.md](SPEC_REVIEW.md)。

---

## 历史 / 弃用

### DEVIATION-007 `listenPort = 9999` 单点硬编码

- **Spec 来源**：[api.md](../../api.md) §"默认端口"
- **现实**：`main.go:60 const listenPort = 9999` 与 `internal/agentd/*.go` 的常量；spec 只声明了端口值，未声明"代码层常量名"。
- **原因**：spec 没规范命名层；多个文件独立硬编码 9999 是技术债候选。
- **v0.3 计划**：抽到 `internal/protocol/defaults.go` 的 `DefaultPort = 9999` 常量，spec-check 自动匹配。
