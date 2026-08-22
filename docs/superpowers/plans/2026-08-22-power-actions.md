# Spotter 远程电源管理（Reboot / Shutdown）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 spotterd 增加 opt-in 的 reboot/shutdown HTTP 端点，并在 spotter-client GUI 详情标题头右侧新增对应按钮（带二次确认）。

**Architecture:** 三层增量 — agent 加 HTTP POST + 配置开关（默认 OFF）；scanner 加 HTTP 调用 client；Wails App 暴露两个绑定；前端 DetailPanel 标题头右侧加两按钮（带 antd Modal 二次确认）。systemd unit 同时引入基础 hardening（`NoNewPrivileges`/`ProtectSystem`/`ProtectHome`/`PrivateTmp`）。

**Tech Stack:** Go 1.25 / `net/http` / `os/exec`；Wails v2；React + antd（`Modal.confirm`、`message`、`Space`）。沿用现有测试栈 `testing` + `httptest`。

## Global Constraints

- 模块路径 `github.com/spotter/spotter`；Go 1.25。
- TOML 配置字段 `enable_power_actions` 默认 `false`，缺失即关闭（沿用现有 tomlConfig 行为）。
- agent 端鉴权保持现状「无鉴权，仅可信局域网」；不引入 token/SSH 凭据。
- 端点仅接受 `POST`；其他方法返回 `405`。
- agent 端响应 403/202/500 均为 `application/json` 格式：`{"error":"..."}` 或 `{"status":"scheduled","action":"..."}`。
- client scanner 沿用 `s.opts.HTTPClient`（默认 3s 超时）；App 绑定包一层 5s context；超时视为「乐观成功」不报错。
- 不新增 device 状态字段（复用现有 online/offline）。
- 提交作者固定 `letmlook <letmlook@aliyun.com>`，通过 `git commit --author=...` 指定，**不要改 git config**。
- commit message 用中文 `[类型] 简述` 格式。
- `make test` 必须全部通过；前端无可用单测框架，依靠手工 e2e 验证清单（见 §5.5 设计 spec）。
- systemd unit hardening 只追加 4 个开关：`NoNewPrivileges=true`、`ProtectSystem=strict`、`ProtectHome=true`、`PrivateTmp=true`，不引入 `ReadOnlyPaths` 或 kernel 限制。
- 所有改动文件清单见 spec §1 头部表格。
- 参考文档：
  - 设计 spec：`docs/superpowers/specs/2026-08-22-power-actions-design.md`
  - 现有 agent 测试：`internal/agentd/agent_test.go`、`internal/agentd/udp_test.go`
  - 现有 scanner 测试：`internal/scanner/scanner_test.go`
  - 现有前端 hook：`frontend/src/hooks/useDeviceActions.ts`
  - 现有 DetailPanel：`frontend/src/components/DetailPanel.tsx`
  - i18n：`frontend/src/i18n/dictionaries.ts`

---

## File Structure

**新增：**
- `internal/agentd/http_test.go` — agent HTTP handler 测试（reboot/shutdown 端点的 403/202/405 三态）

**修改：**
- `internal/agentd/agent.go` — `Config` 加 `EnablePowerActions bool`
- `internal/agentd/http.go` — 加 `POST /api/v1/reboot` 与 `/shutdown` handler，复用 `handlePowerAction`；引入包级 `var execSystemctl` 以便测试注入
- `cmd/agent/main.go` — `tomlConfig` 加字段；透传给 `agentd.New`
- `scripts/spotterd.service` — `[Service]` 段末尾追加 4 行 hardening
- `internal/scanner/scanner.go` — 新增 `RebootDevice` / `ShutdownDevice` / 内部 `postPowerAction` + `errPowerActionTimeout` sentinel
- `internal/scanner/scanner_test.go` — 新增 `TestPostPowerAction_*` 测试覆盖 202/403/500/timeout 四路径
- `main.go` — `App` 加 `RebootDevice(deviceID)` 与 `ShutdownDevice(deviceID)` 绑定
- `frontend/src/hooks/useDeviceActions.ts` — hook 加 `reboot` / `shutdown` 两个方法
- `frontend/src/components/DetailPanel.tsx` — `<h2>` 改为 flex 行，右侧加 `Space` 装两按钮
- `frontend/src/i18n/dictionaries.ts` — 中英双语新增 5 个键
- `README.md` / `docs/api.md` / `docs/api.en.md` / `SECURITY.md` / `docs/operations.md` / `docs/operations.en.md` — 文档更新

---

## Task 1: Agent Config 与 TOML 字段

**Files:**
- Modify: `internal/agentd/agent.go:11-16`（`Config` 结构体）
- Modify: `cmd/agent/main.go:29-34`（`tomlConfig`）、`cmd/agent/main.go:63-69`（`agentd.New` 调用）

**Interfaces:**
- Consumes: 无
- Produces: `agentd.Config{EnablePowerActions: bool}` — 后续 handler 在此字段为 false 时返回 403

- [ ] **Step 1: 在 `agentd.Config` 中加 `EnablePowerActions` 字段**

