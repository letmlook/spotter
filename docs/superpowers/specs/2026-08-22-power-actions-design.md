# Spotter 远程电源管理（Reboot / Shutdown）

| 项 | 值 |
|---|---|
| 项目名 | **Spotter** |
| 文档日期 | 2026-08-22 |
| 状态 | 设计草案，待用户审核 |
| 范围 | `spotterd` 增加 reboot/shutdown HTTP 端点；`spotter-client` GUI 增加对应按钮 |
| 改动文件 | `cmd/agent/`, `internal/agentd/`, `internal/scanner/`, `main.go`, `frontend/src/...`, `scripts/spotterd.service` |
| 文档更新 | `README.md`, `docs/api.md`, `SECURITY.md` |

---

## 1. 目标与范围

### 1.1 目标

让客户端用户能通过 GUI **远程触发** spotterd 设备的 **reboot** 与 **shutdown**：

- 在 device detail 面板的**标题头（hostname 同行右侧）**新增两个按钮（重启、关机）。
- 点击后 GUI 二次确认（antd Modal），确认后通过 HTTP 触发设备端 `systemctl reboot` / `systemctl poweroff`。
- 操作完成后设备进入正常 online/offline 状态转换（不引入新状态字段）。

### 1.2 非目标（明确排除）

- 新增身份认证 / token / SSH 凭据机制。沿用现有「仅可信局域网」假设。
- 新增 `online / offline` 之外的设备状态（`rebooting` / `shutting-down` 等）。复用现有 poll 失败机制。
- 唤醒（Wake-on-LAN）、定时任务、批量操作。
- 设备端 syslog / audit log 上报（仅本机 slog）。
- 把 `install.sh` / `deploy.sh` 改成新的鉴权模式。

### 1.3 验收标准

| # | 标准 |
|---|------|
| A | agent 端 `enable_power_actions = false`（opt-out）时，`POST /api/v1/reboot` 与 `/shutdown` 均返回 `403` + `{"error":"power actions disabled"}` |
| B | agent 端 `enable_power_actions = true` 时，`POST /api/v1/reboot` 与 `/shutdown` 返回 `202` + `{"status":"scheduled","action":"reboot"}`，并实际执行 `systemctl reboot` / `systemctl poweroff` |
| C | agent HTTP handler 异常 panic 走现有 `recoverMiddleware` 返回 500，不影响进程 |
| D | GUI 设备 offline 时，重启/关机按钮位于详情标题头右侧且 disabled；刷新按钮仍在底部 DeviceActions。点击 online 设备的电源按钮触发 antd Modal 二次确认，文案带设备 hostname；确认后发送 HTTP 请求，成功显示 `message.success` toast |
| E | 客户端调用 `RebootDevice(deviceID)` 时，registry 找不到对应 device 或 device.offline 时返回明确错误（不静默吞错） |
| F | systemd unit 引入 `NoNewPrivileges=true`、`ProtectSystem=strict`、`ProtectHome=true`、`PrivateTmp=true` 后，`systemctl reboot` 仍能正常工作 |
| G | 已有测试 `make test` 全部通过；新增 agent handler 与 scanner 调用路径有单元测试覆盖 |
| H | 现有 e2e / integration 测试不被影响（不破坏 registry poll、UDP multicast 等路径） |

---

## 2. 现状与障碍

### 2.1 现状

**Agent（`spotterd`）**：

- HTTP server 仅暴露 GET 端点：`/healthz`、`/api/v1/info`（见 `internal/agentd/http.go`）。
- 配置通过 `/etc/spotterd/agent.toml` 读取（`internal/agentd/agent.go` 的 `tomlConfig`），字段：`device_id`、`listen_addr`、`multicast_group`、`agent_version`。
- 以 root 身份运行（unit 文件无 `User=`），理论上具备调用 `systemctl` 的权限。
- systemd unit（`scripts/spotterd.service`）当前**没有任何 hardening**：`NoNewPrivileges`/`ProtectSystem`/`ProtectHome`/`PrivateTmp` 均未设置。

**Client（`spotter-client`）**：

