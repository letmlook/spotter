# Spotter 设备执行日志流式面板

| 项 | 值 |
|---|---|
| 项目名 | **Spotter** |
| 文档日期 | 2026-08-22 |
| 状态 | 设计草案，待用户审核 |
| 范围 | `spotterd` 暴露实时日志流式 HTTP 端点；`spotter-client` GUI 在详情面板底部新增可折叠的实时日志区 |
| 改动文件 | `cmd/agent/`, `internal/agentd/`, `internal/scanner/`, `main.go`, `main_test.go`, `frontend/src/components/{DetailPanel,LogSection}.tsx`, `frontend/src/i18n/dictionaries.ts` |
| 文档更新 | `README.md`, `docs/api.md`, `docs/operations.md`, `SECURITY.md` |

---

## 1. 目标与范围

### 1.1 目标

让 spotter-client 用户能在 GUI 的设备详情面板底部查看 spotterd（以及任何以 systemd unit 形式运行的设备端软件）的**实时执行日志**：

- 默认收起为一个折叠区，标题为「执行日志 / Execution Log」。
- 用户点击展开后，client 通过流式 HTTP 长连接拉到 agent 端的日志，**先回放最近 100 行历史**，再开始 follow 新增行。
- 日志以**单色等宽字体**显示在固定高度区域内，自动滚动到最新一行；前端内存里保留最新 1000 行。
- 收起折叠区或切换设备时，流停止；重新展开会重新订阅。
- agent 端用 `enable_log_stream` 配置开关（默认 `false`），沿用 power-actions 的「opt-in」安全姿态。

### 1.2 非目标（明确排除）

- 新增身份认证 / token / SSH 凭据机制。沿用现有「仅可信局域网」假设。
- 任意文件 / journal 之外的日志源（不接 syslog 服务器、不读容器日志、不接 k8s）。
- 跨重启的日志持久化（agent 端由 systemd journal 自带持久性负责；client 端不持久化，关闭面板即丢）。
- 客户端日志导出 / 复制 / 搜索 / 过滤。
- 多设备并发订阅同一 agent（每设备最多一个 stream；同一设备多次 Start 幂等）。
- 断线重连 / 断点续传（收起再展开视为重新订阅）。

### 1.3 验收标准

| # | 标准 |
|---|------|
| A | agent TOML 中 `enable_log_stream` 缺失或 `false` 时，`GET /api/v1/logs` 返回 `403` + 文本 `log streaming disabled` |
| B | agent TOML `enable_log_stream = true` 且 `log_unit = "spotterd.service"` 时，`GET /api/v1/logs?tail=100` 返回 `200`，`Content-Type: application/x-ndjson`，body 为 NDJSON 流；先回放至多 100 行历史，再 follow 新行 |
| C | agent 主机无 `journalctl`（PATH 中找不到）时，handler 返回 `500` + JSON `{"error":"journalctl not available"}` |
| D | GUI 选中设备、展开折叠区后，1–2s 内看到历史日志；之后每条新日志几乎即时出现（≤ 1s 延迟） |
| E | GUI 收起折叠区后，agent 端 `journalctl` 进程被 kill（不再产生新的 `__REALTIME_TIMESTAMP` 写出，且 OS 进程消失） |
| F | 切换到另一台设备：旧设备的 stream 被 stop、新设备的 stream 开始；旧设备的 `device-log-end:{id}` 事件触发 |
| G | 同一设备重复 Start（idempotent）：第二次调用 `App.StartLogStream` 返回 nil，不创建第二个 goroutine |
| H | 设备 offline 时 `LogSection` 折叠面板标题旁显示 `Offline` 标签且不可展开（保留 disabled 提示） |
| I | 已有测试 `make test` 全部通过；agent handler、scanner reader、App 绑定路径都有单元测试覆盖 |
| J | 前端 cap 1000 行：超过时从最旧裁掉（O(1) slice） |

---

## 2. 现状与障碍

### 2.1 现状

**Agent（`spotterd`）**：

- HTTP 仅暴露 `GET /healthz`、`GET /api/v1/info`、`POST /api/v1/{reboot,shutdown}`（见 `internal/agentd/http.go`）。
- 配置：`Config{DeviceID, ListenAddr, MulticastGroup, AgentVersion, EnablePowerActions}`；`cmd/agent/main.go` 通过 TOML 文件 `agent.toml` 加载。
- `internal/collector/` 下 `basic_linux.go`、`jetson_linux.go`、`network_linux.go`（均带 `//go:build linux`）——本次不直接复用，仅作为「Linux-only」约束的参考。
- 没有现成的子进程 stdout 透传 / 流式响应代码。
- systemd unit `scripts/spotterd.service` 当前以 root 运行，未引入 hardening（power-actions 已提议加入 `NoNewPrivileges`/`ProtectSystem` 等——本次不重复，假设该 hardening 已合入）。