打开 `internal/agentd/agent.go`，修改 `Config` 结构体（11-16 行附近）：

```go
// Config holds the agent's runtime settings.
type Config struct {
	DeviceID            string
	ListenAddr          string
	MulticastGroup      string
	AgentVersion        string
	EnablePowerActions  bool // opt-in: allow POST /api/v1/reboot & /shutdown
}
```

- [ ] **Step 2: 在 `cmd/agent/main.go` 的 `tomlConfig` 中加同名字段**

修改 `cmd/agent/main.go` 的 `tomlConfig`（29-34 行附近）：

```go
type tomlConfig struct {
	DeviceID            string `toml:"device_id"`
	ListenAddr          string `toml:"listen_addr"`
	MulticastGroup      string `toml:"multicast_group"`
	AgentVersion        string `toml:"agent_version"`
	EnablePowerActions  bool   `toml:"enable_power_actions"`
}
```

并修改 `main.go` 第 63-69 行附近 `agentd.New` 调用，把 `cfg.EnablePowerActions` 加进去：

```go
agent, err := agentd.New(agentd.Config{
	DeviceID:            cfg.DeviceID,
	ListenAddr:          cfg.ListenAddr,
	MulticastGroup:      cfg.MulticastGroup,
	AgentVersion:        cfg.AgentVersion,
	EnablePowerActions:  cfg.EnablePowerActions,
}, log)
```

- [ ] **Step 3: 编译验证**

```bash
make agent
```

Expected: 编译通过，无报错。注意：agent 是 `//go:build linux`，macOS 上需要 `GOOS=linux`：

```bash
CGO_ENABLED=0 GOOS=linux make agent-linux-arm64
```

- [ ] **Step 4: 提交**

```bash
git add internal/agentd/agent.go cmd/agent/main.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(agent): 增加 enable_power_actions 配置开关

为后续 reboot/shutdown 端点提供 opt-in 控制。默认 false；agent.toml 缺失该字段即关闭。
EOF
)"
```

---

## Task 2: Agent HTTP Handler（reboot/shutdown）

**Files:**
- Create: `internal/agentd/http_test.go`
- Modify: `internal/agentd/http.go`（在 `Handler` 注册两个新路由；新增 `handlePowerAction`；引入包级 `execSystemctl` 函数变量）

**Interfaces:**
- Consumes: `agentd.Config.EnablePowerActions`（Task 1）
- Produces:
  - `a.Handler() http.Handler` 新路由：`POST /api/v1/reboot`、`POST /api/v1/shutdown`
  - 包级变量 `var execSystemctl = func(action string) error { ... }`（测试可覆盖）
  - 403 响应：`{"error":"power actions disabled"}`
  - 202 响应：`{"status":"scheduled","action":"reboot"|"shutdown"}`

- [ ] **Step 1: 写失败的测试 — 创建 `internal/agentd/http_test.go`**

文件内容：

```go
package agentd_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/spotter/spotter/internal/agentd"
)

func TestPowerAction_DisabledReturns403(t *testing.T) {
	a, err := agentd.New(agentd.Config{
		DeviceID:           "x",
		ListenAddr:         "127.0.0.1:0",
		AgentVersion:       "0.1.0",
		EnablePowerActions: false,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, action := range []string{"reboot", "shutdown"} {
		resp, err := http.Post(ts.URL+"/api/v1/"+action, "application/json", nil)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403", action, resp.StatusCode)
		}
		var got map[string]string
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: decode: %v", action, err)
		}
		if got["error"] != "power actions disabled" {
			t.Errorf("%s: error=%q, want %q", action, got["error"], "power actions disabled")
		}
	}
}

func TestPowerAction_EnabledSchedulesAndCallsExecutor(t *testing.T) {
	var (
		mu      sync.Mutex
		invoked []string
	)
	orig := agentd.ExecSystemctl
	agentd.ExecSystemctl = func(action string) error {
		mu.Lock()
		invoked = append(invoked, action)
		mu.Unlock()
		return nil
	}
	defer func() { agentd.ExecSystemctl = orig }()

	a, err := agentd.New(agentd.Config{
		DeviceID:           "x",
		ListenAddr:         "127.0.0.1:0",
		AgentVersion:       "0.1.0",
		EnablePowerActions: true,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, action := range []string{"reboot", "shutdown"} {
		resp, err := http.Post(ts.URL+"/api/v1/"+action, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("%s: got %d, want 202", action, resp.StatusCode)
		}
		var got map[string]string
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("%s: decode: %v", action, err)
		}
		if got["status"] != "scheduled" || got["action"] != action {
			t.Errorf("%s: body=%v", action, got)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(invoked) != 2 || invoked[0] != "reboot" || invoked[1] != "shutdown" {
		t.Errorf("invoked=%v, want [reboot shutdown]", invoked)
	}
}

func TestPowerAction_NonPOSTReturns405(t *testing.T) {
	a, err := agentd.New(agentd.Config{
		DeviceID:           "x",
		ListenAddr:         "127.0.0.1:0",
		AgentVersion:       "0.1.0",
		EnablePowerActions: true,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	for _, action := range []string{"reboot", "shutdown"} {
		resp, err := http.Get(ts.URL + "/api/v1/" + action)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s: got %d, want 405", action, resp.StatusCode)
		}
		if !strings.Contains(string(body), "method not allowed") {
			t.Errorf("%s: body=%q", action, body)
		}
	}
}
```

