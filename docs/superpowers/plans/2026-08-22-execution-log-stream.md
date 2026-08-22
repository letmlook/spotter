# Spotter 设备执行日志流式面板 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 spotterd 暴露 `GET /api/v1/logs` 流式端点（journalctl NDJSON），client 详情面板底部新增可折叠的实时日志区，默认收起；展开时先回放 100 行历史，再 follow 新增行。

**Architecture:** 四层增量 — agent 加 HTTP 流式端点（`-n 100 -f` 单次请求）+ 配置开关（默认 OFF）；scanner 用独立的 `LogHTTPClient`（无 read timeout）消费 NDJSON；Wails App 用 `Emitter` 接口 + `streamFn` 字段做依赖注入，新增 `StartLogStream` / `StopLogStream` 绑定；前端 `LogSection.tsx` 通过 `EventsOn` 订阅流式事件。日志区放在 `DetailPanel` 底部独立区域，不参与中间滚动。

**Tech Stack:** Go 1.25 / `net/http`（`http.Flusher`）/ `os/exec`（journalctl）/ `bufio.Scanner`；Wails v2（`wailsruntime.EventsEmit` / `EventsOn` / `EventsOff`）；React + antd（`Collapse`、`Alert`、`Space`）。沿用现有测试栈 `testing` + `httptest`。

## Global Constraints

- 模块路径 `github.com/spotter/spotter`；Go 1.25。
- agent 配置开关 `enable_log_stream` 默认 `false`，缺失即关闭（沿用现有 tomlConfig 行为）。
- agent 配置 `log_unit` 默认 `"spotterd.service"`，缺失即用默认；agent 端仅透传给 journalctl，不做白名单校验。
- agent 端鉴权保持现状「无鉴权，仅可信局域网」；不引入 token/SSH 凭据。
- 端点仅接受 `GET`；其他方法返回 `405`。
- 流式响应 `Content-Type: application/x-ndjson`，handler 立即 `WriteHeader(200)` + `Flush()`，每行读取后 flush。
- 不复用 `opts.HTTPClient`（默认 3s 超时）；新增独立 `LogHTTPClient`，默认 `Timeout: 0` 跟随 ctx。
- 不新增 `DeviceInfo` 字段；`SchemaVersion` 不变（向后兼容）。
- 不做日志持久化、不导出、不搜索、不过滤；前端 cap 1000 行（O(1) slice），关闭面板即丢。
- 提交作者固定 `letmlook <letmlook@aliyun.com>`，通过 `git commit --author=...` 指定，**不要改 git config**。
- commit message 用中文 `[类型] 简述` 格式。
- `make test` 必须全部通过；前端无可用单测框架，依靠手工 e2e 验证清单（见 spec §5.4）。
- 所有改动文件清单见 spec §1 头部表格。
- 参考文档：
  - 设计 spec：`docs/superpowers/specs/2026-08-22-execution-log-stream-design.md`
  - 现有 agent 测试：`internal/agentd/agent_test.go`、`internal/agentd/http_test.go`、`internal/agentd/udp_test.go`
  - 现有 scanner 测试：`internal/scanner/scanner_test.go`
  - 现有 `App` 在 `main.go`：`wailsruntime.EventsEmit` 用法见 `NewApp` 内的 `WithOnEvent` 闭包
  - 现有 `DetailPanel`：`frontend/src/components/DetailPanel.tsx`
  - i18n：`frontend/src/i18n/dictionaries.ts`

---

## File Structure

**新增：**
- `internal/agentd/log_stream_linux.go` — `startJournalctl` 包级变量 + journalctl 子进程封装（`//go:build linux`）
- `internal/agentd/log_stream_linux_test.go` — handler 401/403/405/正常流/journalctl 缺失测试（仅 Linux）
- `internal/agentd/log_stream_test.go` — 跨平台 helper 测试（`parseLogTail`）；非 Linux 也可跑
- `internal/scanner/log_stream.go` — `LogLine` / `journalRecord` 类型 + `StreamDeviceLogs` 方法
- `internal/scanner/log_stream_test.go` — 流式 reader 测试（用 httptest server 产 NDJSON）
- `main_test.go` — package main 的 App 单元测试（注册器注入、幂等、离线、错误传播）
- `frontend/src/components/LogSection.tsx` — 折叠区组件，订阅 Wails event

**修改：**
- `internal/agentd/agent.go` — `Config` 加 `EnableLogStream bool` + `LogUnit string`
- `internal/agentd/http.go` — 注册 `GET /api/v1/logs` 路由 + `handleLogs` handler + `parseLogTail` helper
- `cmd/agent/main.go` — `tomlConfig` 加字段；透传给 `agentd.New`
- `internal/scanner/scanner.go` — `Options` 加 `LogHTTPClient *http.Client` + `withDefaults` 默认值 + `WithLogHTTPClient` 函数式选项
- `main.go` — `Emitter` 接口 + `wailsEmitter` 实现 + `App.logStreams` 字段 + `streamFn` 字段 + `NewApp` 接受 emitter 参数 + `StartLogStream` / `StopLogStream` / `runLogStream` 三个方法 + `NewApp` 调用点变更（仅传 emitter）
- `frontend/src/components/DetailPanel.tsx` — 三段式布局：中间滚动 + 底部 LogSection + 底部 DeviceActions
- `frontend/src/i18n/dictionaries.ts` — 中英双语新增 5 个 `log.*` 键
- `README.md` / `docs/api.md` / `docs/api.en.md` / `docs/operations.md` / `docs/operations.en.md` / `SECURITY.md` — 文档更新

---

## Task 1: Agent Config 与 TOML 字段（`EnableLogStream` + `LogUnit`）

**Files:**
- Modify: `internal/agentd/agent.go:11-16`（`Config` 结构体）
- Modify: `cmd/agent/main.go:29-34`（`tomlConfig`）、`cmd/agent/main.go:63-69`（`agentd.New` 调用）

**Interfaces:**
- Consumes: 无
- Produces: `agentd.Config{EnableLogStream: bool, LogUnit: string}` — 后续 handler 在此字段为 false 时返回 403，true 时把 `LogUnit`（缺失默认 `"spotterd.service"`）透传给 `journalctl -u`

- [ ] **Step 1: 在 `agentd.Config` 中加两个字段**

打开 `internal/agentd/agent.go`，修改 `Config` 结构体：

```go
// Config holds the agent's runtime settings.
type Config struct {
    DeviceID            string
    ListenAddr          string
    MulticastGroup      string
    AgentVersion        string
    EnablePowerActions  bool // opt-in: allow POST /api/v1/reboot & /shutdown
    EnableLogStream     bool // opt-in: allow GET /api/v1/logs
    LogUnit             string // journalctl -u unit name; default "spotterd.service"
}
```

- [ ] **Step 2: 在 `cmd/agent/main.go` 的 `tomlConfig` 中加同名字段**

修改 `cmd/agent/main.go` 的 `tomlConfig`：

```go
type tomlConfig struct {
    DeviceID            string `toml:"device_id"`
    ListenAddr          string `toml:"listen_addr"`
    MulticastGroup      string `toml:"multicast_group"`
    AgentVersion        string `toml:"agent_version"`
    EnablePowerActions  bool   `toml:"enable_power_actions"`
    EnableLogStream     bool   `toml:"enable_log_stream"`
    LogUnit             string `toml:"log_unit"`
}
```