**Client（`spotter-client`）**：

- `internal/scanner/scanner.go` 提供 `HTTPClient() *http.Client`（默认 3s timeout），主要供 poll/probe 使用。
- `main.go` 的 `App` 已绑定：`ListDevices`、`ScanSubnet`、`ProbeByIP`、`RefreshNow`、`AcceptUnknownDevice`、`ClearRegistry`、`RebootDevice`、`ShutdownDevice` 等；`App.ctx` 在 `OnStartup` 注入，前置用 `context.Background()` 占位。
- scanner 事件通过 `wailsruntime.EventsEmit` 推送（见 `NewApp`）。
- 前端 `DetailPanel.tsx` 渲染三张 card（`BasicCard` / `NetworkCard` / `JetsonCard`）和底部 `DeviceActions`；无折叠区。
- i18n：单一字典 `frontend/src/i18n/dictionaries.ts`，中英双 key 平行。
- 前端没 vitest / jest；App 也没测试文件。

### 2.2 与现有架构的对接

- 不引入新 schema 字段；`protocol.DeviceInfo` 不变（避免 bump `SchemaVersion`）。
- agent 端新增独立的 HTTP 路径 `/api/v1/logs`，与 `/api/v1/info` 同样挂在 `mux.HandleFunc`；handler 抽象出可注入的 `startJournalctl` 函数以方便测试。
- client 端不阻塞既有 30s poll 循环；新 stream 走独立的 `http.Client`（无 read timeout），不与 poll client 共享。
- Wails 绑定 `StartLogStream` / `StopLogStream` 为同步函数；流推送以 **Wails event**（`device-log:{id}` / `device-log-error:{id}` / `device-log-end:{id}`）异步发到前端。
- 前端通过 `EventsOn` / `EventsOff` 订阅；折叠区组件 unmount 时清理。

---

## 3. 设计

### 3.1 端到端流程

```
[GUI: 选中设备 → 展开 LogSection]
   │
   ▼
[App.StartLogStream(deviceID)]              ← main.go 新增绑定
   │  reg.Get → ip:port → 入口校验（在线、已注册）
   │  logStreamsMu.Lock → map[id]→cancel；已存在则 no-op
   │  cancel = WithCancel(Background)
   │
   ▼  goroutine
[scanner.StreamDeviceLogs(ctx, ip, port, onLine)]    ← scanner 新增方法
   │  http.NewRequestWithContext(ctx, GET, /api/v1/logs?tail=100)
   │  opts.LogHTTPClient.Do(req)    ← 不复用 3s 超时 client
   │  bufio.Scanner (1MB buffer) 读 NDJSON
   │  journalRecord → LogLine 转换 → onLine
   ▼
[agent handleLogs]                                  ← agentd 新增 handler
   │  cfg.EnableLogStream == false → 403
   │  unit := cfg.LogUnit or "spotterd.service"
   │  exec.LookPath("journalctl") 缺失 → 500 + JSON
   │  startJournalctl(ctx, unit, tail) → io.ReadCloser
   │  w.WriteHeader(200) + Flush
   │  io.Copy(w, stdout) + 定期 Flush
   │  ctx.Done() → journalctl.Process.Kill()
   ▼
[journalctl -u <unit> -n 100 -f --output=json]
   │  每条 record 转 NDJSON: {"ts":"...","line":"...","cursor":"..."}
   ▼
[client reader parse → onLine(LogLine) → emitter.Emit("device-log:"+id, line)]
   ▼
[前端 EventsOn("device-log:"+id) → setLines → 自动滚动]

[GUI: 收起 LogSection / 切走设备]
   │
   ▼
[useEffect cleanup → StopLogStream(id)]
   │  logStreamsMu.Lock → cancel() → delete
   ▼
[scanner reader ctx.Done → 退出]
[agent handler ctx.Done → journalctl.Process.Kill]
[App goroutine defer: emit "device-log-end:"+id]
```

### 3.2 Agent 端

**3.2.1 配置 (`internal/agentd/agent.go`)**

```go
type Config struct {
    // ... existing fields ...
    EnablePowerActions bool

    // New:
    EnableLogStream bool   // 总开关，默认 true（opt-out via false；见 SPEC_DEVIATIONS）
    LogUnit         string // journalctl -u 的 unit 名，默认 "spotterd.service"
}
```