- [ ] **Step 2: 运行测试，预期 FAIL（编译失败：`agentd.ExecSystemctl` 未定义）**

```bash
go test ./internal/agentd/... -run TestPowerAction -count=1
```

Expected: 编译错误，提示 `undefined: agentd.ExecSystemctl`。

- [ ] **Step 3: 实现 — 修改 `internal/agentd/http.go`**

在文件顶部 `import` 段加入：

```go
import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime/debug"
	"time"
)
```

`os/exec` 与 `errors` 是新增。

在 `import` 之后、类型/方法之前，加包级变量（用于测试注入）：

```go
// ExecSystemctl invokes systemctl with the given action. Package-level
// for test injection; production code does not override it.
var ExecSystemctl = func(action string) error {
	cmd := exec.Command("systemctl", action)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Hand the child to systemd (PID 1) so the agent doesn't keep it
	// attached to its std{fd}. Do NOT Wait — reboot/poweroff will hang
	// the connection.
	return cmd.Process.Release()
}
```

修改 `Handler` 方法，注册两个新路由（替换原 `Handler`）：

```go
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/api/v1/info", a.handleInfo)
	mux.HandleFunc("/api/v1/reboot", a.handlePowerAction("reboot"))
	mux.HandleFunc("/api/v1/shutdown", a.handlePowerAction("shutdown"))
	return a.recoverMiddleware(mux)
}
```

在 `handleInfo` 之后加新方法：

```go
// handlePowerAction returns an http.Handler for POST /api/v1/{reboot,shutdown}.
// The action string is both the URL suffix passed to systemctl and the
// "action" field echoed in the 202 response body.
func (a *Agent) handlePowerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if err := ExecSystemctl(action); err != nil {
			a.logger.Error("start systemctl",
				slog.String("action", action),
				slog.String("err", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		a.logger.Info("power action scheduled", slog.String("action", action))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "scheduled",
			"action": action,
		})
	}
}
```

- [ ] **Step 4: 运行测试，预期 PASS**

```bash
go test ./internal/agentd/... -run TestPowerAction -count=1 -v
```

Expected: 三个测试全部 PASS。

- [ ] **Step 5: 跑全量测试，确保未破坏现有用例**

```bash
make test
```

Expected: 全部 PASS。如果 `agentd` 在 macOS 上因 build tag 跳过，预期结果中 `internal/agentd` 显示 `ok`（无 `_linux` 后缀的测试文件可跨平台运行）。

- [ ] **Step 6: 提交**

```bash
git add internal/agentd/http.go internal/agentd/http_test.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(agent): 增加 POST /api/v1/{reboot,shutdown} 端点

受 enable_power_actions 配置开关管控；关闭时 403，开启时调用 systemctl 并返回 202。仅接受 POST，其他方法返回 405。ExecSystemctl 暴露为包级变量以便测试注入。
EOF
)"
```

---

## Task 3: systemd unit hardening

**Files:**
- Modify: `scripts/spotterd.service`（在 `[Service]` 段末尾追加 4 行）

**Interfaces:**
- Consumes: 无
- Produces: 修改后的 systemd unit，部署时 `systemctl daemon-reload && systemctl restart spotterd` 即生效

- [ ] **Step 1: 修改 `scripts/spotterd.service`**

在现有 `[Service]` 段最后（`StartLimitIntervalSec=60` 之后、`[Install]` 之前）追加：

```ini
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
```

完整文件应类似：

```ini
[Unit]
Description=Spotter Device Discovery Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/spotterd --config /etc/spotterd/agent.toml
Restart=on-failure
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-multi.target
```

注意：保留 `WantedBy=multi-user.target`（保留原文不动）。最终 `[Install]` 段必须是 `WantedBy=multi-user.target`，不是 `multi-multi.target`——上面只是示意，请以原文为准。

- [ ] **Step 2: 验证文件格式**

```bash
cat scripts/spotterd.service
```

Expected: 看到追加的 4 行；其他内容不变。

- [ ] **Step 3: 提交**

```bash
git add scripts/spotterd.service
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
chore(agent): systemd unit 加基础 hardening

引入 NoNewPrivileges / ProtectSystem=strict / ProtectHome / PrivateTmp；不影响 systemctl reboot/poweroff（D-Bus 调用不依赖 setuid），但限制 agent 进程对系统的写访问。
EOF
)"
```

部署时无需重装：`scp` 覆盖 unit → `sudo systemctl daemon-reload && sudo systemctl restart spotterd`。

---

## Task 4: Scanner 端 HTTP 调用方法（TDD）

**Files:**
- Modify: `internal/scanner/scanner.go`（新增 `RebootDevice`、`ShutdownDevice`、`postPowerAction`、`errPowerActionTimeout`）
- Modify: `internal/scanner/scanner_test.go`（在文件末尾追加 4 个测试）