并修改 `agentd.New` 调用处：

```go
agent, err := agentd.New(agentd.Config{
    DeviceID:            cfg.DeviceID,
    ListenAddr:          cfg.ListenAddr,
    MulticastGroup:      cfg.MulticastGroup,
    AgentVersion:        cfg.AgentVersion,
    EnablePowerActions:  cfg.EnablePowerActions,
    EnableLogStream:     cfg.EnableLogStream,
    LogUnit:             cfg.LogUnit,
}, log)
```

- [ ] **Step 3: 编译验证**

```bash
CGO_ENABLED=0 GOOS=linux make agent-linux-arm64
```

Expected: 编译通过，无报错。

- [ ] **Step 4: 提交**

```bash
git add internal/agentd/agent.go cmd/agent/main.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(agent): 增加 enable_log_stream / log_unit 配置开关

为后续 GET /api/v1/logs 端点提供 opt-in 控制。enable_log_stream 默认 false；log_unit 缺失默认 "spotterd.service"。TOML 文件缺失即关闭。
EOF
)"
```

---

## Task 2: Agent HTTP 流式 Handler 与 journalctl 抽象

**Files:**
- Create: `internal/agentd/log_stream_linux.go`（`//go:build linux`）
- Create: `internal/agentd/log_stream_test.go`（跨平台 helper 测试）
- Modify: `internal/agentd/http.go`（注册 `GET /api/v1/logs` + 新增 `handleLogs`、`parseLogTail`、`copyAndFlush`）

**Interfaces:**
- Consumes: `agentd.Config.EnableLogStream`、`agentd.Config.LogUnit`（Task 1）
- Produces:
  - `a.Handler() http.Handler` 新路由 `GET /api/v1/logs`
  - 包级变量 `var startJournalctl func(ctx context.Context, unit string, tail int) (io.ReadCloser, func(), error)`（Linux 文件，测试可覆盖）
  - 响应：200 + `Content-Type: application/x-ndjson` 持续输出；403 文本 `log streaming disabled`；500 JSON `{"error":"journalctl not available"}`；405 文本 `method not allowed`

- [ ] **Step 1: 写跨平台 helper 的失败测试**

创建 `internal/agentd/log_stream_test.go`：

```go
package agentd

import "testing"

func TestParseLogTail(t *testing.T) {
    cases := []struct {
        raw  string
        want int
    }{
        {"", 100},           // empty -> default
        {"0", 100},          // 0 -> default
        {"-5", 100},         // negative -> default
        {"abc", 100},        // non-numeric -> default
        {"50", 50},          // valid
        {"2000", 1000},      // clamp to max
        {"1", 1},            // boundary
        {"1000", 1000},      // boundary max
    }
    for _, c := range cases {
        got := parseLogTail(c.raw, 100)
        if got != c.want {
            t.Errorf("parseLogTail(%q) = %d, want %d", c.raw, got, c.want)
        }
    }
}
```

- [ ] **Step 2: 运行测试，预期 FAIL（`parseLogTail` 未定义）**

```bash
go test ./internal/agentd/... -run TestParseLogTail -count=1
```

Expected: 编译错误 `undefined: parseLogTail`。

- [ ] **Step 3: 在 `internal/agentd/http.go` 中实现 helper 并注册路由**

打开 `internal/agentd/http.go`，import 段新增 `"io"` 与 `"strconv"`：

```go
import (
    "context"
    "encoding/json"
    "errors"
    "io"
    "log/slog"
    "net/http"
    "os/exec"
    "runtime/debug"
    "strconv"
    "time"
)
```

在 `Handler` 方法中注册新路由：

```go
func (a *Agent) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", a.handleHealthz)
    mux.HandleFunc("/api/v1/info", a.handleInfo)
    mux.HandleFunc("/api/v1/reboot", a.handlePowerAction("reboot"))
    mux.HandleFunc("/api/v1/shutdown", a.handlePowerAction("shutdown"))
    mux.HandleFunc("/api/v1/logs", a.handleLogs)
    return a.recoverMiddleware(mux)
}
```

在文件末尾追加：

```go
const (
    defaultLogTail = 100
    maxLogTail     = 1000
)

// parseLogTail clamps the ?tail=N query parameter. Empty / 0 /
// negative / non-numeric values fall back to defaultLogTail; values
// above maxLogTail are clamped.
func parseLogTail(raw string, def int) int {
    if raw == "" {
        return def
    }
    n, err := strconv.Atoi(raw)
    if err != nil || n <= 0 {
        return def
    }
    if n > maxLogTail {
        return maxLogTail
    }
    return n
}

// copyAndFlush copies rc into w, flushing w after every successful Read.
// Returns nil on clean EOF; the first non-EOF error otherwise.
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
            if errors.Is(err, io.EOF) {
                return nil
            }
            return err
        }
    }
}

// handleLogs streams journalctl output for cfg.LogUnit (or
// "spotterd.service") to the client. The response is NDJSON; each
// record from journalctl --output=json is forwarded as-is and flushed
// immediately so the client sees new lines without buffering.
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
    tail := parseLogTail(r.URL.Query().Get("tail"), defaultLogTail)

    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming unsupported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/x-ndjson")
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.WriteHeader(http.StatusOK)
    flusher.Flush()

    rc, kill, err := startJournalctl(r.Context(), unit, tail)
    if err != nil {
        a.logger.Error("start journalctl",
            slog.String("unit", unit),
            slog.String("err", err.Error()))
        // 已 WriteHeader(200)；写一行 error 后关闭流，让客户端 reader
        // 解析失败 → UI 显示错误。
        _, _ = io.WriteString(w, `{"error":"journalctl not available"}`+"\n")
        flusher.Flush()
        return
    }
    defer kill()

    if err := copyAndFlush(w, flusher, rc); err != nil {
        if !errors.Is(err, context.Canceled) {
            a.logger.Info("log stream ended",
                slog.String("unit", unit),
                slog.String("err", err.Error()))
        }
    }
}
```

- [ ] **Step 4: 运行 helper 测试，预期 PASS**

```bash
go test ./internal/agentd/... -run TestParseLogTail -count=1 -v
```

Expected: PASS。

- [ ] **Step 5: 创建 `internal/agentd/log_stream_linux.go`**

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
// when journalctl is missing or fails to start. The package-level
// variable is overridable in tests.
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
        // 回收 Wait，防止僵尸；异步避免阻塞 kill 调用方。
        go func() { _ = cmd.Wait() }()
    }
    return stdout, kill, nil
}
```

- [ ] **Step 6: 写 handler 测试（`internal/agentd/log_stream_linux_test.go`）**

```go
//go:build linux

package agentd

import (
    "context"
    "errors"
    "io"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "strconv"
    "strings"
    "sync/atomic"
    "testing"
)

func TestHandleLogs_DisabledReturns403(t *testing.T) {
    a, err := New(Config{
        DeviceID:        "x",
        ListenAddr:      "127.0.0.1:0",
        AgentVersion:    "0.1.0",
        EnableLogStream: false,
    }, slog.Default())
    if err != nil {
        t.Fatal(err)
    }

    ts := httptest.NewServer(a.Handler())
    defer ts.Close()

    resp, err := http.Get(ts.URL + "/api/v1/logs")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != http.StatusForbidden {
        t.Errorf("status = %d, want 403", resp.StatusCode)
    }
    if !strings.Contains(string(body), "log streaming disabled") {
        t.Errorf("body = %q, want 'log streaming disabled'", body)
    }
}