`tomlConfig` 同步：

```go
EnableLogStream bool   `toml:"enable_log_stream"`
LogUnit         string `toml:"log_unit"`
```

`cmd/agent/main.go` 把 `cfg.EnableLogStream` / `cfg.LogUnit` 透传给 `agentd.New` 的 `Config`。

**3.2.2 HTTP handler (`internal/agentd/http.go`)**

```go
mux.HandleFunc("/api/v1/logs", a.handleLogs)
```

```go
const (
    defaultLogTail    = 100
    maxLogTail        = 1000
    logContentType    = "application/x-ndjson"
)

func (a *Agent) handleLogs(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if !a.cfg.EnableLogStream {
        http.Error(w, "log streaming disabled", http.StatusForbidden)
        return
    }
    unit := a.cfg.LogUnit
    if unit == "" {
        unit = "spotterd.service"
    }
    tail := parseLogTail(r.URL.Query().Get("tail"), defaultLogTail) // clamp [1, maxLogTail]

    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming unsupported", http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", logContentType)
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.WriteHeader(http.StatusOK)
    flusher.Flush()

    rc, kill, err := startJournalctl(r.Context(), unit, tail)
    if err != nil {
        // 已 WriteHeader(200)，但 body 尚未发出。可继续写一条 error 行后关闭，
        // client 端 reader 解析失败 → emit error event。
        _, _ = io.WriteString(w, `{"error":"journalctl not available"}`+"\n")
        flusher.Flush()
        a.logger.Error("start journalctl",
            slog.String("unit", unit),
            slog.String("err", err.Error()))
        return
    }
    defer kill()

    // 每行 flush 一次，保证前端 1s 内拿到。
    if err := copyAndFlush(w, flusher, rc); err != nil {
        if !errors.Is(err, context.Canceled) {
            a.logger.Info("log stream ended",
                slog.String("unit", unit),
                slog.String("err", err.Error()))
        }
    }
}

func parseLogTail(raw string, def int) int {
    if raw == "" { return def }
    n, err := strconv.Atoi(raw)
    if err != nil || n <= 0 { return def }
    if n > maxLogTail { return maxLogTail }
    return n
}

func copyAndFlush(w io.Writer, flusher http.Flusher, rc io.ReadCloser) error {
    buf := make([]byte, 16*1024)
    for {
        n, err := rc.Read(buf)
        if n > 0 {
            if _, werr := w.Write(buf[:n]); werr != nil {
                return werr
            }
            flusher.Flush()
        }
        if err != nil {
            if errors.Is(err, io.EOF) { return nil }
            return err
        }
    }
}
```

**3.2.3 journalctl 抽象 (`internal/agentd/log_stream_linux.go`，`//go:build linux`)**

```go
//go:build linux

package agentd

import (
    "context"
    "io"
    "os/exec"
    "strconv"
)

// startJournalctl invokes journalctl in follow mode and returns a
// ReadCloser of its stdout plus a kill callback. Returns an error
// when journalctl is missing or fails to start.
//
// The package-level variable is overridable in tests.
var startJournalctl = func(ctx context.Context, unit string, tail int) (io.ReadCloser, func(), error) {
    if _, err := exec.LookPath("journalctl"); err != nil {
        return nil, nil, err
    }
    args := []string{"-u", unit, "--no-pager", "--output=json", "-n", strconv.Itoa(tail), "-f"}
    cmd := exec.CommandContext(ctx, "journalctl", args...)
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, nil, err
    }
    if err := cmd.Start(); err != nil {
        return nil, nil, err
    }
    kill := func() {
        if cmd.Process != nil {
            _ = cmd.Process.Kill()
        }
        // 回收 Wait，防止僵尸。
        go func() { _ = cmd.Wait() }()
    }
    return stdout, kill, nil
}
```

要点：
- `exec.CommandContext` 保证 client ctx cancel 时 kernel 给进程 SIGKILL（双重保险，再加显式 `Process.Kill`）。
- `defer kill()` 在 handler 返回时回收；正常路径（client 断开导致 ctx cancel）也会触发。
- 使用 `-n <tail> -f`：journalctl 先打印最近 N 行，再跟随新增行（单次请求包揽 history + follow，不拆两步）。

### 3.3 Client 端

**3.3.1 Scanner (`internal/scanner/scanner.go` + `internal/scanner/log_stream.go`)**

新增类型：