**Interfaces:**
- Consumes: `agentd` HTTP 端点契约（Task 2）
- Produces:
  - `Scanner.RebootDevice(ctx context.Context, ip string, port int) error`
  - `Scanner.ShutdownDevice(ctx context.Context, ip string, port int) error`
  - 包级 `var errPowerActionTimeout = errors.New("...")` —— 客户端 App（Task 5）据此把超时当作乐观成功

- [ ] **Step 1: 写失败的测试 — 在 `internal/scanner/scanner_test.go` 末尾追加**

打开 `internal/scanner/scanner_test.go`，在文件最后一个 `}` 之后追加（保持 `package scanner_test` 不变）：

```go
func TestPostPowerAction_AcceptedReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"scheduled","action":"reboot"}`))
	}))
	defer srv.Close()

	reg, _ := registry.Open(filepath.Join(t.TempDir(), "devices.json"))
	sc := scanner.New(reg)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	if err := sc.RebootDevice(context.Background(), addr.IP.String(), addr.Port); err != nil {
		t.Fatalf("reboot: %v", err)
	}
	if err := sc.ShutdownDevice(context.Background(), addr.IP.String(), addr.Port); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestPostPowerAction_ForbiddenReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"power actions disabled"}`))
	}))
	defer srv.Close()

	reg, _ := registry.Open(filepath.Join(t.TempDir(), "devices.json"))
	sc := scanner.New(reg)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	err := sc.RebootDevice(context.Background(), addr.IP.String(), addr.Port)
	if err == nil || !strings.Contains(err.Error(), "power actions disabled") {
		t.Fatalf("want disabled error, got %v", err)
	}
}

func TestPostPowerAction_500ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"oops"}`))
	}))
	defer srv.Close()

	reg, _ := registry.Open(filepath.Join(t.TempDir(), "devices.json"))
	sc := scanner.New(reg)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	err := sc.ShutdownDevice(context.Background(), addr.IP.String(), addr.Port)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if strings.Contains(err.Error(), "power actions disabled") {
		t.Fatalf("500 must NOT be reported as disabled: %v", err)
	}
}

func TestPostPowerAction_TimeoutReturnsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block past the client timeout; HTTPClient default is 3s,
		// but we set the HTTP client below to a shorter one.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	reg, _ := registry.Open(filepath.Join(t.TempDir(), "devices.json"))
	sc := scanner.New(reg, scanner.WithHTTPClient(&http.Client{Timeout: 100 * time.Millisecond}))
	addr := srv.Listener.Addr().(*net.TCPAddr)
	err := sc.RebootDevice(context.Background(), addr.IP.String(), addr.Port)
	if !errors.Is(err, scanner.ErrPowerActionTimeout) {
		t.Fatalf("want ErrPowerActionTimeout, got %v", err)
	}
}
```

- [ ] **Step 2: 运行新增测试，预期 FAIL（编译失败：`RebootDevice`/`ErrPowerActionTimeout` 未定义）**

```bash
go test ./internal/scanner/... -run TestPostPowerAction -count=1
```

Expected: 编译错误。

- [ ] **Step 3: 在 `internal/scanner/scanner.go` 中实现**

`import` 段加入：

```go
import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/spotter/spotter/internal/protocol"
	"github.com/spotter/spotter/internal/registry"
)
```

`fmt` 和 `errors` 是新增。

在文件顶部、`type Event interface` 之前加 sentinel：

```go
// ErrPowerActionTimeout is returned by RebootDevice/ShutdownDevice when
// the HTTP client timed out. Callers (e.g. the Wails App) treat it as
// "the command may have been sent" and surface an optimistic success.
var ErrPowerActionTimeout = errors.New("scanner: power action timed out, device may have responded")
```

注意：测试代码用 `scanner.ErrPowerActionTimeout` 访问，所以必须是**导出**变量。

在文件末尾、`HTTPClient` 方法之后加：

```go
// RebootDevice POSTs to /api/v1/reboot on the device. Returns
// ErrPowerActionTimeout on client-side timeout (treated as "may have
// succeeded" by callers); other errors are terminal.
func (s *Scanner) RebootDevice(ctx context.Context, ip string, port int) error {
	return s.postPowerAction(ctx, ip, port, "reboot")
}

// ShutdownDevice POSTs to /api/v1/shutdown. Same semantics as RebootDevice.
func (s *Scanner) ShutdownDevice(ctx context.Context, ip string, port int) error {
	return s.postPowerAction(ctx, ip, port, "shutdown")
}

func (s *Scanner) postPowerAction(ctx context.Context, ip string, port int, action string) error {
	url := fmt.Sprintf("http://%s:%d/api/v1/%s", ip, port, action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.opts.HTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isHTTPClientTimeout(err) {
			return ErrPowerActionTimeout
		}
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("power actions disabled")
	default:
		return fmt.Errorf("power action %q: unexpected status %d", action, resp.StatusCode)
	}
}