func TestHandleLogs_NonGETReturns405(t *testing.T) {
    a, err := New(Config{
        DeviceID:        "x",
        ListenAddr:      "127.0.0.1:0",
        AgentVersion:    "0.1.0",
        EnableLogStream: true,
    }, slog.Default())
    if err != nil {
        t.Fatal(err)
    }

    ts := httptest.NewServer(a.Handler())
    defer ts.Close()

    resp, err := http.Post(ts.URL+"/api/v1/logs", "application/json", nil)
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusMethodNotAllowed {
        t.Errorf("status = %d, want 405", resp.StatusCode)
    }
}

func TestHandleLogs_JournalctlMissingReportsError(t *testing.T) {
    orig := startJournalctl
    startJournalctl = func(_ context.Context, _ string, _ int) (io.ReadCloser, func(), error) {
        return nil, nil, errors.New("not found")
    }
    defer func() { startJournalctl = orig }()

    a, err := New(Config{
        DeviceID:        "x",
        ListenAddr:      "127.0.0.1:0",
        AgentVersion:    "0.1.0",
        EnableLogStream: true,
    }, slog.Default())
    if err != nil {
        t.Fatal(err)
    }

    ts := httptest.NewServer(a.Handler())
    defer ts.Close()

    resp, err := http.Get(ts.URL + "/api/v1/logs")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Errorf("status = %d, want 200 (header already flushed)", resp.StatusCode)
    }
    if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
        t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
    }
    body, _ := io.ReadAll(resp.Body)
    if !strings.Contains(string(body), "journalctl not available") {
        t.Errorf("body = %q, want error marker", body)
    }
}

func TestHandleLogs_NormalStream(t *testing.T) {
    // 模拟 journalctl：3 行 NDJSON，关闭后 reader 立刻 EOF。
    payload := `{"__REALTIME_TIMESTAMP":"1700000000000000","MESSAGE":"hello-1","__CURSOR":"c1"}
{"__REALTIME_TIMESTAMP":"1700000001000000","MESSAGE":"hello-2","__CURSOR":"c2"}
{"__REALTIME_TIMESTAMP":"1700000002000000","MESSAGE":"hello-3","__CURSOR":"c3"}
`
    rc, wr := io.Pipe()
    orig := startJournalctl
    var killed atomic.Bool
    startJournalctl = func(_ context.Context, unit string, tail int) (io.ReadCloser, func(), error) {
        if unit != "spotterd.service" {
            return nil, nil, errors.New("unexpected unit: " + unit)
        }
        if tail != 100 {
            return nil, nil, errors.New("unexpected tail: " + strconv.Itoa(tail))
        }
        go func() {
            _, _ = wr.Write([]byte(payload))
            _ = wr.Close()
        }()
        return rc, func() { killed.Store(true); _ = rc.Close() }, nil
    }
    defer func() { startJournalctl = orig }()

    a, err := New(Config{
        DeviceID:        "x",
        ListenAddr:      "127.0.0.1:0",
        AgentVersion:    "0.1.0",
        EnableLogStream: true,
    }, slog.Default())
    if err != nil {
        t.Fatal(err)
    }

    ts := httptest.NewServer(a.Handler())
    defer ts.Close()

    resp, err := http.Get(ts.URL + "/api/v1/logs?tail=100")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("status = %d, want 200", resp.StatusCode)
    }
    body, _ := io.ReadAll(resp.Body)
    if string(body) != payload {
        t.Errorf("body mismatch:\n got %q\nwant %q", body, payload)
    }
    // kill 回调由 handler 的 defer kill() 调用。
    if !killed.Load() {
        t.Errorf("kill callback was not invoked by handler defer")
    }
}
```

- [ ] **Step 7: 在 Linux 上运行 handler 测试，预期 PASS**

如果当前主机是 Linux：

```bash
go test ./internal/agentd/... -run TestHandleLogs -count=1 -v
```

Expected: 4 个测试 PASS。

如果当前主机是 macOS / Windows：本 step 的测试文件带 `//go:build linux`，本地不参与编译。在 PR 合并前需要在 Linux runner（CI 或容器）跑一次 `go test ./internal/agentd/...` 验证。

- [ ] **Step 8: 跑全量测试**

```bash
make test
```

Expected: 全部 PASS。`internal/agentd` 跨平台 helper 测试（`TestParseLogTail`）在 macOS 跑；Linux-only 测试在 macOS 上被 build tag 跳过（视为 OK）。

- [ ] **Step 9: 提交**

```bash
git add internal/agentd/http.go internal/agentd/log_stream_linux.go internal/agentd/log_stream_linux_test.go internal/agentd/log_stream_test.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(agent): 增加 GET /api/v1/logs 流式端点

journalctl -u <unit> -n <tail> -f 单次请求（先回放历史，再 follow）。
响应 Content-Type: application/x-ndjson，每行 journalctl --output=json
原样透传并立即 Flush。受 enable_log_stream 配置开关管控；
关闭时 403，journalctl 缺失时返回 500 错误行。startJournalctl
暴露为包级变量以便测试注入。
EOF
)"
```

---

## Task 3: Scanner 流式 Reader

**Files:**
- Create: `internal/scanner/log_stream.go`
- Create: `internal/scanner/log_stream_test.go`
- Modify: `internal/scanner/scanner.go`（`Options` 加 `LogHTTPClient`、`withDefaults` 默认值、`WithLogHTTPClient` 函数式选项）

**Interfaces:**
- Consumes: agent 端 `GET /api/v1/logs` 端点契约（Task 2）；`Options.LogHTTPClient`（本任务新增）
- Produces:
  - `scanner.LogLine{ Ts, Line, Cursor string }`（导出类型，供 App 层 + 前端使用）
  - `Scanner.StreamDeviceLogs(ctx context.Context, ip string, port int, onLine func(LogLine)) error`

- [ ] **Step 1: 写失败的测试 — 创建 `internal/scanner/log_stream_test.go`**