- `internal/scanner/scanner.go` 持有 `*http.Client`（`opts.HTTPClient`，默认 3s timeout），当前仅用于 poll/probe。
- `main.go` 的 `App` 结构体已绑定 `ScanSubnet`、`ProbeByIP`、`RefreshNow`、`ListDevices`、`AcceptUnknownDevice`、`ClearRegistry` 等 Wails 方法。
- 前端 `frontend/src/components/DeviceActions.tsx` 目前只有「刷新」按钮；`useDeviceActions` hook 提供 `scan` / `add` / `refresh` 三个方法。
- 前端依赖：React + antd（`Modal`/`message` 已使用）。

### 2.2 MVP 限制的更新

当前 README「已知限制」段已写明：
- **不支持远端命令执行** —— 仅提供静态信息面板。
- HTTP 端点 **无身份认证** —— 仅限可信局域网内部署。

本次实现：
- **取消第一条**：增加 opt-in 的远端电源管理。开启后项目「支持」部分远端命令，但仅限 reboot/shutdown，仍不提供 shell / 自定义命令。
- **保留第二条**：电源管理端点沿用无鉴权模型。

---

## 3. 设计

### 3.1 端到端流程

```
[GUI: 点击重启按钮]
   │  (antd Modal 二次确认，文案带 hostname)
       ▼
[Wails App.RebootDevice(deviceID)]    ← main.go 新增绑定
   │  registry.Get(deviceID) 查 ip:port
   │  port == 0 → 用 listenPort 默认值 (9999)
   ▼
[scanner.RebootDevice(ctx, ip, port)] ← internal/scanner 新增方法
   │  HTTP POST http://<ip>:<port>/api/v1/reboot
   │  使用 s.opts.HTTPClient（沿用，3s timeout）
   │  agent 在 shutdown 时会让连接 hang；客户端 3s 超时即视为「已发送」
   ▼
[agent handler.handleReboot / handleShutdown]
   │  cfg.EnablePowerActions == false → 403 + JSON 错误
   │  cfg.EnablePowerActions == true  → exec.Command("systemctl", action).Start()
   │                                      返回 202 Accepted + JSON {status, action}
   ▼
[systemd 执行 reboot/poweroff]
   │  设备下线 → scanner poll 探测失败 → registry 标记 offline
   │  重启完成后自动 online（新 uptime）
```

### 3.2 Agent 端

**3.2.1 配置 (`internal/agentd/agent.go` + `cmd/agent/main.go`)**

- `Config` 结构体新增字段：
  ```go
  EnablePowerActions bool
  ```
- `tomlConfig` 新增字段：
  ```go
  EnablePowerActions bool `toml:"enable_power_actions"`
  ```
- 默认值：**`true`**（自 v0.2 起，安装脚本会显式写入；opt-out 设为 `false`）。早期 spec 把默认列为 `false` 的立场已被废弃，原因是 0.1 GA 时 install.sh 已经显式开启，文档显式 opt-in 反而误导用户——见 `SPEC_DEVIATIONS.md` 记录。
- `cmd/agent/main.go` 把 `cfg.EnablePowerActions` 透传给 `agentd.New` 的 `Config`。

**3.2.2 HTTP handler (`internal/agentd/http.go`)**

新增路由：

```go
mux.HandleFunc("/api/v1/reboot",   a.handlePowerAction("reboot"))
mux.HandleFunc("/api/v1/shutdown", a.handlePowerAction("shutdown"))
```

抽公共方法，避免两个 handler 重复：

```go
func (a *Agent) handlePowerAction(action string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }
        if !a.cfg.EnablePowerActions {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusForbidden)
            _ = json.NewEncoder(w).Encode(map[string]string{
                "error": "power actions disabled",
            })
            return
        }
        cmd := exec.Command("systemctl", action)
        if err := cmd.Start(); err != nil {
            a.logger.Error("start systemctl",
                slog.String("action", action),
                slog.String("err", err.Error()))
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusInternalServerError)
            _ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
            return
        }
        // Release so the process isn't held by our std{in,out,err} fds.
        _ = cmd.Process.Release()
        a.logger.Info("power action scheduled", slog.String("action", action))
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusAccepted)
        _ = json.NewEncoder(w).Encode(map[string]string{
            "status": "scheduled",
            "action": action,
        })
    })
}
```

要点：
- 仅接受 POST（拒绝 GET / DELETE 等）。
- `cmd.Start()` 异步启动；不调用 `cmd.Wait()`，避免阻塞 handler 与 hang 住连接。
- `Release()` 解除父进程对子进程的 wait 关系，让 systemd 接管。
- panic 走现有 `recoverMiddleware`。