```go
// internal/scanner/log_stream.go
package scanner

import "time"

type LogLine struct {
    Ts     string `json:"ts"`     // RFC3339Nano from journalctl __REALTIME_TIMESTAMP
    Line   string `json:"line"`   // MESSAGE
    Cursor string `json:"cursor"` // __CURSOR (reserved for future resume)
}

// journalRecord is journalctl --output=json's wire format. We only
// keep the three fields we surface; everything else is discarded.
type journalRecord struct {
    RealTimeUs string `json:"__REALTIME_TIMESTAMP"` // microseconds, decimal string
    Message    string `json:"MESSAGE"`
    Cursor     string `json:"__CURSOR"`
}
```

`internal/scanner/scanner.go`：

```go
type Options struct {
    // ... existing fields ...
    LogHTTPClient *http.Client // for /api/v1/logs; defaults to no-timeout client
}

func (o Options) withDefaults() Options {
    // ... existing defaults ...
    if o.LogHTTPClient == nil {
        o.LogHTTPClient = &http.Client{Timeout: 0} // 0 = no timeout; rely on ctx.
    }
    return o
}

// WithLogHTTPClient overrides the streaming client.
func WithLogHTTPClient(c *http.Client) func(*Options) {
    return func(o *Options) { o.LogHTTPClient = c }
}
```

```go
// internal/scanner/log_stream.go
func (s *Scanner) StreamDeviceLogs(ctx context.Context, ip string, port int, onLine func(LogLine)) error {
    target := fmt.Sprintf("http://%s:%d/api/v1/logs?tail=100", ip, port)
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
    if err != nil { return err }
    req.Header.Set("Accept", "application/x-ndjson") // 文档化用；agent 不强制校验

    resp, err := s.opts.LogHTTPClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusForbidden {
        return fmt.Errorf("log streaming disabled")
    }
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("log stream: unexpected status %d", resp.StatusCode)
    }

    scanner := bufio.NewScanner(resp.Body)
    buf := make([]byte, 0, 64*1024)
    scanner.Buffer(buf, 1<<20) // 1MB cap per line
    for scanner.Scan() {
        raw := scanner.Bytes()
        // skip empty / heartbeat lines
        if len(raw) == 0 { continue }
        var rec journalRecord
        if err := json.Unmarshal(raw, &rec); err != nil {
            // Best-effort: skip malformed line, continue.
            continue
        }
        line := LogLine{
            Ts:     formatJournalTs(rec.RealTimeUs),
            Line:   rec.Message,
            Cursor: rec.Cursor,
        }
        if ctx.Err() != nil { return ctx.Err() }
        onLine(line)
    }
    if err := scanner.Err(); err != nil {
        if errors.Is(err, context.Canceled) { return nil }
        return err
    }
    return nil
}

func formatJournalTs(us string) string {
    if us == "" { return "" }
    n, err := strconv.ParseInt(us, 10, 64)
    if err != nil { return "" }
    return time.UnixMicro(n).UTC().Format(time.RFC3339Nano)
}
```

要点：
- 用独立的 `LogHTTPClient`（无 read timeout），跟随 ctx。poll/probe 的 3s client 不动。
- `bufio.Scanner` 提升 buffer 到 1 MB 容纳长行；超出则 `scanner.Err()` 返回 `bufio.ErrTooLong`，不 panic。
- malformed JSON 行不中断流；跳过即可（journalctl 正常不会产非 JSON，但保险）。

**3.3.2 App (`main.go`)**