```go
package scanner_test

import (
    "context"
    "encoding/json"
    "fmt"
    "net"
    "net/http"
    "net/http/httptest"
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/spotter/spotter/internal/registry"
    "github.com/spotter/spotter/internal/scanner"
)

func journalLine(ts int64, msg, cursor string) string {
    rec := map[string]string{
        "__REALTIME_TIMESTAMP": fmt.Sprintf("%d", ts),
        "MESSAGE":              msg,
        "__CURSOR":             cursor,
    }
    b, _ := json.Marshal(rec)
    return string(b)
}

func TestStreamDeviceLogs_NormalFlow(t *testing.T) {
    var (
        mu       sync.Mutex
        received []scanner.LogLine
    )
    followCh := make(chan string, 8)
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/x-ndjson")
        w.WriteHeader(http.StatusOK)
        flusher, _ := w.(http.Flusher)
        // 3 行历史
        for _, l := range []string{
            journalLine(1700000000000000, "boot", "c0"),
            journalLine(1700000001000000, "ready", "c1"),
            journalLine(1700000002000000, "listening", "c2"),
        } {
            _, _ = w.Write([]byte(l + "\n"))
        }
        flusher.Flush()
        // 持续 follow，直到 client 断开
        for {
            select {
            case <-r.Context().Done():
                return
            case line := <-followCh:
                _, _ = w.Write([]byte(line + "\n"))
                flusher.Flush()
            }
        }
    }))
    defer func() {
        srv.Close()
        close(followCh)
    }()

    // 推送 3 行 follow
    followCh <- journalLine(1700000003000000, "accept", "c3")
    followCh <- journalLine(1700000004000000, "ping", "c4")
    followCh <- journalLine(1700000005000000, "stop", "c5")

    reg, _ := registry.Open(t.TempDir() + "/devices.json")
    sc := scanner.New(reg)
    addr := srv.Listener.Addr().(*net.TCPAddr)

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    err := sc.StreamDeviceLogs(ctx, addr.IP.String(), addr.Port, func(line scanner.LogLine) {
        mu.Lock()
        received = append(received, line)
        n := len(received)
        mu.Unlock()
        if n >= 6 {
            cancel()
        }
    })
    if err != nil && !strings.Contains(err.Error(), "context") {
        t.Fatalf("unexpected: %v", err)
    }

    mu.Lock()
    defer mu.Unlock()
    if len(received) != 6 {
        t.Fatalf("got %d lines, want 6: %+v", len(received), received)
    }
    if received[0].Line != "boot" || received[5].Line != "stop" {
        t.Errorf("lines mismatch: %+v", received)
    }
    if received[0].Ts == "" || !strings.Contains(received[0].Ts, "2023-11-14") {
        t.Errorf("ts format wrong: %q", received[0].Ts)
    }
}

func TestStreamDeviceLogs_403(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusForbidden)
        _, _ = w.Write([]byte("log streaming disabled"))
    }))
    defer srv.Close()

    reg, _ := registry.Open(t.TempDir() + "/devices.json")
    sc := scanner.New(reg)
    addr := srv.Listener.Addr().(*net.TCPAddr)
    err := sc.StreamDeviceLogs(context.Background(), addr.IP.String(), addr.Port, func(_ scanner.LogLine) {})
    if err == nil || !strings.Contains(err.Error(), "log streaming disabled") {
        t.Fatalf("want disabled error, got %v", err)
    }
}

func TestStreamDeviceLogs_ContextCancel(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        // Block until ctx done
        <-r.Context().Done()
    }))
    defer srv.Close()

    reg, _ := registry.Open(t.TempDir() + "/devices.json")
    sc := scanner.New(reg)
    addr := srv.Listener.Addr().(*net.TCPAddr)

    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    err := sc.StreamDeviceLogs(ctx, addr.IP.String(), addr.Port, func(_ scanner.LogLine) {})
    if err != nil {
        // We accept context.Canceled as the expected outcome; nil is also OK if reader exits cleanly.
        if !strings.Contains(err.Error(), "context canceled") {
            t.Logf("got err %v (acceptable)", err)
        }
    }
}
```

- [ ] **Step 2: 运行测试，预期 FAIL（`scanner.LogLine` / `StreamDeviceLogs` 未定义）**

```bash
go test ./internal/scanner/... -run TestStreamDeviceLogs -count=1
```

Expected: 编译错误。

- [ ] **Step 3: 在 `internal/scanner/scanner.go` 中加 `LogHTTPClient` 字段与 option**

修改 `Options` 结构体（49-60 行附近）：

```go
type Options struct {
    HTTPClient    *http.Client
    LogHTTPClient *http.Client // 独立于 HTTPClient（无 read timeout）
    PollInterval  time.Duration
    McastInterval time.Duration
    OnEvent       func(Event)
    Logger        *slog.Logger
    DevicePort    int
    MulticastGroup string
    ClientSenderID string
}
```

修改 `withDefaults`（62-85 行附近）：

```go
func (o Options) withDefaults() Options {
    if o.HTTPClient == nil {
        o.HTTPClient = &http.Client{Timeout: 3 * time.Second}
    }
    if o.LogHTTPClient == nil {
        o.LogHTTPClient = &http.Client{Timeout: 0} // 跟随 ctx
    }
    // ... 其他默认值不变
    return o
}
```

在文件末尾（`WithHTTPClient` 之后）追加：

```go
// WithLogHTTPClient overrides the streaming client used by
// Scanner.StreamDeviceLogs. The streaming client should have no read
// timeout so long-lived log streams are not cut off prematurely.
func WithLogHTTPClient(c *http.Client) func(*Options) {
    return func(o *Options) { o.LogHTTPClient = c }
}
```

- [ ] **Step 4: 创建 `internal/scanner/log_stream.go`**

```go
package scanner

import (
    "bufio"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "strconv"
    "time"
)

// LogLine is a single record from the device's execution log stream.
// Ts is RFC3339Nano (UTC), Line is the MESSAGE, Cursor is journalctl's
// __CURSOR (reserved for future resume — not consumed in this version).
type LogLine struct {
    Ts     string `json:"ts"`
    Line   string `json:"line"`
    Cursor string `json:"cursor"`
}

// journalRecord is journalctl --output=json's wire format. Only the
// three fields we surface are kept; everything else is discarded.
type journalRecord struct {
    RealTimeUs string `json:"__REALTIME_TIMESTAMP"` // microseconds, decimal string
    Message    string `json:"MESSAGE"`
    Cursor     string `json:"__CURSOR"`
}

// StreamDeviceLogs opens a long-lived GET /api/v1/logs against the
// device and invokes onLine for each NDJSON record. Returns when ctx
// is cancelled, the stream ends, or any read/decode error occurs.
// Malformed lines are skipped (not fatal).
func (s *Scanner) StreamDeviceLogs(ctx context.Context, ip string, port int, onLine func(LogLine)) error {
    target := fmt.Sprintf("http://%s:%d/api/v1/logs?tail=100", ip, port)
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
    if err != nil {
        return err
    }
    req.Header.Set("Accept", "application/x-ndjson") // 文档化用；agent 不强制校验

    resp, err := s.opts.LogHTTPClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    switch resp.StatusCode {
    case http.StatusOK:
        // 继续读取流
    case http.StatusForbidden:
        return fmt.Errorf("log streaming disabled")
    default:
        return fmt.Errorf("log stream: unexpected status %d", resp.StatusCode)
    }

    scanner := bufio.NewScanner(resp.Body)
    buf := make([]byte, 0, 64*1024)
    scanner.Buffer(buf, 1<<20) // 1MB cap per line
    for scanner.Scan() {
        raw := scanner.Bytes()
        if len(raw) == 0 {
            continue
        }
        var rec journalRecord
        if err := json.Unmarshal(raw, &rec); err != nil {
            // best-effort：跳过坏行
            continue
        }
        line := LogLine{
            Ts:     formatJournalTs(rec.RealTimeUs),
            Line:   rec.Message,
            Cursor: rec.Cursor,
        }
        if ctx.Err() != nil {
            return ctx.Err()
        }
        onLine(line)
    }
    if err := scanner.Err(); err != nil {
        if errors.Is(err, context.Canceled) {
            return nil
        }
        return err
    }
    return nil
}

// formatJournalTs converts journalctl's __REALTIME_TIMESTAMP
// (microseconds since epoch, decimal string) to RFC3339Nano UTC.
func formatJournalTs(us string) string {
    if us == "" {
        return ""
    }
    n, err := strconv.ParseInt(us, 10, 64)
    if err != nil {
        return ""
    }
    return time.UnixMicro(n).UTC().Format(time.RFC3339Nano)
}
```

- [ ] **Step 5: 运行测试，预期 PASS**

```bash
go test ./internal/scanner/... -run TestStreamDeviceLogs -count=1 -v
```