// isHTTPClientTimeout detects the http.Client's own timeout (not
// context-driven). http.Client surfaces it as a *url.Error whose
// Timeout() method returns true.
func isHTTPClientTimeout(err error) bool {
	var urlErr *httpErr
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	return false
}
```

注意：`isHTTPClientTimeout` 引用了未定义的 `*httpErr`。修正——直接用 `*url.Error`：

```go
func isHTTPClientTimeout(err error) bool {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}
	return false
}
```

并在 `import` 段加入：

```go
	"net/url"
```

**如果 `WithHTTPClient` 在 Options 上不存在**：在 `internal/scanner/scanner.go` 的 `Options` 结构体中加一行：

```go
HTTPClient     *http.Client
```

并在 `withDefaults` 默认值分支上方不动（`if o.HTTPClient == nil` 已经存在）。无需其他改动。如果 `WithHTTPClient` 测试 helper 不存在，添加：

```go
// WithHTTPClient overrides the default HTTP client (used by Scanner.RebootDevice etc.).
func WithHTTPClient(c *http.Client) func(*Options) {
	return func(o *Options) { o.HTTPClient = c }
}
```

> 实施者注：`Options.HTTPClient` 与 `withDefaults` 已存在于 `scanner.go`（见现有 `withDefaults` 实现）；但 `WithHTTPClient` 函数式选项可能不存在，需要新增到文件末尾。

- [ ] **Step 4: 运行测试，预期 PASS**

```bash
go test ./internal/scanner/... -run TestPostPowerAction -count=1 -v
```

Expected: 4 个测试全部 PASS。

- [ ] **Step 5: 跑全量测试**

```bash
make test
```

Expected: 全部 PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(scanner): 增加 RebootDevice / ShutdownDevice 方法

封装 POST /api/v1/{reboot,shutdown} 调用，复用现有 HTTPClient。超时返回 ErrPowerActionTimeout sentinel，供 App 层做乐观成功处理。
EOF
)"
```

---

## Task 5: Wails App 绑定（main.go）

**Files:**
- Modify: `main.go`（新增 `RebootDevice(deviceID string) error` 与 `ShutdownDevice(deviceID string) error` 两个方法）

**Interfaces:**
- Consumes: `Scanner.RebootDevice` / `ShutdownDevice`（Task 4）；`registry.Registry.Get`（已有）
- Produces: 两个 Wails 绑定方法，签名 `RebootDevice(deviceID string) error` 与 `ShutdownDevice(deviceID string) error`

- [ ] **Step 1: 在 `main.go` 末尾追加两个方法**

定位到 `ClearRegistry` 方法之后（约 348 行），追加：

```go
// RebootDevice sends a remote reboot command to the device identified
// by deviceID. Returns an error if the device is not in the registry
// or is marked offline. A client-side HTTP timeout is treated as
// success — the command may have been dispatched before the agent's
// connection hung up during reboot.
func (a *App) RebootDevice(deviceID string) error {
	return a.powerAction(deviceID, "reboot")
}

// ShutdownDevice sends a remote shutdown command. Same semantics as
// RebootDevice. Note: there is no remote power-on; the device must be
// physically powered back on.
func (a *App) ShutdownDevice(deviceID string) error {
	return a.powerAction(deviceID, "shutdown")
}

// powerAction is the shared body of RebootDevice/ShutdownDevice.
func (a *App) powerAction(deviceID string, action string) error {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	switch action {
	case "reboot":
		err = a.scanner.RebootDevice(ctx, entry.IP, port)
	case "shutdown":
		err = a.scanner.ShutdownDevice(ctx, entry.IP, port)
	}
	if errors.Is(err, scanner.ErrPowerActionTimeout) {
		a.logger.Info("power action timeout (optimistic success)",
			slog.String("device_id", deviceID),
			slog.String("action", action))
		return nil
	}
	return err
}
```

- [ ] **Step 2: 检查 `import` 段是否已包含 `errors` 和 `scanner`**

`main.go` 现有 imports（7-32 行）已包含 `"github.com/spotter/spotter/internal/scanner"`。需要确认 `"errors"` 是否已存在。如果不存在，加上：

```go
	"errors"
```

- [ ] **Step 3: 编译验证**

```bash
make client
```

Expected: 编译通过（无报错）。注意：Wails 会触发前端构建，可能耗时较久；如果只想验证 Go 编译，可直接：

```bash
go build .
```

Expected: 无错误。

- [ ] **Step 4: 提交**

```bash
git add main.go
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(client): 暴露 RebootDevice / ShutdownDevice Wails 绑定

App 通过 registry 查 ip:port，5s context 包装 scanner 调用。超时视作乐观成功；offline / 未注册设备返回明确错误。
EOF
)"
```

---

## Task 6: 前端 hook（useDeviceActions）

**Files:**
- Modify: `frontend/src/hooks/useDeviceActions.ts`

**Interfaces:**
- Consumes: Wails 自动生成的 `RebootDevice` / `ShutdownDevice` 绑定（来自 Task 5；Wails 编译后落在 `wailsjs/go/main/App.d.ts`）
- Produces:
  - `DeviceActions` 接口新增 `reboot(deviceID: string): Promise<void>` 与 `shutdown(deviceID: string): Promise<void>`
  - `useDeviceActions()` 返回的对象新增这两个方法