```go
// Emitter abstracts wailsruntime.EventsEmit so tests can substitute.
type Emitter interface {
    Emit(ctx context.Context, eventName string, data ...interface{})
}

type wailsEmitter struct{}
func (wailsEmitter) Emit(ctx context.Context, name string, data ...interface{}) {
    wailsruntime.EventsEmit(ctx, name, data...)
}

type App struct {
    reg          *registry.Registry
    logger       *slog.Logger
    scanner      *scanner.Scanner
    emitter      Emitter
    ctx          context.Context
    logStreams   map[string]context.CancelFunc
    logStreamsMu sync.Mutex
}

func NewApp(reg *registry.Registry, logger *slog.Logger, emitter Emitter) *App {
    if emitter == nil { emitter = wailsEmitter{} }
    app := &App{
        reg: reg, logger: logger, emitter: emitter,
        ctx: context.Background(),
        logStreams: map[string]context.CancelFunc{},
    }
    app.scanner = scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
        app.logger.Info("scanner event", slog.String("tag", e.Tag()))
        app.emitter.Emit(app.ctx, e.Tag(), e)
    }))
    return app
}

// StartLogStream begins streaming the device's execution log. Each
// NDJSON record is emitted as "device-log:{deviceID}" with payload
// LogLine. Idempotent for the same deviceID.
func (a *App) StartLogStream(deviceID string) error {
    entry, ok := a.reg.Get(deviceID)
    if !ok {
        return fmt.Errorf("device not found: %s", deviceID)
    }
    if !entry.Online {
        return fmt.Errorf("device %s is offline", deviceID)
    }
    port := entry.Port
    if port == 0 { port = listenPort }

    a.logStreamsMu.Lock()
    if _, exists := a.logStreams[deviceID]; exists {
        a.logStreamsMu.Unlock()
        return nil
    }
    ctx, cancel := context.WithCancel(context.Background())
    a.logStreams[deviceID] = cancel
    a.logStreamsMu.Unlock()

    go a.runLogStream(ctx, deviceID, entry.IP, port)
    return nil
}

// StopLogStream cancels the active log stream for deviceID. Returns
// nil even if no stream is active.
func (a *App) StopLogStream(deviceID string) error {
    a.logStreamsMu.Lock()
    defer a.logStreamsMu.Unlock()
    if cancel, ok := a.logStreams[deviceID]; ok {
        cancel()
        delete(a.logStreams, deviceID)
    }
    return nil
}

func (a *App) runLogStream(ctx context.Context, deviceID, ip string, port int) {
    defer func() {
        a.logStreamsMu.Lock()
        if c, ok := a.logStreams[deviceID]; ok { c(); delete(a.logStreams, deviceID) }
        a.logStreamsMu.Unlock()
        a.emitter.Emit(a.ctx, "device-log-end:"+deviceID, true)
    }()
    err := a.scanner.StreamDeviceLogs(ctx, ip, port, func(line scanner.LogLine) {
        a.emitter.Emit(a.ctx, "device-log:"+deviceID, line)
    })
    if err != nil && !errors.Is(err, context.Canceled) {
        a.logger.Warn("log stream ended",
            slog.String("device_id", deviceID),
            slog.String("err", err.Error()))
        a.emitter.Emit(a.ctx, "device-log-error:"+deviceID, err.Error())
    }
}
```

### 3.4 前端

**3.4.1 新增 `LogSection.tsx`**

```tsx
// frontend/src/components/LogSection.tsx
import { Alert, Collapse, Space } from 'antd';
import { useEffect, useRef, useState } from 'react';
import { useI18n } from '../state/I18nContext';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { StartLogStream, StopLogStream } from '../../wailsjs/go/main/App';

type LogLine = { ts: string; line: string; cursor: string };

const MAX_LINES = 1000;

export default function LogSection({
  deviceID,
  online,
}: { deviceID: string; online: boolean }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const bufRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    if (!online) { setError(t('log.offline')); return; }
    setError(null);
    setLines([]);
    StartLogStream(deviceID).catch((e) => setError(String(e?.message ?? e)));
    const onLine = (line: LogLine) => {
      setLines((prev) => {
        const next = prev.length >= MAX_LINES
          ? [...prev.slice(prev.length - MAX_LINES + 1), line]
          : [...prev, line];
        return next;
      });
    };
    const onErr = (msg: string) => setError(msg);
    EventsOn('device-log:' + deviceID, onLine);
    EventsOn('device-log-error:' + deviceID, onErr);
    return () => {
      StopLogStream(deviceID).catch(() => {});
      EventsOff('device-log:' + deviceID);
      EventsOff('device-log-error:' + deviceID);
    };
  }, [open, deviceID, online, t]);

  useEffect(() => {
    if (bufRef.current) bufRef.current.scrollTop = bufRef.current.scrollHeight;
  }, [lines, open]);

  return (
    <Collapse
      bordered={false}
      activeKey={open ? ['log'] : []}
      onChange={(keys) => setOpen(Array.isArray(keys) ? keys.includes('log') : !!keys)}
      items={[{
        key: 'log',
        label: (
          <Space size="small">
            <span>{t('log.title')}</span>
            {open && <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>● {t('log.streaming')}</span>}
          </Space>
        ),
        children: error ? (
          <Alert type="error" message={error} />
        ) : (
          <div
            ref={bufRef}
            style={{
              height: 240, overflowY: 'auto',
              background: '#0e0e10', color: '#d4d4d4',
              padding: 8, borderRadius: 4,
              fontFamily: 'ui-monospace, Menlo, monospace',
              fontSize: 12, whiteSpace: 'pre-wrap',
            }}
          >
            {lines.length === 0
              ? <span style={{ color: '#888' }}>{t('log.empty')}</span>
              : lines.map((l, i) => (
                <div key={i}>
                  <span style={{ color: '#7aa2f7' }}>{l.ts}</span>{' '}
                  <span>{l.line}</span>
                </div>
              ))}
          </div>
        ),
        extra: online ? null : (
          <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
            {t('log.disabled_offline')}
          </span>
        ),
      }]}
    />
  );
}
```