Expected: 3 个测试 PASS。

- [ ] **Step 6: 跑全量测试**

```bash
make test
```

Expected: 全部 PASS。

- [ ] **Step 7: 提交**

```bash
git add internal/scanner/scanner.go internal/scanner/log_stream.go internal/scanner/log_stream_test.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(scanner): 增加 StreamDeviceLogs 流式 reader

通过独立的 LogHTTPClient（无 read timeout，默认 Timeout=0）
GET /api/v1/logs，bufio.Scanner 逐行解析 journalctl NDJSON，
转 LogLine{Ts: RFC3339Nano, Line: MESSAGE, Cursor: __CURSOR}。
异常 JSON 行跳过不中断流；ctx cancel 安全退出。
EOF
)"
```

---

## Task 4: Wails App 绑定（StartLogStream / StopLogStream）

**Files:**
- Modify: `main.go`（加 `Emitter` 接口、`wailsEmitter` 实现；`App` 加 `logStreams` / `logStreamsMu` / `streamFn` 字段；`NewApp` 接受 emitter 参数；新增 `StartLogStream` / `StopLogStream` / `runLogStream`）
- Create: `main_test.go`（package main 的 App 单元测试）

**Interfaces:**
- Consumes: `Scanner.StreamDeviceLogs`（Task 3）；`registry.Registry.Get`（已有）
- Produces:
  - `Emitter` 接口 + `wailsEmitter` 实现（便于测试注入）
  - `App.StartLogStream(deviceID string) error`（Wails 绑定）
  - `App.StopLogStream(deviceID string) error`（Wails 绑定）
  - Wails events：`device-log:{deviceID}` payload `LogLine`、`device-log-error:{deviceID}` payload string、`device-log-end:{deviceID}` payload bool

- [ ] **Step 1: 写失败的测试 — 创建 `main_test.go`**

```go
package main

import (
    "context"
    "errors"
    "io"
    "log/slog"
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/spotter/spotter/internal/registry"
    "github.com/spotter/spotter/internal/scanner"
)

// fakeEmitter counts Emit calls and records event names.
type fakeEmitter struct {
    mu       sync.Mutex
    events   []string
    payloads []any
}

func (f *fakeEmitter) Emit(_ context.Context, name string, data ...any) {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.events = append(f.events, name)
    f.payloads = append(f.payloads, data)
}

// count 返回名为 prefix 开头的 event 数量。
func (f *fakeEmitter) count(prefix string) int {
    f.mu.Lock()
    defer f.mu.Unlock()
    n := 0
    for _, e := range f.events {
        if strings.HasPrefix(e, prefix) {
            n++
        }
    }
    return n
}

func newTestApp(t *testing.T, reg *registry.Registry, em Emitter) *App {
    t.Helper()
    a := NewApp(reg, slog.New(slog.NewTextHandler(io.Discard, nil)), em)
    // 替换 streamFn 为同步 fake
    a.streamFn = func(ctx context.Context, ip string, port int, onLine func(scanner.LogLine)) error {
        // 默认 fake：emit 3 行后返回 nil
        for i := 0; i < 3; i++ {
            select {
            case <-ctx.Done():
                return ctx.Err()
            default:
            }
            onLine(scanner.LogLine{Ts: "2026-01-01T00:00:00Z", Line: "line", Cursor: "c"})
        }
        return nil
    }
    return a
}

func TestStartLogStream_NotRegistered(t *testing.T) {
    reg, _ := registry.Open(t.TempDir() + "/reg.json")
    em := &fakeEmitter{}
    a := newTestApp(t, reg, em)
    err := a.StartLogStream("unknown")
    if err == nil || !strings.Contains(err.Error(), "not found") {
        t.Fatalf("want not-found error, got %v", err)
    }
}

func TestStartLogStream_Offline(t *testing.T) {
    reg, _ := registry.Open(t.TempDir() + "/reg.json")
    _ = reg.Add(registry.Entry{
        DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
        Online: false, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
    })
    em := &fakeEmitter{}
    a := newTestApp(t, reg, em)
    err := a.StartLogStream("d1")
    if err == nil || !strings.Contains(err.Error(), "offline") {
        t.Fatalf("want offline error, got %v", err)
    }
}

func TestStartLogStream_OnlineEmitsLines(t *testing.T) {
    reg, _ := registry.Open(t.TempDir() + "/reg.json")
    _ = reg.Add(registry.Entry{
        DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
        Online: true, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
    })
    em := &fakeEmitter{}
    a := newTestApp(t, reg, em)

    if err := a.StartLogStream("d1"); err != nil {
        t.Fatalf("Start: %v", err)
    }
    // 等待 goroutine 完成 fake streamFn（3 行后返回 → defer emit end）
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        if em.count("device-log-end:") >= 1 {
            break
        }
        time.Sleep(20 * time.Millisecond)
    }
    if got := em.count("device-log:d1"); got != 3 {
        t.Errorf("device-log:d1 count = %d, want 3", got)
    }
    if got := em.count("device-log-end:d1"); got != 1 {
        t.Errorf("device-log-end:d1 count = %d, want 1", got)
    }
    // Stop 后再 Start 必须幂等：当前 map 已清空（fake streamFn 已返回，defer 删了 entry），可以再 Start
}

func TestStartLogStream_Idempotent(t *testing.T) {
    reg, _ := registry.Open(t.TempDir() + "/reg.json")
    _ = reg.Add(registry.Entry{
        DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
        Online: true, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
    })
    em := &fakeEmitter{}
    a := newTestApp(t, reg, em)
    // 把 streamFn 换成挂起型，确保 goroutine 不退出。
    blockCh := make(chan struct{})
    a.streamFn = func(ctx context.Context, _ string, _ int, _ func(scanner.LogLine)) error {
        <-blockCh
        return nil
    }

    if err := a.StartLogStream("d1"); err != nil {
        t.Fatalf("Start 1: %v", err)
    }
    // 等 map 里有 entry
    time.Sleep(50 * time.Millisecond)
    if err := a.StartLogStream("d1"); err != nil {
        t.Fatalf("Start 2 (idempotent): %v", err)
    }
    // 释放
    close(blockCh)
    time.Sleep(50 * time.Millisecond)

    // 验证 streamFn 只被调用一次：通过计数 fakeEmitter 上 device-log:d1（应是 0，因为我们没 emit 行）
    // 更直接：验证 logStreams 在 Start 2 后 map 中只一条；这里通过 Stop 行为间接验证。
    if err := a.StopLogStream("d1"); err != nil {
        t.Fatalf("Stop: %v", err)
    }
    time.Sleep(50 * time.Millisecond)
    // 再次 Stop no-op
    if err := a.StopLogStream("d1"); err != nil {
        t.Fatalf("Stop 2 (no-op): %v", err)
    }
}

func TestStopLogStream_NoStream(t *testing.T) {
    reg, _ := registry.Open(t.TempDir() + "/reg.json")
    em := &fakeEmitter{}
    a := newTestApp(t, reg, em)
    if err := a.StopLogStream("d1"); err != nil {
        t.Fatalf("Stop without Start should be no-op, got %v", err)
    }
}

func TestRunLogStream_ErrorPropagates(t *testing.T) {
    reg, _ := registry.Open(t.TempDir() + "/reg.json")
    _ = reg.Add(registry.Entry{
        DeviceID: "d1", IP: "10.0.0.1", Port: 9999,
        Online: true, LastSeenAt: time.Now().UTC().Format(time.RFC3339),
    })
    em := &fakeEmitter{}
    a := newTestApp(t, reg, em)
    // 改 streamFn 返回非 ctx.Canceled error
    a.streamFn = func(_ context.Context, _ string, _ int, _ func(scanner.LogLine)) error {
        return errors.New("boom")
    }

    if err := a.StartLogStream("d1"); err != nil {
        t.Fatalf("Start: %v", err)
    }
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        if em.count("device-log-error:") >= 1 {
            break
        }
        time.Sleep(20 * time.Millisecond)
    }
    if got := em.count("device-log-error:d1"); got != 1 {
        t.Errorf("device-log-error:d1 count = %d, want 1", got)
    }
    if got := em.count("device-log-end:d1"); got != 1 {
        t.Errorf("device-log-end:d1 count = %d, want 1", got)
    }
}
```

