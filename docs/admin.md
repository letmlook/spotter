# spotterd `/admin` Web 界面

从 v0.5.0 起，每个 spotterd agent 暴露一个**只读 HTML 监控页面**，无需 Wails 客户端或 API 调用。打开浏览器到 `http://<spotterd-host>:9999/admin` 即可查看：

- Agent 自身身份（hostname、OS、kernel、arch、uptime）
- 网络接口（primary IP + interface 列表）
- Jetson 专属（如果是 Jetson 设备）
- **实时指标**（CPU / 内存 / 温度的最新采样 + 5 分钟历史 JSON 链接）
- **电源审计**（最近 20 条 reboot / shutdown 记录）

## 设计目标

- **零 JS**：纯 HTML + CSS（嵌入到二进制，无需 CDN）
- **零外部资源**：单页 `<300 KB`，无字体、无图片
- **curl 友好**：`curl http://host/admin | less`、`lynx -dump` 是一等公民
- **不会替代 Wails 客户端**：操作能力（reboot / shutdown / 配置）仍在客户端

## 端点

| 路径 | 内容 |
| --- | --- |
| `GET /admin` | 主页（HTML 表格） |
| `GET /admin/` | 同上（trailing slash 兼容） |
| `GET /admin/static/style.css` | 嵌入 CSS |
| `GET /admin/metrics.json` | 最近 60 个采样点 JSON（与 `/api/v1/metrics/recent` 同形） |

## 认证

当 `agent.toml` 中 `[auth]` 启用（`enabled = true` 且 `token = "..."`）时，`/admin/*` 走 **HTTP Basic auth**，密码 = token，user 字段忽略。

浏览器首次访问会弹原生认证对话框；session 内保持登录。

无 token（`enabled = false` 或缺省）时 `/admin` **完全开放**——同网段任何人都可读，仅适用于"LAN 即信任边界"的家庭/实验室部署。

## 与 JSON API 的关系

`/admin` 复用现有 `/api/v1/*` 数据，不引入新的"admin"端点。`/admin/metrics.json` 的响应与 `/api/v1/metrics/recent` **形状完全一致**（`{interval_seconds, samples[]}`），便于 `curl + jq` 流水线：

```bash
$ curl -s -u :token http://host/admin/metrics.json | jq '.samples[-1].cpu_percent'
12.4
```

## 故障排查

**`401 Unauthorized` 且 token 正确**

- 浏览器缓存了旧认证，强制重新登录（地址栏输入 `http://<token>@host/admin`）
- 代理剥离了 Authorization 头（curl 时加 `-i` 看响应头确认）

**`/admin/metrics.json` 返回 503**

- Metrics sampler 没启动（罕见）—— 检查 `agent.toml` 是否手动设了 lifecycle context 而清空
- agent 启动不到 5 秒（sampler 第一次 tick 在 5s 后），等一下

**`/admin` 慢**

- `refreshForAdmin` 每次访问跑一次 fresh collect（5s timeout），慢的 collect 拖累首屏
- 用浏览器 devtools 看 Network 面板定位是否 collect 阻塞

## 资源占用

- 嵌入 CSS：~3 KB
- HTML 模板：~4 KB
- 每次 `/admin` 请求触发一次 `collect()`（同 poll 路径）—— 与 5s 周期内的 poll 复用同一 `*Agent` 状态，**不**额外起 goroutine

## 升级到 v1.0.0

- 加 power action 按钮（CSRF token + 同 token Basic auth 复用）
- 多设备 fleet 视图（agent 当前只能看自己；spotter-server 端已有 `/api/v1/devices`，可挂到 admin 子路径）
- 实时推送（Server-Sent Events 替换轮询）