要点：
- `useEffect` 仅在 `open && online` 时真正调 `StartLogStream`；cleanup 调 `StopLogStream`。
- `deviceID` 变化 / `online` 变化 → 重新走 effect，旧的 cleanup 先停，再开新的；与「同设备重复 Start 幂等」配合天然无副作用。
- 滚动到底用 ref 直接赋值 `scrollTop`，不参与 React 状态。

**3.4.2 `DetailPanel.tsx` 改动**

将当前两段结构改为三段：

```tsx
<div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
  <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
    {!device ? <EmptyState /> : (
      <>
        <h2>...</h2>
        <div style={{ display: 'grid', gap: 12 }}>
          <BasicCard device={device} />
          <NetworkCard device={device} />
          <JetsonCard device={device} />
        </div>
      </>
    )}
  </div>
  {device && (
    <div style={{ padding: '0 16px 8px 16px', borderTop: '1px solid var(--border-color)' }}>
      <LogSection deviceID={device.device_id} online={device.online} />
    </div>
  )}
  {device && <DeviceActions />}
</div>
```

日志区在中间滚动区之外，与 `DeviceActions` 同一层；不随中间内容滚动。

**3.4.3 i18n (`frontend/src/i18n/dictionaries.ts`)**

新增键（中英双语都给完整版本）：

| key | en | zh |
|---|---|---|
| `log.title` | Execution Log | 执行日志 |
| `log.streaming` | live | 实时 |
| `log.empty` | Waiting for log lines… | 等待日志输出… |
| `log.offline` | Device is offline. Log stream unavailable. | 设备离线，无法查看日志。 |
| `log.disabled_offline` | Offline | 离线 |

---

## 4. 错误处理与边界

| 场景 | agent 行为 | client 行为 | UI 反馈 |
|------|-----------|------------|---------|
| `EnableLogStream=false`（默认） | 403 + `log streaming disabled` | `StreamDeviceLogs` 返回 error → `runLogStream` emit `device-log-error:{id}` | `<Alert type="error">` |
| journalctl 缺失 | 500（handler 提前判断）→ 流开头写一行 `{"error":"journalctl not available"}` | reader 解析失败 → 忽略（best-effort 跳过） | 显示"等待日志输出"占位（不致命） |
| 设备 offline（client 入口） | — | `StartLogStream` 入口 `entry.Online==false` → 返回 error | `<Alert>`：设备离线 |
| 设备不在 registry | — | `StartLogStream` 入口 `reg.Get` 失败 → error | `<Alert>`：设备未注册 |
| 流中途断开 | 连接 close（journalctl 死/网络中断） | reader 返回 error（非 ctx.Canceled）→ emit `device-log-error:{id}` | `<Alert>` 显示错误，用户收起再展开重试 |
| 用户重复 Start 同一设备 | — | map 已有 → no-op return nil | 无副作用 |
| 用户收起 LogSection | — | `useEffect` cleanup → `StopLogStream` → cancel ctx | journalctl 被 SIGKILL |
| 用户切走设备（旧 deviceID 卸载） | — | 同上 | 同上 |
| Wails 进程退出 | — | goroutine 随进程终止 | — |
| 单行 > 1MB | — | `bufio.Scanner` buffer cap → `scanner.Err()` 返回；reader 退出 + emit error | `<Alert>`：单行过长 |
| agent panic | 500（middleware） | reader 拿到非200 → error | `<Alert>` |
| 前端 cap 触发 | — | `setLines` slice 保留后 999 + 新一行 | 视觉上看不到「丢失」；超过 1000 行最旧的裁掉 |

幂等性：
- `App.StartLogStream` 同设备二次调用 no-op。
- `App.StopLogStream` 不存在的 stream no-op。
- 服务端 goroutine 由 ctx cancel 终止，无需显式回收（defer 中 delete map）。

并发：
- `logStreamsMu` 保护 map 读写。
- 同一设备多次快速 Start：第二次命中已有 cancel，直接返回 nil。
- 多个不同设备可并发运行多个 stream；每设备一个 goroutine、一个 cancel。

---

## 5. 测试

### 5.1 Agent handler (`internal/agentd/http_test.go` 扩展 + `log_stream_linux_test.go`)