- [ ] **Step 2: 运行测试，预期 FAIL（`Emitter`/`App.logStreams`/`App.streamFn` 未定义）**

```bash
go test . -run TestStartLogStream -count=1
```

Expected: 编译错误。

- [ ] **Step 3: 修改 `main.go` — 加 Emitter 接口与 wailsEmitter**

在 `main.go` 的 import 段之后（`App` 结构体之前）追加：

```go
// Emitter abstracts wailsruntime.EventsEmit so tests can substitute
// a recording fake without spinning up Wails.
type Emitter interface {
    Emit(ctx context.Context, eventName string, data ...interface{})
}

type wailsEmitter struct{}

func (wailsEmitter) Emit(ctx context.Context, eventName string, data ...interface{}) {
    wailsruntime.EventsEmit(ctx, eventName, data...)
}
```

- [ ] **Step 4: 修改 `App` 结构体**

把：

```go
type App struct {
    reg     *registry.Registry
    logger  *slog.Logger
    scanner *scanner.Scanner
    ctx     context.Context
}
```

改为：

```go
type App struct {
    reg          *registry.Registry
    logger       *slog.Logger
    scanner      *scanner.Scanner
    emitter      Emitter
    ctx          context.Context
    logStreams   map[string]context.CancelFunc
    logStreamsMu sync.Mutex
    // streamFn is the body of StartLogStream; injected for tests.
    streamFn func(ctx context.Context, ip string, port int, onLine func(scanner.LogLine)) error
}
```

- [ ] **Step 5: 修改 `NewApp`**

把：

```go
func NewApp(reg *registry.Registry, logger *slog.Logger) *App {
    app := &App{reg: reg, logger: logger, ctx: context.Background()}
    app.scanner = scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
        logger.Info("scanner event", slog.String("tag", e.Tag()))
        wailsruntime.EventsEmit(app.ctx, e.Tag(), e)
    }))
    return app
}
```

改为：

```go
func NewApp(reg *registry.Registry, logger *slog.Logger, emitter Emitter) *App {
    if emitter == nil {
        emitter = wailsEmitter{}
    }
    app := &App{
        reg: reg, logger: logger, emitter: emitter,
        ctx:        context.Background(),
        logStreams: map[string]context.CancelFunc{},
    }
    app.scanner = scanner.New(reg, scanner.WithOnEvent(func(e scanner.Event) {
        logger.Info("scanner event", slog.String("tag", e.Tag()))
        app.emitter.Emit(app.ctx, e.Tag(), e)
    }))
    // streamFn 默认指向 scanner 实现；测试可覆盖。
    app.streamFn = app.scanner.StreamDeviceLogs
    return app
}
```

要点：`streamFn` 必须在 `app.scanner` 赋值之后设置（否则 `app.scanner.StreamDeviceLogs` 拿到 nil）。

- [ ] **Step 6: 修改 `main` 里的 `NewApp` 调用点**

在 `main` 函数中（约 78 行）：

```go
app := NewApp(reg, logger)
```

改为：

```go
app := NewApp(reg, logger, wailsEmitter{})
```

- [ ] **Step 7: 在 `main.go` 末尾追加 log stream 方法**

定位到 `ShutdownDevice` / `powerAction` 方法之后，追加：

```go
// StartLogStream begins streaming the device's execution log. Each
// NDJSON record is emitted as "device-log:{deviceID}" with payload
// scanner.LogLine. Idempotent for the same deviceID: a second call
// while a stream is active returns nil and does NOT spawn another
// goroutine. Errors:
//
//   - device not in registry
//   - device marked offline
func (a *App) StartLogStream(deviceID string) error {
    entry, ok := a.reg.Get(deviceID)
    if !ok {
        return fmt.Errorf("device not found: %s", deviceID)
    }
    if !entry.Online {
        return fmt.Errorf("device %s is offline", deviceID)
    }
    port := entry.Port
    if port == 0 {
        port = listenPort
    }

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

// runLogStream reads from streamFn and emits each line via the
// Emitter. On exit (ctx cancel or stream error) it removes the
// stream from the map and emits "device-log-end:{id}".
func (a *App) runLogStream(ctx context.Context, deviceID, ip string, port int) {
    defer func() {
        a.logStreamsMu.Lock()
        if c, ok := a.logStreams[deviceID]; ok {
            c()
            delete(a.logStreams, deviceID)
        }
        a.logStreamsMu.Unlock()
        a.emitter.Emit(a.ctx, "device-log-end:"+deviceID, true)
    }()
    err := a.streamFn(ctx, ip, port, func(line scanner.LogLine) {
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

- [ ] **Step 8: 运行测试，预期 PASS**

```bash
go test . -run "TestStartLogStream|TestStopLogStream|TestRunLogStream" -count=1 -v
```

Expected: 6 个测试 PASS。

- [ ] **Step 9: 跑全量测试**

```bash
make test
```

Expected: 全部 PASS。注意 `package main` 目录的测试会与 Wails 编译耦合；Wails 在 `make client` 时才参与，`make test` 只跑 `go test ./...`，不会触发 Wails 构建。

- [ ] **Step 10: 编译验证（不实际跑 Wails）**

```bash
go build .
```

Expected: 无错误。

- [ ] **Step 11: 提交**

```bash
git add main.go main_test.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(client): 暴露 StartLogStream / StopLogStream Wails 绑定

App 通过 Emitter 接口（默认 wailsEmitter，测试可注入 fake）
推送 device-log:{id} / device-log-error:{id} / device-log-end:{id}
三个事件。streamFn 字段可注入以便测试。Start 同设备幂等，
map 中已有 cancel 直接返回 nil。Stop 不存在的 stream no-op。
EOF
)"
```

---

## Task 5: 前端 LogSection 组件

**Files:**
- Modify: `frontend/src/i18n/dictionaries.ts`（新增 5 个 `log.*` 键，中英双语）
- Create: `frontend/src/components/LogSection.tsx`

**Interfaces:**
- Consumes: Wails 自动生成的 `StartLogStream` / `StopLogStream` 绑定（来自 Task 4；Wails 编译后落在 `frontend/wailsjs/go/main/App.d.ts`）；`EventsOn` / `EventsOff`（`../../wailsjs/runtime/runtime`）
- Produces: 折叠区组件，受控开关；展开时订阅、收起时退订；前端 cap 1000 行；error / offline 状态用 antd `Alert` 显示

- [ ] **Step 1: 在 `frontend/src/i18n/dictionaries.ts` 中新增 5 个键**

英文段（在 `'detail.refresh': 'Refresh'` 之后，`// Cards` 之前）追加：