- [ ] **Step 1: 修改 `frontend/src/hooks/useDeviceActions.ts`**

完整文件内容：

```typescript
import {
  ScanSubnet,
  ProbeByIP,
  RefreshNow,
  RebootDevice,
  ShutdownDevice,
} from '../../wailsjs/go/main/App';

export interface DeviceActions {
  scan: (cidr?: string) => Promise<void>;
  add: (ip: string, port: number, username: string) => Promise<void>;
  refresh: () => Promise<void>;
  reboot: (deviceID: string) => Promise<void>;
  shutdown: (deviceID: string) => Promise<void>;
}

export function useDeviceActions(): DeviceActions {
  return {
    scan: async (cidr) => {
      await ScanSubnet(cidr ?? '');
    },
    add: async (ip, port, _username) => {
      await ProbeByIP(ip, port, _username);
    },
    refresh: async () => {
      await RefreshNow();
    },
    reboot: async (deviceID) => {
      await RebootDevice(deviceID);
    },
    shutdown: async (deviceID) => {
      await ShutdownDevice(deviceID);
    },
  };
}
```

- [ ] **Step 2: 验证 Wails 绑定文件存在**

```bash
ls frontend/wailsjs/go/main/App.d.ts && grep -E 'RebootDevice|ShutdownDevice' frontend/wailsjs/go/main/App.d.ts
```

Expected: 文件存在，且两个绑定名称都已声明。如果 Wails 尚未重新生成（CI 流程未跑过），可手动触发：

```bash
wails generate module
```

或在 Task 5 之后 `make client` 时由 Wails 自动生成。

- [ ] **Step 3: TypeScript 类型检查**

```bash
cd frontend && npx tsc --noEmit
```

Expected: 无类型错误。如果 `tsc` 缺失，先 `npm install`（参见 README）。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/hooks/useDeviceActions.ts
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(gui): useDeviceActions 增加 reboot / shutdown 方法

直接代理 Wails 绑定；前端组件用统一的 actions.reboot(deviceID) / shutdown(deviceID) 调用。
EOF
)"
```

---

## Task 7: 前端 DetailPanel 标题头右侧电源按钮

**Files:**
- Modify: `frontend/src/components/DetailPanel.tsx`
- Modify: `frontend/src/i18n/dictionaries.ts`（新增 5 个键，中英双语）

**Interfaces:**
- Consumes: `useDeviceActions()` 返回的 `reboot` / `shutdown` 方法（Task 6）；antd `Button` / `Modal` / `message` / `Space`；`@ant-design/icons` 的 `PoweroffOutlined` / `CloseCircleOutlined`
- Produces: 详情标题头右侧出现两个按钮，online 时可点；点击触发 `Modal.confirm` 二次确认

- [ ] **Step 1: 在 `dictionaries.ts` 新增 5 个键（中文段 + 英文段）**

打开 `frontend/src/i18n/dictionaries.ts`：

英文段（在 `'detail.refresh': 'Refresh'` 之后，`// Cards` 之前）追加：

```typescript
    'detail.refresh': 'Refresh',
    'detail.actions.power.reboot': 'Reboot',
    'detail.actions.power.shutdown': 'Shut down',
    'detail.actions.power.reboot.confirmTitle': 'Reboot {hostname}?',
    'detail.actions.power.reboot.confirmOk': 'Reboot',
    'detail.actions.power.shutdown.confirmTitle': 'Shut down {hostname}?',
    'detail.actions.power.shutdown.confirmOk': 'Shut down',
    'detail.actions.power.disabledHint': 'Enable enable_power_actions on the agent to use this.',
```

中文段（在 `'detail.refresh': '刷新'` 之后）追加：

```typescript
    'detail.refresh': '刷新',
    'detail.actions.power.reboot': '重启',
    'detail.actions.power.shutdown': '关机',
    'detail.actions.power.reboot.confirmTitle': '即将重启 {hostname}？',
    'detail.actions.power.reboot.confirmOk': '重启',
    'detail.actions.power.shutdown.confirmTitle': '即将关闭 {hostname}？',
    'detail.actions.power.shutdown.confirmOk': '关闭电源',
    'detail.actions.power.disabledHint': '该设备未启用远程电源管理（需在 agent.toml 设置 enable_power_actions = true）。',
```

- [ ] **Step 2: 修改 `DetailPanel.tsx`**

完整文件内容（替换原文件）：