- `TestHandleLogs_Disabled`: cfg.EnableLogStream=false → 403 + body `log streaming disabled`。
- `TestHandleLogs_WrongMethod`: POST → 405。
- `TestHandleLogs_JournalctlMissing`: 临时把 PATH 清空 / mock `startJournalctl` 返回 error → 响应 body 含 `journalctl not available`。
- `TestHandleLogs_NormalFlow`: mock `startJournalctl` 返回一个产出 NDJSON 的 fake `io.ReadCloser`（如 `bytes.Buffer` + 手工 close）；验证 `Content-Type: application/x-ndjson`、body 内容能读到。
- `TestHandleLogs_ClientDisconnect`: mock 中给 fake rc 一个「读到 N 行后 sleep」的逻辑；用 ctx cancel 模拟 client 断开；验证 `kill` 被调用。
- `TestParseLogTail`: 各种 raw 字符串（空、超界、负数、非数字）→ 默认值/上限/clamp。

`startJournalctl` 是包级变量，测试里：

```go
func TestHandleLogs_JournalctlMissing(t *testing.T) {
    orig := startJournalctl
    startJournalctl = func(_ context.Context, _ string, _ int) (io.ReadCloser, func(), error) {
        return nil, nil, errors.New("not found")
    }
    defer func() { startJournalctl = orig }()
    // ... call handler, assert body ...
}
```

### 5.2 Scanner (`internal/scanner/scanner_test.go` 扩展 + `log_stream_test.go`)

- `TestStreamDeviceLogs_NormalFlow`: `httptest.NewServer` 注册一个 handler，按 `tail=100` 回放 5 行 NDJSON，再持续产生 3 行后保持 open；调用 `StreamDeviceLogs`（用短 ctx），验证回调收到全部 8 行、`Ts` 是 RFC3339Nano、`Line` / `Cursor` 正确。
- `TestStreamDeviceLogs_403`: server 返回 403 + 文本 → reader 返回 error 含 "log streaming disabled"。
- `TestStreamDeviceLogs_ContextCancel`: 流开 200ms 后 cancel ctx → reader 退出（≤ 100ms），回调不再触发。
- `TestStreamDeviceLogs_LongLine`: server 发一行 2 MB → reader 不 panic，回调收到的 line 是被截断的 1 MB 块（bufio.Scanner.ErrTooLong → reader 返回）。
- `TestFormatJournalTs`: `__REALTIME_TIMESTAMP="1700000000000000"` → RFC3339Nano `"2023-11-14T22:13:20Z"`。

### 5.3 App 绑定 (`main_test.go`，新增；package main)

依赖：`Emitter` 接口已抽出 → 测试里用一个 `fakeEmitter` 计数 + 记录 emit 调用；`streamFn func(ctx, ip, port, onLine) error` 字段在 `App` 上，测试里替换为同步 fake（避免依赖真实 HTTP）。

`App` 新增：

```go
type App struct {
    // ... existing ...
    streamFn func(ctx context.Context, ip string, port int, onLine func(scanner.LogLine)) error
}
```

`runLogStream` 内改为 `a.streamFn(ctx, ip, port, onLine)`；默认 `NewApp(reg, logger, emitter)` 里赋值为 `a.scanner.StreamDeviceLogs`。

测试：

- `TestStartLogStream_NotRegistered`: `reg` 空 → 期望 error 含 `not found`。
- `TestStartLogStream_Offline`: 注册 + Online=false → 期望 error 含 `offline`。
- `TestStartLogStream_Online_Success`: 注册 + Online=true + mock `streamFn` 同步 emit 3 行 → 期望 `emitter.Emit("device-log:id", line)` 被调用 3 次 + map 里有 cancel。
- `TestStartLogStream_Idempotent`: 同设备二次 Start → 第二次返回 nil + emitter emit 次数不翻倍（streamFn 只被调一次，因为第二次命中已有 cancel 直接返回）。
- `TestStopLogStream_NoStream`: 未 Start → 返回 nil 不 panic。
- `TestStopLogStream_Cancels`: Start 后 Stop → ctx 被 cancel + map 中 entry 被删 + emitter 收到 `device-log-end:{id}`（在 goroutine 退出时 emit；用 `sync.WaitGroup` 等待）。
- `TestRunLogStream_ErrorPropagates`: mock `streamFn` 返回非 ctx.Canceled error → 期望 emit `device-log-error:{id}` + `device-log-end:{id}`。

### 5.4 前端

不引入 vitest/jest。手工 e2e 清单（PR description）：

