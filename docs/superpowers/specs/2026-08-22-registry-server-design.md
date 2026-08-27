# Spotter Server 设计 Spec（v0.5.0）

## 目标

新增独立二进制 `spotter-server`，作为 spotterd 注册表与心跳的中心枢纽。多 spotter-client（多运维人员协作、跨子网发现）通过它统一设备列表。

## 范围（in-scope）

1. **REST 端点**（裸 HTTP，无 TLS 1.5+）：
   - `POST /api/v1/devices` — agent 注册（带 device_id、ip、port、token）
   - `PATCH /api/v1/devices/:id` — 更新 ip/port/last_seen_at
   - `POST /api/v1/devices/:id/heartbeat` — agent 周期心跳（idempotent）
   - `GET /api/v1/devices` — client 查询设备列表
   - `GET /api/v1/devices/:id` — client 查询单条
   - `DELETE /api/v1/devices/:id` — client 取消关注
   - `GET /api/v1/audit` — 电源操作 / 配置变更日志（v1.x）

2. **WebSocket `GET /ws/events`**：
   - 客户端订阅设备列表更新（device added/removed/updated/heartbeat_lost）
   - 暂时为只读，后续支持日志流（v0.5.1）

3. **持久化**：JSON 文件 `<dataDir>/server.json`（v0.5 PoC），SQLite（WAL）从 v0.5.1 起
4. **鉴权**：Bearer token（共享 secret 与 spotterd 的 `auth_token` 模型一致）
5. **进程模型**：Go HTTP server + 优雅关闭（SIGINT/SIGTERM）

## 不在范围（v0.5）

- TLS（用 reverse proxy / wireguard 加固）
- 多实例 / HA（v0.5.x 通过 WAL + 共享存储考虑）
- Agent 端 `auth_token` 强制推送（v0.5 安装时不会自动开启）
- 远程命令执行 API（运维动作仍由 client 直连 agent）
- 客户端大量改造（现有 client 直连 agent 模式保留）

## 数据模型

```sql
-- PoC: device / heartbeat 表
devices (
  device_id TEXT PRIMARY KEY,
  ip TEXT NOT NULL,
  port INT NOT NULL DEFAULT 9999,
  token_hash TEXT,                   -- bcrypt(token) 用于 server→agent 反向调
  last_seen_at TIMESTAMP,
  last_source TEXT,                  -- "agent-registered" / "client-pushed"
  online BOOL,
  tags JSON                          -- v0.6 设备分组
);

heartbeats (
  device_id TEXT,
  ts TIMESTAMP,
  uptime_seconds INT,
  PRIMARY KEY (device_id, ts)
);
```

## 测试

- `TestServer_RegisterFlow`：`POST /devices` → `GET /devices/:id` 命中
- `TestServer_HeartbeatTTL`：心跳超时 → 自动转 offline 并 broadcast WS
- `TestServer_TokenGuard`：非注册 device 不能 POST /heartbeat
- `TestServer_ConcurrentClients`：100 client 并发 GET 不串扰

## 风险与回滚

- 单点故障 → 客户端默认直连 agent，`server URL` 是可选字段；缺省时跳过
- JSON 持久化不可靠：v0.5 仅 PoC，缺文件则从空开始
- WebSocket 重连风暴：client 实现 exponential backoff

## 升级路径

- v0.5 GA → 在 v0.6 引入 SQLite WAL
- v0.7 引入 TLS（auto-cert via Let's Encrypt）
- v1.0 引入 OIDC / 多用户 RBAC

## 决策点

| 决策 | 推荐 | 替代 |
|---|---|---|
| 持久化 | JSON（PoC） | SQLite WAL（更可靠） |
| 部署形态 | 单 binary | Docker compose + caddy |
| API 形态 | REST + WS | gRPC streaming |