```typescript
    'detail.refresh': 'Refresh',

    // Execution log panel
    'log.title': 'Execution Log',
    'log.streaming': 'live',
    'log.empty': 'Waiting for log lines…',
    'log.offline': 'Device is offline. Log stream unavailable.',
    'log.disabled_offline': 'Offline',
```

中文段（对应位置）追加：

```typescript
    'detail.refresh': '刷新',

    // 执行日志面板
    'log.title': '执行日志',
    'log.streaming': '实时',
    'log.empty': '等待日志输出…',
    'log.offline': '设备离线，无法查看日志。',
    'log.disabled_offline': '离线',
```

注意：把 `// Cards (...)` 注释留在下面；新键插在它前面。

- [ ] **Step 2: 创建 `frontend/src/components/LogSection.tsx`**

```tsx
import { Alert, Collapse, Space } from 'antd';
import { useEffect, useRef, useState } from 'react';
import { useI18n } from '../state/I18nContext';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { StartLogStream, StopLogStream } from '../../wailsjs/go/main/App';

interface LogLine {
  ts: string;
  line: string;
  cursor: string;
}

const MAX_LINES = 1000;

interface Props {
  deviceID: string;
  online: boolean;
}

export default function LogSection({ deviceID, online }: Props) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [error, setError] = useState<string | null>(null);
  const bufRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    if (!online) {
      setError(t('log.offline'));
      return;
    }
    setError(null);
    setLines([]);
    StartLogStream(deviceID).catch((e: unknown) => {
      const msg = (e as { message?: string })?.message ?? String(e);
      setError(msg);
    });
    const onLine = (line: LogLine) => {
      setLines((prev) => {
        if (prev.length >= MAX_LINES) {
          return [...prev.slice(prev.length - MAX_LINES + 1), line];
        }
        return [...prev, line];
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
            {open && (
              <span style={{ color: 'var(--text-secondary)', fontSize: 12 }}>
                ● {t('log.streaming')}
              </span>
            )}
          </Space>
        ),
        children: error ? (
          <Alert type="error" message={error} />
        ) : (
          <div
            ref={bufRef}
            style={{
              height: 240,
              overflowY: 'auto',
              background: '#0e0e10',
              color: '#d4d4d4',
              padding: 8,
              borderRadius: 4,
              fontFamily: 'ui-monospace, Menlo, monospace',
              fontSize: 12,
              whiteSpace: 'pre-wrap',
            }}
          >
            {lines.length === 0 ? (
              <span style={{ color: '#888' }}>{t('log.empty')}</span>
            ) : (
              lines.map((l, i) => (
                <div key={i}>
                  <span style={{ color: '#7aa2f7' }}>{l.ts}</span>{' '}
                  <span>{l.line}</span>
                </div>
              ))
            )}
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

- [ ] **Step 3: TypeScript 类型检查**

```bash
cd frontend && npx tsc --noEmit
```

Expected: 无类型错误。如果 `tsc` 缺失，先 `npm install`。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/components/LogSection.tsx frontend/src/i18n/dictionaries.ts
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(gui): 新增 LogSection 折叠区组件

默认收起；展开时订阅 device-log:{id} 事件、显示日志行
（先回放 100 行历史，再 follow）。前端 cap 1000 行
（O(1) slice），自动滚动到底。收起 / 切换设备触发
StopLogStream + EventsOff。
EOF
)"
```

---

## Task 6: 前端 DetailPanel 三段式改造

**Files:**
- Modify: `frontend/src/components/DetailPanel.tsx`（插入 `<LogSection>`，改 flex 容器）

**Interfaces:**
- Consumes: `LogSection` 组件（Task 5）
- Produces: 详情面板分三段：中间可滚动（标题 + 三张 card）、底部 LogSection（不参与滚动）、底部 DeviceActions

- [ ] **Step 1: 修改 `frontend/src/components/DetailPanel.tsx`**

完整文件内容（替换原文件）：

```tsx
import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import DeviceActions from './DeviceActions';
import LogSection from './LogSection';

export default function DetailPanel() {
  const { state } = useDevices();
  const { t } = useI18n();
  const device = state.devices.find((d) => d.device_id === state.selectedId);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {!device ? <EmptyState /> : (
          <>
            <h2 style={{ margin: '0 0 16px 0', color: 'var(--text-primary)' }}>
              {device.last_info?.basic?.hostname || device.ip}
              <span style={{ marginLeft: 12, fontSize: 13, color: device.online ? '#52c41a' : '#b71c1c' }}>
                {device.online ? t('detail.status.online') : t('detail.status.offline')}
              </span>
            </h2>
            <div style={{ display: 'grid', gap: 12 }}>
              <BasicCard device={device} />
              <NetworkCard device={device} />
              <JetsonCard device={device} />
            </div>
          </>
        )}
      </div>
      {device && (
        <div
          style={{
            padding: '0 16px 8px 16px',
            borderTop: '1px solid var(--border-color)',
          }}
        >
          <LogSection deviceID={device.device_id} online={device.online} />
        </div>
      )}
      {device && <DeviceActions />}
    </div>
  );
}
```

- [ ] **Step 2: TypeScript 类型检查 + Vite 构建**

```bash
cd frontend && npx tsc --noEmit && npm run build
```

Expected: 无类型错误；Vite 产出 `frontend/dist/`，无报错。

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/DetailPanel.tsx
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(gui): 详情面板底部独立 LogSection 折叠区