- [ ] agent TOML 默认（`enable_log_stream` 缺失）→ UI 展开 → Alert 显示 `log streaming disabled`
- [ ] agent TOML `enable_log_stream = true` + `log_unit = "spotterd.service"` → UI 展开 → 1–2s 看到历史行 + 持续 follow
- [ ] agent 端 `journalctl -u spotterd -n5` 看到 5 行 → UI 展开同样的 5 行出现
- [ ] agent 端 `systemctl restart spotterd`，观察 UI 是否在 restart 期间继续显示旧日志（service 起停不影响 reader）
- [ ] 切设备 → 旧 stream 收尾 + 新 stream 开始
- [ ] 收起折叠区 → 后端 slog 不再出现新的 "log stream ended"，agent 端 `pgrep journalctl` 找不到对应子进程
- [ ] 设备 offline → 折叠区显示 `Offline` 标签，disabled 不可展开
- [ ] 1000 行 cap验证：人为灌入 1500 行 → UI 保持最后 1000
- [ ] 长行（> 1MB）测试：客户端不卡死，显示 Alert

### 5.5 回归

- 已有 `make test` 全部通过。
- 现有 `internal/agentd/http_test.go` 的 power-actions 测试不被影响（mux 多注册一个 path）。
- 现有 `internal/scanner/scanner_test.go` 的 poll/mcast/merge 测试不被影响（新增方法不影响 Options 已有字段）。

---

## 6. 文档更新

### 6.1 `docs/api.md`（中英双语）

新增端点：

```
GET /api/v1/logs?tail=N

说明：
  流式 NDJSON 响应。先回放最近 N 行（默认 100，上限 1000）历史，再 follow 新行。
  每行是单条 JSON 对象：{"ts":"<RFC3339Nano>","line":"<MESSAGE>","cursor":"<__CURSOR>"}。
  仅当 agent.toml 中 enable_log_stream = true 时 200；否则 403 + 文本 "log streaming disabled"。
  无身份认证；部署方负责网络隔离。
```

### 6.2 `docs/operations.md`（中英双语）

在 agent 配置段补 `enable_log_stream` / `log_unit` 字段说明；新增「远程日志查看」段，描述 UI 操作流程。

### 6.3 `README.md`

「已知限制」段：
- 「不支持远端命令执行」改为：「可通过 GUI opt-in 的远程 reboot/shutdown 与执行日志查看（需在 agent 端启用 `enable_power_actions` / `enable_log_stream`）。**不提供 shell 或自定义命令**。」

### 6.4 `SECURITY.md`

- 新增：「`enable_log_stream = true` 等于授予该子网任何客户端读取 agent 的 systemd journal（仅限配置的 unit）。生产环境务必启用 TLS / 隔离 VLAN / VPN」。
- 新增：「该端点返回日志可能包含敏感信息（路径、凭据片段等），开启前评估数据敏感性。」

---

## 7. 风险与回退

| 风险 | 缓解 |
|------|------|
| agent 暴露新端点被同网段攻击者读取日志 | 与现有 `/api/v1/info` 同一假设；默认 `false`；README + SECURITY 明确警告 |
| journalctl 在某些镜像（Alpine / non-systemd）不存在 | handler 返回 500 + error；前端 Alert 提示；不影响其他功能 |
| 长行（恶意或异常）撑爆内存 | bufio.Scanner buffer 1MB cap + emit error，不 panic |
| journalctl 子进程泄漏（client 异常断开后未回收） | `exec.CommandContext` + 显式 `Process.Kill` + goroutine Wait 回收 |
| 同一设备多次 Start 创建多个 stream | map 已有 cancel → no-op |
| 1000 行 cap 用户感知不到丢失 | 文档说明；未来可加导出 / 翻页（不在本期） |
| agent 端 systemd hardening 阻止 journalctl D-Bus 调用 | journalctl 通过文件系统读 binary log，不需要 D-Bus；hardening 不影响 |

回退路径：
- agent TOML `enable_log_stream = false`（或删除该行）+ `systemctl restart spotterd` 即生效。
- 不需要回退代码；后续 release 仅当 client 配 old agent 时，调用会得到 403 → UI 显示 Alert（兼容）。

---

## 8. 范围之外（明确推迟）

- 日志持久化 / 导出 / 搜索 / 过滤 / 高亮
- 多 agent 聚合流
- 客户端断线重连 / 断点续传（cursor 字段已写入响应但本期不消费）
- TLS / 鉴权
- 自定义 log_unit UI 配置（本期仅 agent.toml 静态配置）
- 多设备批量订阅
- 日志行彩色（按 level）渲染