**3.2.3 systemd unit hardening (`scripts/spotterd.service`)**

在 `[Service]` 段末尾追加：

```ini
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
```

为什么可以保留 root + 加 hardening：
- `systemctl` 通过 D-Bus（`org.freedesktop.systemd1`）调用，需要 client UID == 0，与 systemd (PID 1) 通信；不需要 setuid。
- `NoNewPrivileges=true` 阻止进一步提权，但 D-Bus 调用不依赖 setuid。
- `ProtectSystem=strict` 把 `/usr`、`/boot`、`/efi` 等设为只读；agent 只读 `/proc`、`/sys`（已只读），不影响采集。
- `PrivateTmp=true` 给独立 `/tmp`，与系统隔离。
- 不加 `ProtectKernelTunables` / `ProtectKernelModules` 等更激进的限制，避免干扰 `/proc` 读取与 `systemctl` D-Bus socket（`/run/systemd/private`）。

**3.2.4 install.sh 不变**

`install.sh` 已经写默认 TOML。新增字段在 TOML 中缺失时为 false，符合「默认关闭」安全姿态。升级时已存在的 `agent.toml` 不需要修改。

### 3.3 Client 端

**3.3.1 Scanner (`internal/scanner/scanner.go`)**

新增方法：

```go
func (s *Scanner) RebootDevice(ctx context.Context, ip string, port int) error
func (s *Scanner) ShutdownDevice(ctx context.Context, ip string, port int) error
```

实现：
- 调用 `s.opts.HTTPClient.Post(...)` 目标 URL。`s.opts.HTTPClient` 默认 3s 超时（见 `withDefaults`）。
- 超时处理：HTTP 客户端超时（底层 `*url.Error` 包装的 `context.DeadlineExceeded`）时，返回 sentinel 错误 `errPowerActionTimeout`，让 App 层把它当作「乐观成功」处理（设备可能已开始响应但 agent 连接已 hang）。
- 共用一个内部 helper `postPowerAction(ctx, ip, port, action string) error`，避免重复。

```go
var errPowerActionTimeout = errors.New("power action: client timeout, device may have responded")

func (s *Scanner) postPowerAction(ctx context.Context, ip string, port int, action string) error {
    url := fmt.Sprintf("http://%s:%d/api/v1/%s", ip, port, action)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
    if err != nil { return err }
    resp, err := s.opts.HTTPClient.Do(req)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) || isHTTPClientTimeout(err) {
            return errPowerActionTimeout
        }
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusAccepted { return nil }
    if resp.StatusCode == http.StatusForbidden {
        return fmt.Errorf("power actions disabled")
    }
    return fmt.Errorf("unexpected status %d", resp.StatusCode)
}
```

**3.3.2 App (`main.go`)**

新增绑定：

```go
func (a *App) RebootDevice(deviceID string) error
func (a *App) ShutdownDevice(deviceID string) error
```

逻辑：
- `a.reg.Get(deviceID)` 找不到 → `fmt.Errorf("device not found: %s", deviceID)`。
- `entry.Online == false` → 返回明确错误，GUI 用来显示 toast「设备离线，无法操作」。
- `port == 0` → 用 `listenPort` (9999) 默认值。
- `ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)`；5s 余量足以覆盖 LAN 抖动（scanner HTTPClient 默认 3s 超时先到）。
- 调用 `a.scanner.RebootDevice / ShutdownDevice`。
- 若返回 `errPowerActionTimeout`，**转换为 nil**（乐观成功），由 UI 显示「重启命令已发送」。

### 3.4 前端

**3.4.1 `useDeviceActions` hook**

新增方法：

```typescript
reboot: (deviceID: string) => Promise<void>;
shutdown: (deviceID: string) => Promise<void>;
```

调用 `RebootDevice(deviceID)` / `ShutdownDevice(deviceID)`（Wails 自动生成的绑定）。

**3.4.2 按钮放置 — 详情标题头右侧 + 底部刷新**

布局调整分两部分：