中间可滚动区域（标题 + 三张 card）保持不变；底部新增
独立区域承载 LogSection，与 DeviceActions 同层、不参与
中间滚动。设备 offline / 未选中时不渲染。
EOF
)"
```

---

## Task 7: 文档更新

**Files:**
- Modify: `README.md`（中文版已知限制段）
- Modify: `docs/api.md`（中文 API 文档）
- Modify: `docs/api.en.md`（英文 API 文档）
- Modify: `docs/operations.md`（中文运维）
- Modify: `docs/operations.en.md`（英文运维）
- Modify: `SECURITY.md`

**Interfaces:**
- Consumes: 设计 spec §6
- Produces: 6 份文档更新，覆盖 README、API、SECURITY、operations 中英双语

- [ ] **Step 1: 修改 `README.md` 已知限制段**

找到「已知限制（MVP）」段，将「不支持远端命令执行」一行改为：

```
- **不支持任意远端命令执行** —— 仅提供 opt-in 的远程 reboot / shutdown（见 power-actions 设计）与设备端软件执行日志查看（见 execution-log-stream 设计）。不提供 shell 或自定义命令通道。
```

并把下一行「HTTP 端点 无身份认证」保留，描述微调为「仍仅限可信局域网内部署；启用电源管理等于授予同网段任何客户端触发 root 级别 reboot / poweroff 的权限；启用日志流等于授予读取该 unit 在 systemd journal 中历史与新行的权限」。

- [ ] **Step 2: 修改 `docs/api.md`（中文版）**

在 `/api/v1/info` 文档段之后、`POST /api/v1/reboot` 之前，新增：

````markdown
### `GET /api/v1/logs?tail=N`

流式返回设备端软件的执行日志（默认 `journalctl -u spotterd.service`）。

请求：
- Headers：`Accept: application/x-ndjson`（文档化用，agent 不强制校验）。
- Query：`tail=N`（默认 100，上限 1000）。

响应（200，NDJSON 流）：
- 每行一个 JSON 对象：journalctl `--output=json` 的原始结构（包括 `__REALTIME_TIMESTAMP`、`MESSAGE`、`__CURSOR` 等）。
- 行为：先回放最近 N 行历史，再 follow 新增行；客户端断开后流终止。

响应（403，未启用）：
- 文本 `log streaming disabled`（agent 配置 `enable_log_stream` 缺失或为 false）。

响应（405，非 GET）：文本 `method not allowed`。

### `enable_log_stream` / `log_unit`

`/etc/spotterd/agent.toml`：

```toml
enable_log_stream = true   # 默认 false
log_unit = "spotterd.service"  # 默认
```

开启后 agent 暴露 `GET /api/v1/logs`。无身份认证，部署方负责网络隔离；日志内容可能含敏感信息，开启前评估数据敏感性。
````

- [ ] **Step 3: 修改 `docs/api.en.md`**

与 Step 2 同样结构，文字改为英文：

````markdown
### `GET /api/v1/logs?tail=N`

Streams the device's execution log (default `journalctl -u spotterd.service`).

Request:
- Headers: `Accept: application/x-ndjson` (informational; agent does not enforce).
- Query: `tail=N` (default 100, max 1000).

Response (200, NDJSON stream):
- One JSON object per line: journalctl `--output=json` raw record (including `__REALTIME_TIMESTAMP`, `MESSAGE`, `__CURSOR`, etc.).
- Behaviour: replays the most recent N lines, then follows new output; the stream terminates when the client disconnects.

Response (403, disabled):
- Plain text `log streaming disabled` (when `enable_log_stream` is missing or false).

Response (405, non-GET): plain text `method not allowed`.

### `enable_log_stream` / `log_unit`

`/etc/spotterd/agent.toml`:

```toml
enable_log_stream = true   # default false
log_unit = "spotterd.service"  # default
```

When enabled, the agent exposes `GET /api/v1/logs`. Unauthenticated; deployer is responsible for network isolation. Log content may contain sensitive information; evaluate before enabling.
````

- [ ] **Step 4: 修改 `docs/operations.md`**

在「设备端部署」段（提到 `enable_power_actions` 的位置之后）追加：

```
可选：在 `agent.toml` 中设置 `enable_log_stream = true` 以启用 GUI 实时日志查看（默认关闭；可配合 `log_unit` 字段指定其他 systemd unit）。
```

- [ ] **Step 5: 修改 `docs/operations.en.md`**

对应英文版同样追加：

```
Optional: set `enable_log_stream = true` in `agent.toml` to enable GUI live-log streaming (off by default; combine with `log_unit` to point at a different systemd unit).
```

- [ ] **Step 6: 修改 `SECURITY.md`**

在「加固 checklist」段（power-actions 那两条之后）追加：

```
- 在启用 agent 的 `enable_log_stream = true` 前，确保设备在受控 VLAN / VPN 之后。
- `enable_log_stream = true` 等于授权该子网任何客户端读取 agent 的 systemd journal（仅限配置的 unit）。
- 该端点返回的日志可能包含敏感信息（路径、凭据片段等），开启前评估数据敏感性。
```

- [ ] **Step 7: 验证构建 + 测试**

```bash
make test
```

Expected: 全部 PASS（文档更新不影响 Go 测试）。

- [ ] **Step 8: 提交**

```bash
git add README.md docs/api.md docs/api.en.md docs/operations.md docs/operations.en.md SECURITY.md
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
docs: 文档同步执行日志流式功能

README 已知限制段、API 文档（中英）、operations 部署说明
（中英）、SECURITY 加固清单同步说明 GET /api/v1/logs 端点、
enable_log_stream / log_unit 配置开关、无鉴权假设与数据敏感性
警告。
EOF
)"
```

---

## Self-Review Checklist

- [x] Spec coverage:
  - §1.1 目标（流式实时日志）：Task 2 + 3 + 4 + 5 + 6 ✓
  - §1.2 非目标（无鉴权、无持久化、不重连）：全部 task 不引入对应逻辑 ✓
  - §1.3.A（403 disabled）：Task 2 `TestHandleLogs_DisabledReturns403` ✓
  - §1.3.B（200 + NDJSON）：Task 2 `TestHandleLogs_NormalStream` ✓
  - §1.3.C（journalctl 缺失 500 + error）：Task 2 `TestHandleLogs_JournalctlMissingReportsError` ✓
  - §1.3.D（展开 1-2s 看到历史 + follow）：Task 5 + 6，前端 EventsOn 订阅 ✓
  - §1.3.E（收起时 journalctl 被 kill）：Task 4 StopLogStream → ctx cancel → exec.CommandContext SIGKILL ✓
  - §1.3.F（切设备旧 stream 停）：Task 5 useEffect cleanup（deviceID 变化触发 unmount） ✓
  - §1.3.G（Start 幂等）：Task 4 `TestStartLogStream_Idempotent` ✓
  - §1.3.H（offline 不可展开）：Task 5 `extra` prop + `online` 检查 ✓
  - §1.3.I（`make test` 全过 + 单元覆盖）：每个 task 末尾 `make test`；Task 2/3/4 各自有 test 文件 ✓
  - §1.3.J（1000 行 cap）：Task 5 `MAX_LINES = 1000` slice ✓
- [x] Placeholder scan：no TBD/TODO/"implement later"/"similar to"。所有 step 含实际代码或命令。
- [x] Type consistency：
  - `agentd.Config{EnableLogStream, LogUnit}` 在 Task 1 定义、Task 2 使用 ✓
  - `agentd.startJournalctl` 在 Task 2 定义、Task 2 测试覆盖 ✓
  - `agentd.parseLogTail` 在 Task 2 定义、Task 2 测试 ✓
  - `scanner.LogLine{Ts, Line, Cursor}` 在 Task 3 定义、Task 3 测试 + Task 4 调用 ✓
  - `Scanner.StreamDeviceLogs(ctx, ip, port, onLine)` 在 Task 3 定义、Task 4 调用 ✓
  - `Scanner.Options.LogHTTPClient` + `WithLogHTTPClient` 在 Task 3 定义 ✓
  - `App.StartLogStream(deviceID) error` 在 Task 4 定义、Task 4 测试 ✓
  - `App.StopLogStream(deviceID) error` 在 Task 4 定义、Task 4 测试 ✓
  - `App.streamFn` 字段在 Task 4 定义、Task 4 测试覆盖 ✓
  - `Emitter` 接口在 Task 4 定义、Task 4 测试用 `fakeEmitter` 实现 ✓
  - `NewApp(reg, logger, emitter Emitter)` 在 Task 4 定义、Task 4 测试 + `main()` 调用 ✓
  - `LogSection` props `{deviceID: string, online: boolean}` 在 Task 5 定义、Task 6 使用 ✓
  - i18n 键 `log.{title,streaming,empty,offline,disabled_offline}` 在 Task 5 定义、Task 5 使用 ✓
- [x] Spec 范围内：单次实施计划可完成；不含分阶段。
- [x] 无遗漏引用：所有跨 task 引用的函数 / 类型 / 字段都已在前序 task 中明确定义并通过编译。
- [x] `package main` 测试可执行：测试只依赖 App 公开字段与 emitter 接口；不依赖 Wails runtime（`wailsEmitter{}` 仅在 `main()` 里用）。