```tsx
import { useState } from 'react';
import { Button, Modal, Space, message } from 'antd';
import { PoweroffOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import DeviceActions from './DeviceActions';

export default function DetailPanel() {
  const { state } = useDevices();
  const { t } = useI18n();
  const actions = useDeviceActions();
  const [busyAction, setBusyAction] = useState<'reboot' | 'shutdown' | null>(null);
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  const hostname = device?.last_info?.basic?.hostname || device?.ip || '';

  const runPowerAction = async (kind: 'reboot' | 'shutdown') => {
    if (!device) return;
    const titleKey = kind === 'reboot' ? 'detail.actions.power.reboot.confirmTitle' : 'detail.actions.power.shutdown.confirmTitle';
    const okKey = kind === 'reboot' ? 'detail.actions.power.reboot.confirmOk' : 'detail.actions.power.shutdown.confirmOk';
    const bodyKey = kind === 'shutdown' ? 'detail.actions.power.shutdown' : 'detail.actions.power.reboot';

    Modal.confirm({
      title: t(titleKey).replace('{hostname}', hostname),
      content: t(bodyKey),
      okText: t(okKey),
      okButtonProps: { danger: kind === 'shutdown' },
      cancelText: t('detail.actions.cancel') ?? 'Cancel',
      onOk: async () => {
        setBusyAction(kind);
        try {
          if (kind === 'reboot') await actions.reboot(device.device_id);
          else await actions.shutdown(device.device_id);
          message.success(t('detail.actions.power.toast.success') ?? 'Command sent');
        } catch (e: any) {
          const msg = (e?.message ?? String(e)) as string;
          if (msg.includes('power actions disabled')) {
            message.error(t('detail.actions.power.disabledHint'));
          } else if (msg.includes('offline')) {
            message.error(msg);
          } else {
            message.error(msg);
          }
        } finally {
          setBusyAction(null);
        }
      },
    });
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {!device ? <EmptyState /> : (
          <>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 16,
              }}
            >
              <h2 style={{ margin: 0, color: 'var(--text-primary)' }}>
                {device.last_info?.basic?.hostname || device.ip}
                <span
                  style={{
                    marginLeft: 12,
                    fontSize: 13,
                    color: device.online ? '#52c41a' : '#b71c1c',
                  }}
                >
                  {device.online ? t('detail.status.online') : t('detail.status.offline')}
                </span>
              </h2>
              <Space>
                <Button
                  size="small"
                  icon={<PoweroffOutlined />}
                  disabled={!device.online || busyAction !== null}
                  loading={busyAction === 'reboot'}
                  onClick={() => runPowerAction('reboot')}
                >
                  {t('detail.actions.power.reboot')}
                </Button>
                <Button
                  size="small"
                  danger
                  icon={<CloseCircleOutlined />}
                  disabled={!device.online || busyAction !== null}
                  loading={busyAction === 'shutdown'}
                  onClick={() => runPowerAction('shutdown')}
                >
                  {t('detail.actions.power.shutdown')}
                </Button>
              </Space>
            </div>
            <div style={{ display: 'grid', gap: 12 }}>
              <BasicCard device={device} />
              <NetworkCard device={device} />
              <JetsonCard device={device} />
            </div>
          </>
        )}
      </div>
      {device && <DeviceActions />}
    </div>
  );
}
```

注意：
- `Modal.confirm` 文案使用 `t(...).replace('{hostname}', hostname)` 占位符替换，避免在 i18n 字符串里直接拼接。
- 关闭按钮文案临时用 `t('detail.actions.cancel') ?? 'Cancel'`；如果后续 `dictionaries.ts` 没有这键，fallback 为 'Cancel'。可选：把 `Cancel` 也加入 i18n keys，但 YAGNI——保留 fallback。

- [ ] **Step 3: TypeScript 类型检查 + Vite 构建**

```bash
cd frontend && npx tsc --noEmit && npm run build
```

Expected: 类型无错误；Vite 产出 `frontend/dist/`，无报错。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/components/DetailPanel.tsx frontend/src/i18n/dictionaries.ts
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
feat(gui): 详情标题头右侧增加重启 / 关机按钮