1. **重启 / 关机按钮**：放在 `DetailPanel.tsx` 的标题头 `<h2>` 行右侧，与 hostname + online 状态同行。标题头改为 flex 行：左侧 hostname + 状态文字，右侧 `Space` 容器装两个按钮。

   ```
   ┌────────────────────────────────────────────────────────────┐
   │ hostname         [● online]               [🔌 重启] [⏻ 关机]│  ← 标题头
   ├────────────────────────────────────────────────────────────┤
   │ <BasicCard>                                                 │
   │ <NetworkCard>                                               │
   │ <JetsonCard>                                                │
   ├────────────────────────────────────────────────────────────┤
   │ ACTIONS                                                     │
   │ [🔄 刷新]                                                   │  ← 底部 DeviceActions
   └────────────────────────────────────────────────────────────┘
   ```

   实现要点（在 `DetailPanel.tsx` 的 `<h2>` 节点上）：
   - `<h2>` 改为 `display: flex; justifyContent: space-between; alignItems: center`。
   - 左侧：`<span>` 包 hostname + 状态。
   - 右侧：`<Space>` 包两个 `<Button>`（`icon` + 文案），`size="small"`。
   - 重启按钮：`icon={<PoweroffOutlined />}`，普通样式（蓝色）。
   - 关机按钮：`icon={<PoweroffOutlined />}` 或 `{<CloseCircleOutlined />}`，`danger` 类型（红色）。
   - 两个按钮的 `disabled={!device.online}`；`loading={busyAction}` 由各自 state 控制。

2. **刷新按钮**：保留在底部 `DeviceActions`，逻辑不变。

行为：
- 重启 / 关机按钮在 `device.online === false` 时 `disabled`。
- 点击电源按钮触发 `Modal.confirm`：
  - 标题：`即将重启 ${hostname}` / `即将关闭 ${hostname}`。
  - 内容：警告语 + 操作不可逆说明（关机的强调「需手动上电才能恢复」）。
  - OK 按钮文案：`重启` / `关闭电源`，`danger` 类型（红色）。
  - 取消按钮：保留默认。
- 确认后 `await actions.reboot(deviceID)` / `shutdown(deviceID)`：
  - 成功 → `message.success('重启命令已发送')`。
  - 失败 → `message.error(err.message)`；错误包含 `"power actions disabled"` 时，UI 文案改为「该设备未启用远程电源管理（需在 agent 配置 enable_power_actions = true）」。

**3.4.3 i18n**

`frontend/src/i18n/` 下新增字符串键：
- `detail.actions.power.reboot` / `detail.actions.power.shutdown`
- `detail.actions.power.reboot.confirmTitle` / `...shutdown.confirmTitle`
- `detail.actions.power.reboot.confirmOk` / `...shutdown.confirmOk`
- `detail.actions.power.toast.success` / `...toast.disabledHint`

中英双语都给完整版本（沿用项目现有约定）。

---

## 4. 错误处理与边界

| 场景 | agent 响应 | client 处理 | UI 反馈 |
|------|-----------|-------------|---------|
| 开关 OFF | 403 + `{"error":"power actions disabled"}` | 透传错误 | toast: 该设备未启用远程电源管理 |
| 开关 ON，命令执行失败 | 500 + `{"error":...}` | 透传 | toast: 命令执行失败 |
| 设备 offline（client 侧检查） | — | `RebootDevice` 返回 error | toast: 设备离线，无法操作 |
| 设备不存在（client 侧检查） | — | `RebootDevice` 返回 error | toast: 设备未在注册表中 |
| agent panic | 500（中间件） | 透传 | toast: 服务器内部错误 |
| client 3s HTTP 超时 | —（agent 可能已返回 202 但连接 hang；或设备已开始重启） | scanner 返回 `errPowerActionTimeout`；App 视为成功 | toast: 重启命令已发送 |
| 用户点击两次 | — | 按钮 `loading` 期间禁用 | 第二次点击无效果 |

幂等性：reboot/shutdown 本身非幂等操作；UI 通过 `loading` 状态防止重复触发。

---

## 5. 测试

### 5.1 Agent handler 测试

`internal/agentd/http_test.go`（新增文件 / 扩展示有测试）：
- `TestHandleReboot_Disabled`：cfg.EnablePowerActions=false → 期望 403 + JSON `{"error":"power actions disabled"}`。
- `TestHandleReboot_Enabled`：cfg.EnablePowerActions=true，mock `exec.Command`（用 `os/exec` 的接口抽象，或直接走 http 集成测试用 httptest）。
- `TestHandleReboot_WrongMethod`：GET → 405。
- `TestHandleShutdown_Disabled`：同上。
- 用 `httptest.NewRecorder` + 直接调 `a.Handler()`；不需要真实 socket。

### 5.2 Scanner 测试