带 antd Modal 二次确认（标题带 hostname）；按钮在设备 offline 或操作中 disabled。错误信息区分 403（未启用）与离线 / 其他失败。i18n 中英双语齐全。
EOF
)"
```

---

## Task 8: 文档更新

**Files:**
- Modify: `README.md`（中文版已知限制段）
- Modify: `docs/api.md`（中文 API 文档）
- Modify: `docs/api.en.md`（英文 API 文档）
- Modify: `SECURITY.md`
- Modify: `docs/operations.md`
- Modify: `docs/operations.en.md`

**Interfaces:**
- Consumes: 设计 spec §6
- Produces: 5 份文档更新，覆盖 README、API、SECURITY、operations 中英双语

- [ ] **Step 1: 修改 `README.md` 已知限制段**

找到 README 中「已知限制（MVP）」段，将：

```
- **不支持远端命令执行** —— 仅提供静态信息面板。
```

替换为：

```
- **不支持任意远端命令执行** —— 仅提供 opt-in 的远程 reboot / shutdown（需在 agent 端 `/etc/spotterd/agent.toml` 中设置 `enable_power_actions = true`）。不提供 shell 或自定义命令通道。
```

并把下一行「HTTP 端点 无身份认证」保持不变，但把破折号后的描述从「仅限可信局域网内部署」改为「仍仅限可信局域网内部署；启用电源管理等于授予同网段任何客户端触发 root 级别 reboot / poweroff 的权限」。

- [ ] **Step 2: 修改 `docs/api.md`（中文版）**

在 `/api/v1/info` 与 `/healthz` 文档段之后，新增端点定义：

````markdown
### `POST /api/v1/reboot`

请求设备重启。**仅在 agent 配置 `enable_power_actions = true` 时生效**。

请求：
- Headers：`Content-Type` 不要求；无需 body。

响应（200/202，命令已派发）：
```json
{
  "status": "scheduled",
  "action": "reboot"
}
```

响应（403，未启用）：
```json
{
  "error": "power actions disabled"
}
```

响应（405，非 POST）：文本 `method not allowed`。

### `POST /api/v1/shutdown`

同 reboot，但调用 `systemctl poweroff`。**该操作不可逆**，需手动上电才能恢复。

### `enable_power_actions`

`/etc/spotterd/agent.toml`：

```toml
enable_power_actions = true   # 默认 false
```

开启后 agent 接受 `POST /api/v1/reboot` 与 `/api/v1/shutdown`。无身份认证，部署方负责网络隔离。
````

- [ ] **Step 3: 修改 `docs/api.en.md`**

与 Step 2 同样结构，文字改为英文：

````markdown
### `POST /api/v1/reboot`

Requests the device to reboot. Only effective when `enable_power_actions = true` in the agent config.

Request:
- Headers: no body required.

Response (200/202, scheduled):
```json
{
  "status": "scheduled",
  "action": "reboot"
}
```

Response (403, disabled):
```json
{
  "error": "power actions disabled"
}
```

Response (405, non-POST): plain text `method not allowed`.

### `POST /api/v1/shutdown`

Same as reboot, but invokes `systemctl poweroff`. **Irreversible** — the device requires manual power-on.

### `enable_power_actions`

`/etc/spotterd/agent.toml`:

```toml
enable_power_actions = true   # default false
```

When enabled, the agent accepts `POST /api/v1/reboot` and `/api/v1/shutdown`. Unauthenticated; deployer is responsible for network isolation.
````

- [ ] **Step 4: 修改 `SECURITY.md`**

找到「加固 checklist」或类似段，追加两条 bullet：

```
- 在启用 agent 的 `enable_power_actions = true` 前，确保设备在受控 VLAN / VPN 之后。
- `enable_power_actions = true` 等于授权该子网任何客户端触发 root 级别的 reboot / poweroff。
```

- [ ] **Step 5: 修改 `docs/operations.md`**

在设备端部署段（提到 `agent.toml` 字段的位置）追加一句：

```
可选：在 `agent.toml` 中设置 `enable_power_actions = true` 以启用 GUI 远程电源管理（默认关闭）。
```

- [ ] **Step 6: 修改 `docs/operations.en.md`**

对应英文版同样追加：

```
Optional: set `enable_power_actions = true` in `agent.toml` to enable GUI-driven remote power actions (off by default).
```

- [ ] **Step 7: 验证构建 + 测试**

```bash
make test
```

Expected: 全部 PASS（文档更新不影响 Go 测试）。

- [ ] **Step 8: 提交**

```bash
git add README.md docs/api.md docs/api.en.md SECURITY.md docs/operations.md docs/operations.en.md
git commit --author="letmlook <letmlook@aliyun.com>" -m "$(cat <<'EOF'
docs: 文档同步电源管理功能

README 已知限制段、API 文档（中英）、SECURITY 加固清单、operations 部署说明（中英）同步说明 reboot/shutdown 端点、enable_power_actions 开关与无鉴权假设。
EOF
)"
```

---

## Self-Review Checklist

- [x] Spec coverage:
  - §1.1 目标：Task 7 ✓
  - §1.2 非目标（无状态、无 token）：未引入新状态字段；无 token 逻辑 ✓
  - §1.3 验收标准 A（403 disabled）：Task 2 ✓
  - §1.3 B（202 enabled + systemctl）：Task 2 + 3 ✓
  - §1.3 C（panic 中间件）：Task 2 沿用现有 `recoverMiddleware` ✓
  - §1.3 D（GUI offline disabled + Modal）：Task 7 ✓
  - §1.3 E（registry 找不到/offline 明确错误）：Task 5 ✓
  - §1.3 F（systemd hardening 不影响 reboot）：Task 3 ✓
  - §1.3 G（测试覆盖）：Task 2 + 4 ✓
  - §1.3 H（现有测试不破坏）：每个 task 末尾都有 `make test` 步骤 ✓
- [x] Placeholder scan：no TBD/TODO/"implement later"/"similar to"。
- [x] Type consistency：
  - `scanner.RebootDevice(ctx, ip, port) error` 在 Task 4 定义、Task 5 调用 ✓
  - `scanner.ErrPowerActionTimeout`（导出）在 Task 4 定义、Task 4 测试 + Task 5 使用 ✓
  - `agentd.ExecSystemctl`（导出）在 Task 2 定义、Task 2 测试使用 ✓
  - `useDeviceActions().reboot(deviceID)` 在 Task 6 定义、Task 7 调用 ✓
  - i18n 键名 `detail.actions.power.*` 在 Task 7 Step 1 定义、Step 2 使用 ✓
- [x] Spec 范围内：单次实施计划可完成，不需分解。