`internal/scanner/scanner_test.go`（扩展）：
- 用 `httptest.NewServer` 起一个 mock agent，返回 202 / 403 / 500。
- 验证 `RebootDevice / ShutdownDevice` 在三种状态下的返回。
- 验证超时：mock server hang 超过 5s，验证 ctx deadline 路径。

### 5.3 App 绑定测试

`main.go` 的 App 方法当前没有测试文件。**本次不引入新测试文件**（避免扩散范围）；通过 scanner 单元测试间接覆盖。

### 5.4 前端测试

项目当前没有前端单元测试框架（`frontend/package.json` 无 vitest/jest 依赖）。**本次不引入新框架**；依靠手动验证 + e2e 流程。

### 5.5 手工 e2e 验证清单（在 PR description 里列出）

- [ ] 两台同网段 Linux 设备 + Mac 客户端；agent TOML 加 `enable_power_actions = true` 重启 spotterd。
- [ ] Mac 客户端 GUI 显示设备 online；点击重启按钮 → Modal 确认 → toast 成功 → 设备短暂 offline → 自动重新 online。
- [ ] 同样路径验证关机（设备需手动开机恢复）。
- [ ] 把 TOML 改回 `enable_power_actions = false`（或删除），点击按钮 → toast 提示「未启用」。
- [ ] 在客户端把设备标记 offline（在 settings 中断开网络模拟），确认按钮 disabled。

---

## 6. 文档更新

### 6.1 `README.md`

「已知限制」段：
- 「不支持远端命令执行」改为：「可通过 GUI opt-in 的远程 reboot/shutdown（需在 agent 端启用 `enable_power_actions`）。**不提供 shell 或自定义命令**。」
- 「HTTP 端点无身份认证」保留，措辞微调为「仍仅限可信局域网内部署；启用电源管理等于授予 root 重启/关机的权限」。

### 6.2 `docs/api.md`（中英双语）

新增端点定义：

```
POST /api/v1/reboot
POST /api/v1/shutdown

Request:
  Headers: Content-Type 不要求 body

Response (200/202):
  {
    "status": "scheduled",
    "action": "reboot" | "shutdown"
  }

Response (403, power actions disabled):
  {
    "error": "power actions disabled"
  }

Response (405, non-POST): 文本 "method not allowed"

说明：
  仅当 agent.toml 中 enable_power_actions = true 时 202。
  该操作无身份认证；部署方负责网络隔离。
```

同时在「Agent 配置」段加入 `enable_power_actions` 字段说明（默认 false）。

### 6.3 `SECURITY.md`

加固 checklist 中新增：
- 「远程电源管理启用前，确保 agent 主机在受控 VLAN / VPN 后」。
- 「`enable_power_actions = true` 等于授权该子网任何客户端触发 root 级别的 reboot/poweroff」。

### 6.4 `docs/operations.md`

设备端部署段补一句：在 `agent.toml` 中设置 `enable_power_actions = true` 可启用 GUI 远程电源管理。

---

## 7. 风险与回退

| 风险 | 缓解 |
|------|------|
| 用户误触关机，设备长期不可达 | UI 必须二次确认；关机文案强调「需手动上电」 |
| agent 端 systemd hardening 影响现有 `/proc` `/sys` 读取 | 在 Ubuntu 22.04 / 24.04 + ARM64 Jetson + AMD64 各跑一次 `make test` + 手工 collect 验证 |
| 客户端网络抖动误报超时 | scanner 把 `context.DeadlineExceeded` 视为「可能已发送」不报错；UI 显示乐观成功 |
| agent 暴露新端点被同网段攻击者利用 | 与现有 `/api/v1/info` 同一假设；README 与 SECURITY 明确警告 |
| 字段名拼写错误导致 TOML 不识别 | `enable_power_actions` 用 snake_case，与现有 TOML 字段一致；agent 启动时不报错（缺失字段 = 默认 false） |

回退路径：把 `enable_power_actions` 改回 `false`（或删除该行）+ `systemctl restart spotterd` 即生效，无需重新部署。

---

## 8. 范围之外（明确推迟）

- 电源操作的 dry-run / 预览模式
- 电源操作的限流（rate limit）
- 电源操作的审计日志（本地 slog 已有；不上报）
- 多设备批量操作
- 与 systemd inhibitor 集成（避免 reboot 时被打断）
- 自定义 reboot 时间（延时关机 / `shutdown +5`）