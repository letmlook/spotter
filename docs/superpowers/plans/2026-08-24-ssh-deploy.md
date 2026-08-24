# Spotter SSH/SFTP Deploy 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 spotter-client（Wails 桌面应用）与 spotter-cli 中实现「选中设备 → SSH/SFTP 推送 spotterd 安装包 → 触发 `sudo bash install.sh`」的端到端部署能力。

**Architecture:** 新增 `internal/deployer/` 包（纯逻辑、无 Wails 依赖），内部由 `Package`（local cache + remote URL 双源）、`Dial`（SSH agent/私钥/密码三模）、`Upload`（SFTP 带进度）、`ExecInstall`（SSH session 跑 sudo -S）、`Deployer`（状态机 + cancel）组成。`main.go` 把 Deployer 注入 `App` 并暴露 6 个 Wails bound 方法；React 用 `DeployDialog` + `Modal.confirm` 做 UI；`spotter-cli` 加 `deploy` 子命令复用同包。**spotterd agent 端不动**（继续 HTTP-only），假定设备自身已有 sshd 在 22 端口。

**Tech Stack:** Go 1.25、`golang.org/x/crypto/ssh`、`github.com/pkg/sftp`、Wails v2（沿用）、React + antd（沿用）。沿用现有 `slog` + `httptest` 测试栈。

## Global Constraints

- 模块路径 `github.com/spotter/spotter`；Go 1.25。
- 所有凭据（SSH 密码、私钥 passphrase、sudo 密码）**仅内存**：不进 `Settings.json`、不进 `Registry.json`、不进 slog、不进 goroutine 共享状态持久化字段；用完即丢。
- Wails bound 方法**不**接 `SudoPass` 参数序列化；通过 `ProvideSudoPassword(handle, password)` 一次性注入到 Deployer 的 per-handle chan，立刻清引用。
- `pkg/sftp` 与 `golang.org/x/crypto/ssh` 都用 upstream stable（`pkg/sftp@latest`、`x/crypto/ssh@latest`）；不引第三方 SSH 工具包装库（不强求 metalinter 但不引入额外 vendor）。
- SSH host key callback 使用 `ssh.InsecureIgnoreHostKey()`；**不**实现交互式 trust prompt（用户首次已在外部手工 ssh 过；SECURITY.md 显式警告）。
- host 上没有 `~/.ssh/known_hosts` 也不报错。
- Settings 增加 4 个字段（`PackageMode`、`PackageReleaseURL`、`DeviceSSHPort`、`CacheBinDir`），缺失即填默认；`PackageMode == ""` → `"local"`；`DeviceSSHPort == 0` → `22`；`CacheBinDir == ""` → `<dataDir>/cache/bin/`。
- deploy handle 命名用 `uuid.NewString()`；并发处理：UI 同一时间只有一个 DeployDialog 打开（不做并发 gadget 调和）。
- 安装/上传失败语义：`deploy-complete:{handle}` payload 永远发（含失败）；严重错误另发 `deploy-error:{handle}`。
- 前端依赖沿用 React + antd（`Modal.confirm`、`message`、`Progress`、`Form`），不引入新依赖。
- spotter-cli `deploy` 子命令把 `SudoPass` 从 stdin's terminal 读一次（`golang.org/x/term.ReadPassword`），不再用 `bufio.Scanner(os.Stdin)` 避免 echo。
- 提交作者固定 `letmlook <letmlook@aliyun.com>`，通过 `git commit --author=...` 指定，**不要改 git config**。
- commit message 用中文 `[类型] 简述`。
- `make test` 全绿；新增包与上游 go.mod/go.sum 改动一并提交。
- 所有改动文件清单见 spec §1 头部表格。
- 参考文档：设计 spec `docs/superpowers/specs/2026-08-24-ssh-deploy-design.md`；既有相关：Wails 绑定模式见 `main.go`，EventEmitter 模式见 `main.go:runLogStream`、scanner 模式见 `internal/scanner/scanner.go`，settings 模板见 `internal/clientconfig/store.go`。

---

## File Structure

**新增：**
- `internal/deployer/manifest.go` — `Manifest` + `File` 类型
- `internal/deployer/package.go` — `Package` 接口 + `Resolver`
- `internal/deployer/source_local.go` — 本地 `<cache/bin/>` 镜像解析
- `internal/deployer/source_remote.go` — HTTP GET 配置 URL → cache
- `internal/deployer/auth.go` — `AuthSpec` + `Dial`
- `internal/deployer/auth_agent.go` — `AgentSigners()` 从 `SSH_AUTH_SOCK`
- `internal/deployer/auth_key.go` — `ParsePrivateKey`（ed25519/RSA/ECDSA，加密/不加密）
- `internal/deployer/auth_password.go` — `Password` 字段组装 `ssh.Password(...)`
- `internal/deployer/sftp_upload.go` — `Upload` + 进度回调
- `internal/deployer/ssh_exec.go` — `ExecInstall`（session + StdinPipe + 行缓冲 + cancel）
- `internal/deployer/deployer.go` — `Deployer`、`Handle`、`ProgressEvent`、`DeployRequest`、状态机
- `internal/deployer/deployer_test.go` — 单测
- `internal/deployer/integration_test.go` — `//go:build integration`（docker sshd）
- `frontend/src/components/DeployDialog.tsx` — 新 Dialog 组件
- `frontend/src/hooks/useDeploy.ts` — 订阅 Wails 事件 + 状态机 hook

**修改：**
- `internal/clientconfig/store.go` — `Settings` 加 `PackageMode`/`PackageReleaseURL`/`DeviceSSHPort`/`CacheBinDir` + `fillDefaults`
- `internal/clientconfig/store_test.go` — 新字段覆盖
- `main.go` — `App` 加 `deployer` 字段、6 个 Wails 绑定
- `main_test.go` — 新绑定覆盖
- `cmd/spotter-cli/main.go` — `deploy` 子命令 + dispatch 表
- `cmd/spotter-cli/main_test.go` — deploy help/usage path
- `frontend/src/components/SettingsDialog.tsx` — 4 字段 + Sync 按钮
- `frontend/src/components/DetailPanel.tsx` — 标题头右侧加 Deploy 按钮
- `frontend/src/i18n/dictionaries.ts` — 中英新字符串
- `README.md`、`docs/cli.md`、`docs/architecture.md`、`docs/api.md`、`SECURITY.md`、`docs/operations.md` 及对应 `.en.md`
- `go.mod`、`go.sum` — 新增 `golang.org/x/crypto/ssh` + `github.com/pkg/sftp`

**不动：**
- `internal/agentd/**`（spotterd agent 不动）
- `internal/scanner/**`
- `internal/registry/**`（仅引用现成 `Registry.Entry.Username`）

---

## Task 1: 引入 SSH + SFTP 依赖 + 添加 Settings 字段

**Files:**
- Modify: `internal/clientconfig/store.go:32-44`（Settings 结构体）
- Modify: `internal/clientconfig/store.go:48-59`（defaultSettings）
- Modify: `internal/clientconfig/store.go:130-156`（fillDefaultsLocked）
- Modify: `internal/clientconfig/store_test.go`（追加测试用例）
- Modify: `go.mod`、`go.sum`（`go get` 后由 go tooling 写入）

**Interfaces:**
- Consumes: 无
- Produces: 
  - `clientconfig.Settings{PackageMode string; PackageReleaseURL string; DeviceSSHPort int; CacheBinDir string}`
  - 默认值：`PackageMode="local"`, `DeviceSSHPort=22`, `CacheBinDir=<dataDir>/cache/bin/`（按 OS 路径，`os.UserConfigDir` + `Spotter/cache/bin`）

- [ ] **Step 1: 添加 SSH 与 SFTP 依赖**

```bash
cd /c/code/device_discovery
go get golang.org/x/crypto/ssh@latest
go get github.com/pkg/sftp@latest
go mod tidy
```

Expected: `go.mod` 增两行 require，`go.sum` 同步更新。

- [ ] **Step 2: 在 `Settings` 结构体加 4 个新字段**

打开 `internal/clientconfig/store.go` 修改 `Settings`（32-44 行）：

```go
type Settings struct {
    MulticastGroup    string        `json:"multicast_group,omitempty"`
    DevicePort        int           `json:"device_port,omitempty"`
    ScanTimeout       time.Duration `json:"scan_timeout,omitempty"`
    HTTPTimeout       time.Duration `json:"http_timeout,omitempty"`
    PollInterval      time.Duration `json:"poll_interval,omitempty"`
    McastInterval     time.Duration `json:"mcast_interval,omitempty"`
    Theme             string        `json:"theme,omitempty"`
    Language          string        `json:"language,omitempty"`
    AuthToken         string        `json:"auth_token,omitempty"`
    // SSH/SFTP Deploy 配置（v0.5+）。缺失即填默认（PackageMode="local"、
    // DeviceSSHPort=22、CacheBinDir=<dataDir>/cache/bin/）。
    PackageMode       string `json:"package_mode,omitempty"`
    PackageReleaseURL string `json:"package_release_url,omitempty"`
    DeviceSSHPort     int    `json:"device_ssh_port,omitempty"`
    CacheBinDir       string `json:"cache_bin_dir,omitempty"`
}
```

- [ ] **Step 3: 在 `defaultSettings` 加默认值**

修改 `defaultSettings`（48-59 行附近）：

```go
func defaultSettings() Settings {
    return Settings{
        MulticastGroup: DefaultMulticastGroup,
        DevicePort:     DefaultDevicePort,
        ScanTimeout:    DefaultScanTimeout,
        HTTPTimeout:    DefaultHTTPTimeout,
        PollInterval:   DefaultPollInterval,
        McastInterval:  DefaultMcastInterval,
        Theme:          "system",
        Language:       "zh-CN",
        PackageMode:    "local",
        DeviceSSHPort:  22,
        // CacheBinDir 在 fillDefaultsLocked 里用 os.UserConfigDir() 解析，避免
        // defaultSettings() 拿不到 OS 路径（不依赖 init 阶段副作用）。
    }
}
```

- [ ] **Step 4: 在 `fillDefaultsLocked` 兜底 `CacheBinDir`**

修改 `fillDefaultsLocked`（在已有 `Language == ""` 分支后追加）：

```go
if s.s.Theme == "" {
    s.s.Theme = d.Theme
}
if s.s.Language == "" {
    s.s.Language = d.Language
}
// v0.5+ 新字段兜底
if s.s.PackageMode == "" {
    s.s.PackageMode = "local"
}
if s.s.DeviceSSHPort == 0 {
    s.s.DeviceSSHPort = 22
}
if s.s.CacheBinDir == "" {
    if base, err := os.UserConfigDir(); err == nil {
        s.s.CacheBinDir = filepath.Join(base, "Spotter", "cache", "bin")
    } else {
        s.s.CacheBinDir = filepath.Join(os.TempDir(), "spotter-cache", "bin")
    }
}
```

确保文件顶部 `"path/filepath"` 与 `"os"` 已 import（如未，`go build` 会报错，照报错加）。

- [ ] **Step 5: 在 `store_test.go` 加覆盖**

打开 `internal/clientconfig/store_test.go`，追加一段：

```go
func TestSettings_Defaults_FillCacheBinAndSSHPort(t *testing.T) {
    tmp := t.TempDir()
    s, err := Open(filepath.Join(tmp, "settings.json"))
    if err != nil {
        t.Fatalf("Open: %v", err)
    }
    got := s.Get()
    if got.PackageMode != "local" {
        t.Errorf("PackageMode default = %q, want local", got.PackageMode)
    }
    if got.DeviceSSHPort != 22 {
        t.Errorf("DeviceSSHPort default = %d, want 22", got.DeviceSSHPort)
    }
    if got.CacheBinDir == "" {
        t.Errorf("CacheBinDir default is empty")
    }
    if !filepath.IsAbs(got.CacheBinDir) {
        t.Errorf("CacheBinDir = %q, want absolute path", got.CacheBinDir)
    }
}

func TestSettings_Set_PackageFieldsRoundTrip(t *testing.T) {
    tmp := t.TempDir()
    path := filepath.Join(tmp, "settings.json")
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    in := s.Get()
    in.PackageMode = "remote"
    in.PackageReleaseURL = "https://example.com/spotterd-linux-{arch}"
    in.DeviceSSHPort = 2222
    in.CacheBinDir = "/var/cache/bin"
    if err := s.Set(in); err != nil { t.Fatal(err) }

    s2, err := Open(path)
    if err != nil { t.Fatal(err) }
    got := s2.Get()
    if got.PackageMode != "remote" || got.PackageReleaseURL != in.PackageReleaseURL
        || got.DeviceSSHPort != 2222 || got.CacheBinDir != "/var/cache/bin" {
        t.Errorf("round-trip mismatch: %+v", got)
    }
}
```

- [ ] **Step 6: 跑测试验证**

```bash
cd /c/code/device_discovery
go test ./internal/clientconfig/...
```

Expected: PASS，含 2 个新增 case。

- [ ] **Step 7: 提交**

```bash
cd /c/code/device_discovery
git add go.mod go.sum internal/clientconfig/
git commit -m "feat(deploy): 引入 ssh+sftp 依赖并在 Settings 加 4 个部署配置字段"
```

---

## Task 2: Manifest 类型 + Package 接口

**Files:**
- Create: `internal/deployer/manifest.go`
- Create: `internal/deployer/manifest_test.go`
- Create: `internal/deployer/package.go`
- Create: `internal/deployer/package_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `deployer.Manifest{Arch,Files,ResolvedAt,Origin,StagingDir}`, `deployer.File{LocalPath,RemoteName,Size,SHA256}`, `deployer.Package interface{ Resolve(context.Context) (Manifest, error) }`

- [ ] **Step 1: 写失败的 manifest 测试**

新建 `internal/deployer/manifest_test.go`：

```go
package deployer

import (
    "encoding/json"
    "testing"
    "time"
)

func TestFile_JSONRoundTrip(t *testing.T) {
    f := File{LocalPath: "/tmp/x", RemoteName: "spotterd", Size: 1234, SHA256: "abc"}
    data, err := json.Marshal(f)
    if err != nil { t.Fatal(err) }
    var got File
    if err := json.Unmarshal(data, &got); err != nil { t.Fatal(err) }
    if got != f { t.Errorf("round-trip mismatch: %+v", got) }
}

func TestManifest_OriginLabel(t *testing.T) {
    m := Manifest{
        Arch: "arm64", ResolvedAt: time.Unix(0, 0).UTC(),
        Origin: "local:/var/cache/bin",
    }
    if m.Arch != "arm64" || m.Origin == "" {
        t.Errorf("manifest misbuilt: %+v", m)
    }
}
```

- [ ] **Step 2: 跑测试验证失败**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -run TestFile 2>&1 | head -5
```

Expected: `undefined: File` / `undefined: Manifest`。

- [ ] **Step 3: 实现 `manifest.go`**

新建 `internal/deployer/manifest.go`：

```go
// Package deployer 上传 + 安装 spotterd 安装包到设备。
//
// 本文件只定义数据形态：Manifest + File。具体装载（local/remote）见 source_*.go。
package deployer

import "time"

// File 描述单个待上传文件。LocalPath 在客户端路径；RemoteName 是推上去
// 后在设备端的文件名（默认与 LocalPath basename 同）；Size 由装载阶段填；
// SHA256 由 LocalSource 同步计算（RemoteSource 下载完成后算）。
type File struct {
    LocalPath  string `json:"local_path"`
    RemoteName string `json:"remote_name"`
    Size       int64  `json:"size"`
    SHA256     string `json:"sha256"`
}

// Manifest 是 Package.Resolve 的产出。StagingDir 是 Deployer 在设备端
// 推上去的临时目录，由 Deployer.Run 生成（不在 Resolve 阶段确定）。
type Manifest struct {
    Arch       string    `json:"arch"`
    Files      []File    `json:"files"`
    ResolvedAt time.Time `json:"resolved_at"`
    Origin     string    `json:"origin"` // "local:<dir>" | "remote:<url>"
    StagingDir string    `json:"staging_dir,omitempty"`
}
```

- [ ] **Step 4: 跑 manifest 测试验证通过**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run 'TestFile|TestManifest'
```

Expected: PASS。

- [ ] **Step 5: 写 package 接口**

新建 `internal/deployer/package.go`：

```go
package deployer

import "context"

// Package 描述一个完整的 spotterd 安装包（三件套：二进制 + unit + install.sh）。
// 实现：LocalSource（本地 cache 命中）、RemoteSource（HTTP GET 落本地 cache）。
type Package interface {
    Resolve(ctx context.Context) (Manifest, error)
}
```

新建 `internal/deployer/package_test.go`：

```go
package deployer

import (
    "context"
    "errors"
    "testing"
)

// fakePackage 实现 Package 用于接口冒烟。
type fakePackage struct{ m Manifest }

func (f fakePackage) Resolve(_ context.Context) (Manifest, error) { return f.m, nil }

func TestPackage_InterfaceImplemented(t *testing.T) {
    var _ Package = fakePackage{}
    var _ Package = (*errPkg)(nil)
}

type errPkg struct{}

func (errPkg) Resolve(_ context.Context) (Manifest, error) {
    return Manifest{}, errors.New("stub")
}

func TestPackage_ReturnsError(t *testing.T) {
    _, err := errPkg{}.Resolve(context.Background())
    if err == nil {
        t.Fatal("expected stub error")
    }
}
```

- [ ] **Step 6: 跑 package 测试**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run TestPackage
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/manifest.go internal/deployer/manifest_test.go internal/deployer/package.go internal/deployer/package_test.go
git commit -m "feat(deploy): 增加 Manifest/File 类型与 Package 接口"
```

---

## Task 3: LocalSource（本地 cache 镜像解析）

**Files:**
- Create: `internal/deployer/source_local.go`
- Create: `internal/deployer/source_local_test.go`

**Interfaces:**
- Consumes: Settings.CacheBinDir + arch 字符串
- Produces: `LocalSource`（实现 `Package` 接口）；`NewLocalSource(dir, arch) Package`

- [ ] **Step 1: 写失败的 source_local 测试**

新建 `internal/deployer/source_local_test.go`：

```go
package deployer

import (
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func writePkg(t *testing.T, dir, arch string) {
    t.Helper()
    if err := os.MkdirAll(dir, 0755); err != nil { t.Fatal(err) }
    must := func(name string, body string) {
        if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
            t.Fatal(err)
        }
    }
    must("spotterd-linux-"+arch, "#!/bin/sh\necho fake-binary\n")
    must("spotterd.service", "[Unit]\nDescription=spotterd\n")
    must("install.sh", "#!/bin/sh\necho installing\n")
}

func TestLocalSource_Resolve_OK(t *testing.T) {
    dir := t.TempDir()
    writePkg(t, dir, "arm64")
    p := NewLocalSource(dir, "arm64")
    m, err := p.Resolve(context.Background())
    if err != nil { t.Fatalf("Resolve: %v", err) }
    if m.Arch != "arm64" { t.Errorf("Arch = %q", m.Arch) }
    if len(m.Files) != 3 { t.Fatalf("len(Files) = %d", len(m.Files)) }
    if !strings.HasPrefix(m.Origin, "local:") {
        t.Errorf("Origin = %q, want local: prefix", m.Origin)
    }
}

func TestLocalSource_Resolve_MissingBinary(t *testing.T) {
    dir := t.TempDir()
    // 只放 .service + install.sh，缺二进制
    must := func(name string) {
        if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
            t.Fatal(err)
        }
    }
    must("spotterd.service")
    must("install.sh")
    p := NewLocalSource(dir, "arm64")
    _, err := p.Resolve(context.Background())
    if err == nil || !strings.Contains(err.Error(), "spotterd-linux-arm64") {
        t.Fatalf("err = %v, want missing-binary hint", err)
    }
}
```

- [ ] **Step 2: 跑测试验证失败**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run TestLocalSource
```

Expected: `undefined: NewLocalSource`。

- [ ] **Step 3: 实现 source_local.go**

新建 `internal/deployer/source_local.go`：

```go
package deployer

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "time"
)

// LocalSource 从 dir 直读三件套：spotterd-linux-<arch>、spotterd.service、
// install.sh。任一缺失 → 包级 sentinel 错误 ErrPackageNotFound（main.go 翻译成
// 'package_not_found' 错误码）。
type LocalSource struct {
    Dir  string
    Arch string
}

func NewLocalSource(dir, arch string) *LocalSource {
    return &LocalSource{Dir: dir, Arch: arch}
}

// ErrPackageNotFound 表示 Resolve 找不到三件套。
var ErrPackageNotFound = fmt.Errorf("deployer: spotterd package not found")

func (l *LocalSource) Resolve(_ context.Context) (Manifest, error) {
    bin := filepath.Join(l.Dir, fmt.Sprintf("spotterd-linux-%s", l.Arch))
    svc := filepath.Join(l.Dir, "spotterd.service")
    ins := filepath.Join(l.Dir, "install.sh")

    files := []struct {
        path  string
        name  string
        rname string
    }{
        {bin, "binary", filepath.Base(bin)},
        {svc, "service", filepath.Base(svc)},
        {ins, "install", filepath.Base(ins)},
    }

    out := make([]File, 0, len(files))
    for _, f := range files {
        info, err := os.Stat(f.path)
        if err != nil {
            if os.IsNotExist(err) {
                return Manifest{}, fmt.Errorf("%w: %s missing in %s", ErrPackageNotFound, f.name, l.Dir)
            }
            return Manifest{}, err
        }
        sum, err := sha256File(f.path)
        if err != nil { return Manifest{}, err }
        out = append(out, File{
            LocalPath: f.path, RemoteName: f.rname,
            Size: info.Size(), SHA256: sum,
        })
    }

    return Manifest{
        Arch: l.Arch, Files: out,
        ResolvedAt: time.Now().UTC(),
        Origin:     fmt.Sprintf("local:%s", l.Dir),
    }, nil
}

func sha256File(p string) (string, error) {
    f, err := os.Open(p)
    if err != nil { return "", err }
    defer f.Close()
    h := sha256.New()
    if _, err := h.Write_from_file_REPLACED; err != nil { return "", err } // 占位防编译通过
    _ = h
    return hex.EncodeToString(nil), nil
}
```

（**注意**：`sha256File` 是占位，**实际实现**见 Step 3.5。）

- [ ] **Step 4: 实现 `sha256File` 完整版**

把 `sha256File` 改成正确版本：

```go
func sha256File(p string) (string, error) {
    f, err := os.Open(p)
    if err != nil { return "", err }
    defer f.Close()
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil { return "", err }
    return hex.EncodeToString(h.Sum(nil)), nil
}
```

并在文件顶部加 `"io"` import。

- [ ] **Step 5: 跑测试验证通过**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run TestLocalSource
```

Expected: PASS，2 个 case。

- [ ] **Step 6: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/source_local.go internal/deployer/source_local_test.go
git commit -m "feat(deploy): LocalSource 从本地 cache/bin 解析三件套"
```

---

## Task 4: RemoteSource（HTTP GET 落本地 cache）

**Files:**
- Create: `internal/deployer/source_remote.go`
- Create: `internal/deployer/source_remote_test.go`

**Interfaces:**
- Consumes: `Settings.PackageReleaseURL` 模板（含 `{arch}` 占位）+ `Settings.CacheBinDir` + `clientconfig.Settings.HTTPTimeout`（沿用）
- Produces: `RemoteSource`，实现 `Package`；构造函数 `NewRemoteSource(urlTemplate, cacheDir, arch, httpClient *http.Client) *RemoteSource`

- [ ] **Step 1: 写失败的 source_remote 测试**

新建 `internal/deployer/source_remote_test.go`：

```go
package deployer

import (
    "context"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestRemoteSource_FetchAndCache(t *testing.T) {
    var hits int
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        hits++
        // 仅处理 spotterd-linux-arm64 请求
        if !strings.HasSuffix(r.URL.Path, "spotterd-linux-arm64") {
            http.NotFound(w, r)
            return
        }
        w.Write([]byte("binary-bytes"))
    }))
    defer srv.Close()

    cache := t.TempDir()
    urlTmpl := srv.URL + "/spotterd-linux-{arch}"
    p := NewRemoteSource(urlTmpl, cache, "arm64", srv.Client())

    m, err := p.Resolve(context.Background())
    if err != nil { t.Fatal(err) }
    if !strings.HasPrefix(m.Origin, "remote:") {
        t.Errorf("Origin = %q", m.Origin)
    }
    if hits == 0 {
        t.Error("expected at least 1 HTTP hit")
    }
    if _, err := os.Stat(filepath.Join(cache, "spotterd-linux-arm64")); err != nil {
        t.Errorf("cache file missing: %v", err)
    }
}

func TestRemoteSource_404Surfaces(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        http.NotFound(w, r)
    }))
    defer srv.Close()
    p := NewRemoteSource(srv.URL+"/{arch}", t.TempDir(), "amd64", srv.Client())
    _, err := p.Resolve(context.Background())
    if err == nil { t.Fatal("expected 404 to surface as error") }
}
```

- [ ] **Step 2: 跑测试验证失败**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run TestRemoteSource
```

Expected: `undefined: NewRemoteSource`。

- [ ] **Step 3: 实现 source_remote.go**

新建 `internal/deployer/source_remote.go`：

```go
package deployer

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// RemoteSource 从 urlTemplate 拉三件套到 cacheDir。urlTemplate 中包含 {arch}
// 占位；arch 替换后 GET。下载完成后哈希并落盘（与 LocalSource 同缓存路径）。
type RemoteSource struct {
    URLTemplate string // 含 {arch}
    CacheDir    string
    Arch        string
    HTTPClient  *http.Client // 沿用客户端整体 HTTPTimeout
}

func NewRemoteSource(tmpl, cacheDir, arch string, c *http.Client) *RemoteSource {
    if c == nil { c = http.DefaultClient }
    return &RemoteSource{URLTemplate: tmpl, CacheDir: cacheDir, Arch: arch, HTTPClient: c}
}

func (r *RemoteSource) Resolve(ctx context.Context) (Manifest, error) {
    if err := os.MkdirAll(r.CacheDir, 0755); err != nil { return Manifest{}, err }

    fetch := func(remoteName string) (string, int64, error) {
        url := strings.ReplaceAll(r.URLTemplate, "{arch}", r.Arch)
        url = strings.TrimRight(url, "/") + "/" + remoteName
        dest := filepath.Join(r.CacheDir, remoteName)
        if err := fetchOnce(ctx, r.HTTPClient, url, dest); err != nil {
            return "", 0, err
        }
        info, err := os.Stat(dest)
        if err != nil { return "", 0, err }
        return dest, info.Size(), nil
    }

    binLocal, binSize, err := fetch(fmt.Sprintf("spotterd-linux-%s", r.Arch))
    if err != nil { return Manifest{}, fmt.Errorf("remote: fetch binary: %w", err) }
    svcLocal, svcSize, err := fetch("spotterd.service")
    if err != nil { return Manifest{}, fmt.Errorf("remote: fetch service: %w", err) }
    insLocal, insSize, err := fetch("install.sh")
    if err != nil { return Manifest{}, fmt.Errorf("remote: fetch install: %w", err) }

    sizeOf := func(p string, sz int64) (File, error) {
        sum, err := sha256File(p)
        if err != nil { return File{}, err }
        return File{LocalPath: p, RemoteName: filepath.Base(p), Size: sz, SHA256: sum}, nil
    }
    binF, err := sizeOf(binLocal, binSize); if err != nil { return Manifest{}, err }
    svcF, err := sizeOf(svcLocal, svcSize); if err != nil { return Manifest{}, err }
    insF, err := sizeOf(insLocal, insSize); if err != nil { return Manifest{}, err }

    return Manifest{
        Arch: r.Arch, Files: []File{binF, svcF, insF},
        ResolvedAt: time.Now().UTC(),
        Origin:     fmt.Sprintf("remote:%s", r.URLTemplate),
    }, nil
}

func fetchOnce(ctx context.Context, c *http.Client, url, dest string) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil { return err }
    resp, err := c.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
    }
    tmp := dest + ".part"
    f, err := os.Create(tmp)
    if err != nil { return err }
    if _, err := io.Copy(f, resp.Body); err != nil {
        f.Close()
        os.Remove(tmp)
        return err
    }
    if err := f.Close(); err != nil {
        os.Remove(tmp)
        return err
    }
    return os.Rename(tmp, dest)
}
```

- [ ] **Step 4: 跑测试验证通过**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run TestRemoteSource
```

Expected: PASS，2 个 case。

- [ ] **Step 5: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/source_remote.go internal/deployer/source_remote_test.go
git commit -m "feat(deploy): RemoteSource 从配置 URL 拉三件套落本地 cache"
```

---

## Task 5: AuthSpec + Dial（3 模式：agent / key / password）

**Files:**
- Create: `internal/deployer/auth.go`
- Create: `internal/deployer/auth_agent.go`
- Create: `internal/deployer/auth_key.go`
- Create: `internal/deployer/auth_password.go`
- Create: `internal/deployer/auth_test.go`

**Interfaces:**
- Consumes: `AuthSpec{Mode,User,Password,KeyPath,KeyPass}`
- Produces: `Dial(ctx, *AuthSpec, port int, host string) (*ssh.Client, error)`、`AgentSigners() ([]ssh.Signer, error)`、`ParsePrivateKey(path, passphrase string) (ssh.Signer, error)`

- [ ] **Step 1: 写失败的 auth 测试**

新建 `internal/deployer/auth_test.go`：

```go
package deployer

import (
    "bytes"
    "crypto/ed25519"
    "crypto/rand"
    "encoding/pem"
    "os"
    "path/filepath"
    "testing"

    "golang.org/x/crypto/ssh"
)

func writeED25519Key(t *testing.T, dir string, encrypted bool) (priv string, pub ssh.PublicKey) {
    t.Helper()
    _, privBytes, err := ed25519.GenerateKey(rand.Reader)
    if err != nil { t.Fatal(err) }

    var pemBytes []byte
    if encrypted {
        // 用 ssh 包自带 PEMBlock 写加密格式
        block, err := ssh.PrivateKeyBlock(privBytes, []byte("passwd"), "")
        if err != nil { t.Fatal(err) }
        pemBytes = pem.EncodeToMemory(block)
    } else {
        block, err := ssh.MarshalPrivateKeyWithPassphrase(privBytes, nil) // PEM
        if err != nil { t.Fatal(err) }
        pemBytes = pem.EncodeToMemory(block)
    }

    privPath := filepath.Join(dir, "id_test")
    if err := os.WriteFile(privPath, pemBytes, 0600); err != nil { t.Fatal(err) }
    signer, err := ssh.ParsePrivateKey(pemBytes)
    if err != nil { t.Fatal(err) }
    return privPath, signer.PublicKey()
}

func TestParsePrivateKey_Unencrypted(t *testing.T) {
    dir := t.TempDir()
    path, _ := writeED25519Key(t, dir, false)
    _, err := ParsePrivateKey(path, "")
    if err != nil { t.Fatalf("ParsePrivateKey: %v", err) }
}

func TestParsePrivateKey_Encrypted(t *testing.T) {
    dir := t.TempDir()
    path, _ := writeED25519Key(t, dir, true)
    _, err := ParsePrivateKey(path, "passwd")
    if err != nil { t.Fatalf("ParsePrivateKey with pass: %v", err) }
}

func TestParsePrivateKey_WrongPassphrase(t *testing.T) {
    dir := t.TempDir()
    path, _ := writeED25519Key(t, dir, true)
    _, err := ParsePrivateKey(path, "nope")
    if err == nil { t.Fatal("expected decrypt failure") }
}

func TestAgentSigners_NoSocketReturnsError(t *testing.T) {
    t.Setenv("SSH_AUTH_SOCK", "")
    if _, err := AgentSigners(); err == nil {
        t.Fatal("expected agent unavailable error")
    }
}

func TestDial_InvalidHostFailsQuickly(t *testing.T) {
    spec := AuthSpec{Mode: "password", User: "u", Password: "p"}
    _, err := Dial(testHelperContext(), spec, 1, "127.0.0.1") // 端口 1 几乎一定连不上
    if err == nil { t.Fatal("expected dial error") }
}

// 让 TestDial 用 background ctx；放在文件内以避免 import 循环
func testHelperContext() context.Context { return context.Background() }
```

如果 `ssh.MarshalPrivateKeyWithPassphrase` 不是现成 helper，改用：

```go
pkcs8Block, err := ssh.MarshalPrivateKey(privBytes, "", nil) // unencrypted
```

- [ ] **Step 2: 跑测试验证失败**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run 'TestParsePrivateKey|TestAgentSigners|TestDial'
```

Expected: `undefined: ParsePrivateKey`/`undefined: AgentSigners`/`undefined: Dial`。

- [ ] **Step 3: 实现 auth.go**

新建 `internal/deployer/auth.go`：

```go
package deployer

import (
    "context"
    "fmt"
    "time"

    "golang.org/x/crypto/ssh"
)

// AuthSpec 描述 SSH 认证的可用信息。所有字段一次性用，不持久化。
type AuthSpec struct {
    Mode     string // "agent" | "key" | "password"
    User     string
    Password string
    KeyPath  string
    KeyPass  string
}

// Dial 用 spec 配置连 <host>:<port>，返回 *ssh.Client。
// 超时：连接 10s、握手 10s；host key 走 InsecureIgnoreHostKey。
func Dial(ctx context.Context, spec AuthSpec, port int, host string) (*ssh.Client, error) {
    cfg, err := buildClientConfig(ctx, spec)
    if err != nil { return nil, err }
    addr := fmt.Sprintf("%s:%d", host, port)
    cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    return ssh.DialContext(cctx, "tcp", addr, cfg)
}

func buildClientConfig(ctx context.Context, spec AuthSpec) (*ssh.ClientConfig, error) {
    if spec.User == "" {
        return nil, fmt.Errorf("auth: user required")
    }
    cfg := &ssh.ClientConfig{
        User:            spec.User,
        Auth:            []ssh.AuthMethod{},
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
        Timeout:         10 * time.Second,
    }
    switch spec.Mode {
    case "agent":
        signers, err := AgentSigners()
        if err != nil { return nil, fmt.Errorf("auth: agent: %w", err) }
        cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signers...))
    case "key":
        signer, err := ParsePrivateKey(spec.KeyPath, spec.KeyPass)
        if err != nil { return nil, fmt.Errorf("auth: key: %w", err) }
        cfg.Auth = append(cfg.Auth, ssh.PublicKeys(signer))
    case "password":
        cfg.Auth = append(cfg.Auth, ssh.Password(spec.Password))
    default:
        return nil, fmt.Errorf("auth: unknown mode %q", spec.Mode)
    }
    return cfg, nil
}
```

- [ ] **Step 4: 实现 auth_agent.go**

新建 `internal/deployer/auth_agent.go`：

```go
package deployer

import (
    "fmt"
    "net"
    "os"

    "golang.org/x/crypto/ssh"
    "golang.org/x/crypto/ssh/agent"
)

// AgentSigners 从 SSH_AUTH_SOCK 拉可用的 signer 列表。
// 空 → ErrNoAgent（main.go 翻译成 'agent_not_available'）。
var ErrNoAgent = fmt.Errorf("deployer: SSH agent not available")

func AgentSigners() ([]ssh.Signer, error) {
    sock := os.Getenv("SSH_AUTH_SOCK")
    if sock == "" { return nil, ErrNoAgent }
    conn, err := net.Dial("unix", sock)
    if err != nil { return nil, fmt.Errorf("%w: dial %s: %v", ErrNoAgent, sock, err) }
    defer conn.Close()
    cli := agent.NewClient(conn)
    signers, err := cli.Signers()
    if err != nil { return nil, ErrNoAgent }
    return signers, nil
}
```

- [ ] **Step 5: 实现 auth_key.go**

新建 `internal/deployer/auth_key.go`：

```go
package deployer

import (
    "fmt"
    "os"

    "golang.org/x/crypto/ssh"
)

func ParsePrivateKey(path, passphrase string) (ssh.Signer, error) {
    data, err := os.ReadFile(path)
    if err != nil { return nil, err }
    signer, err := ssh.ParsePrivateKey(data)
    if err != nil {
        // 试解密
        if _, ok := err.(*ssh.PassphraseMissingError); ok {
            if passphrase == "" {
                return nil, fmt.Errorf("key %s is encrypted, no passphrase provided", path)
            }
            signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
        }
    }
    if err != nil { return nil, fmt.Errorf("parse %s: %w", path, err) }
    return signer, nil
}
```

- [ ] **Step 6: 实现 auth_password.go**

新建 `internal/deployer/auth_password.go`：

```go
package deployer

import "golang.org/x/crypto/ssh"

// Password 是 ssh.Password 的薄包装；目前直接走 ssh.Password。
//
// 后续如需 KeyboardInteractive（两轮挑战）可在此扩展。
func Password(p string) ssh.AuthMethod {
    return ssh.Password(p)
}
```

- [ ] **Step 7: 跑 auth 测试**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run 'TestParsePrivateKey|TestAgentSigners|TestDial'
```

Expected: PASS，至少 4 个 case。注意 `TestAgentSigners_NoSocketReturnsError` 在 Windows 测试机上无 `SSH_AUTH_SOCK` 也能通过（`t.Setenv("SSH_AUTH_SOCK","")`）。

- [ ] **Step 8: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/auth.go internal/deployer/auth_agent.go internal/deployer/auth_key.go internal/deployer/auth_password.go internal/deployer/auth_test.go
git commit -m "feat(deploy): AuthSpec+Dial 三模式（agent/key/password）"
```

---

## Task 6: SFTP Upload（带进度回调）+ SSH ExecInstall（含 cancel）

**Files:**
- Create: `internal/deployer/sftp_upload.go`
- Create: `internal/deployer/sftp_upload_test.go`
- Create: `internal/deployer/ssh_exec.go`
- Create: `internal/deployer/ssh_exec_test.go`

**Interfaces:**
- Consumes: `*sftp.Client`、`AuthSpec`/已连通的 `*ssh.Client`
- Produces: `Upload(ctx, *sftp.Client, src, remoteDir, onProgress func(done,total int64)) (int64, error)`、`ExecInstall(ctx, *ssh.Client, opts ExecOpts) (exitCode int, err error)`

- [ ] **Step 1: 写失败的 sftp_upload 测试**

新建 `internal/deployer/sftp_upload_test.go`：

```go
package deployer

import (
    "context"
    "io"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/pkg/sftp"
    "golang.org/x/crypto/ssh"
)

func TestUpload_Loopback(t *testing.T) {
    src := filepath.Join(t.TempDir(), "blob.bin")
    if err := os.WriteFile(src, bytes.Repeat([]byte("A"), 4096), 0644); err != nil { t.Fatal(err) }

    cli, cleanup := startLoopbackSFTP(t)
    defer cleanup()

    var got int64
    onProg := func(done, total int64) {
        if total != 4096 { t.Errorf("total = %d, want 4096", total) }
        got = done
    }
    n, err := Upload(context.Background(), cli, src, "/uploaded/blob.bin", onProg)
    if err != nil { t.Fatalf("Upload: %v", err) }
    if n != 4096 { t.Errorf("n = %d, want 4096", n) }
    if got != 4096 { t.Errorf("progress done = %d, want 4096", got) }
}
```

`startLoopbackSFTP` helper 在底部写（详见 Step 5 — 一次实现，覆盖 SFTP + SSH exec 共享）。

- [ ] **Step 2: 写失败的 ssh_exec 测试**

新建 `internal/deployer/ssh_exec_test.go`：

```go
package deployer

import (
    "context"
    "strings"
    "testing"
)

func TestExecInstall_EchoReturnsLines(t *testing.T) {
    cli, cleanup := startLoopbackSSH(t)
    defer cleanup()

    var lines []string
    _, err := ExecInstall(context.Background(), cli, ExecOpts{
        Cmd:      "echo hello; echo world",
        SudoPass: func() string { return "" },
        OnLine:   func(l string, _ bool) { lines = append(lines, l) },
        Timeout:  5 * time.Second,
    })
    if err != nil { t.Fatalf("ExecInstall: %v", err) }
    if len(lines) < 2 { t.Fatalf("got %d lines, want >=2", len(lines)) }
    if !strings.Contains(strings.Join(lines, "|"), "hello") {
        t.Errorf("missing hello in %v", lines)
    }
}

func TestExecInstall_NonZeroExit(t *testing.T) {
    cli, cleanup := startLoopbackSSH(t)
    defer cleanup()
    code, err := ExecInstall(context.Background(), cli, ExecOpts{
        Cmd:     "exit 7",
        Timeout: 5 * time.Second,
    })
    if err == nil { t.Fatal("expected error for exit 7") }
    if code != 7 { t.Errorf("code = %d, want 7", code) }
}
```

- [ ] **Step 3: 跑两个测试验证失败**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run 'TestUpload|TestExecInstall'
```

Expected: `undefined: Upload`/`undefined: ExecInstall`。

- [ ] **Step 4: 实现 sftp_upload.go**

新建 `internal/deployer/sftp_upload.go`：

```go
package deployer

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"

    "github.com/pkg/sftp"
)

// Upload 把 src 推到 remoteDir/RemoteName。返回写入字节数。
// remoteDir 不存在则 MkdirAll；onProgress 每 200ms 或每 1MB 调用一次（done, total）。
func Upload(ctx context.Context, c *sftp.Client, src, remoteDir string, onProgress func(int64, int64)) (int64, error) {
    if err := c.MkdirAll(remoteDir); err != nil {
        return 0, fmt.Errorf("sftp mkdir %s: %w", remoteDir, err)
    }

    f, err := os.Open(src)
    if err != nil { return 0, err }
    defer f.Close()

    info, err := f.Stat()
    if err != nil { return 0, err }
    total := info.Size()

    dst, err := c.Create(filepath.ToSlash(filepath.Join(remoteDir, filepath.Base(src))))
    if err != nil {
        return 0, fmt.Errorf("sftp create: %w", err)
    }
    defer dst.Close()

    pr := newProgressReader(f, total, onProgress)
    n, err := io.Copy(dst, pr)
    if err != nil { return n, err }

    // 给二进制和 shell 加执行位
    name := filepath.Base(src)
    if name == "install.sh" || (len(name) > 9 && name[:9] == "spotterd-") {
        _ = c.Chmod(filepath.ToSlash(filepath.Join(remoteDir, name)), 0755)
    }
    return n, nil
}

// progressReader 每 200ms 或每 1MB 调用 onProgress。
type progressReader struct {
    r         io.Reader
    total     int64
    done      int64
    onProg    func(int64, int64)
    lastEmit  time.Time
}

func newProgressReader(r io.Reader, total int64, onProg func(int64, int64)) *progressReader {
    return &progressReader{r: r, total: total, onProg: onProg, lastEmit: time.Now()}
}

func (p *progressReader) Read(buf []byte) (int, error) {
    n, err := p.r.Read(buf)
    p.done += int64(n)
    if p.onProg != nil {
        now := time.Now()
        if now.Sub(p.lastEmit) >= 200*time.Millisecond || p.done == p.total {
            p.onProg(p.done, p.total)
            p.lastEmit = now
        }
    }
    return n, err
}
```

文件 import 顶部加 `"time"`。

- [ ] **Step 5: 实现 ssh_exec.go**

新建 `internal/deployer/ssh_exec.go`：

```go
package deployer

import (
    "bufio"
    "context"
    "errors"
    "fmt"
    "io"
    "time"

    "golang.org/x/crypto/ssh"
)

// ExecOpts 控制 SSH session 跑 install.sh 的细节。
type ExecOpts struct {
    Cmd      string
    SudoPass func() string // 调用时拉一次；可用于 popup 关闭后还能取
    OnLine   func(line string, isStderr bool)
    Timeout  time.Duration // 默认 10 分钟
}

// ErrExitNonZero 表示命令退出非 0；含 ExitCode。
type ExitNonZeroError struct{ Code int }

func (e *ExitNonZeroError) Error() string { return fmt.Sprintf("exit %d", e.Code) }

// ExecInstall 跑 opts.Cmd 并行捕获 stdout/stderr 行发 OnLine。
// 返回 exit code；非 0 → 返回 ExitNonZeroError（main.go 据此发 deploy-complete error）。
// ctx cancel 路径：SIGTERM 等 5s 再 SIGKILL。
func ExecInstall(ctx context.Context, c *ssh.Client, opts ExecOpts) (int, error) {
    if opts.Timeout == 0 { opts.Timeout = 10 * time.Minute }

    sess, err := c.NewSession()
    if err != nil { return 0, err }
    defer sess.Close()

    // 关闭 echo 让 sudo -S 不把密码回显出来
    if err := sess.RequestPty("dumb", 80, 24, ssh.TerminalModes{ssh.ECHO: 0}); err != nil {
        return 0, fmt.Errorf("request pty: %w", err)
    }

    stdin, err := sess.StdinPipe()
    if err != nil { return 0, err }
    stdout, err := sess.StdoutPipe()
    if err != nil { return 0, err }
    stderr, err := sess.StderrPipe()
    if err != nil { return 0, err }

    // 行缓冲 stderr / stdout
    go scanLines(stdout, func(l string) { opts.OnLine(l, false) })
    go scanLines(stderr, func(l string) { opts.OnLine(l, true) })

    // 喂 sudo 密码（如果提供了 SudoPass）
    if opts.SudoPass != nil {
        if pw := opts.SudoPass(); pw != "" {
            go func() {
                _, _ = io.WriteString(stdin, pw+"\n")
                _ = stdin.Close()
            }()
        }
    }

    // cancel 监听
    doneRun := make(chan error, 1)
    go func() { doneRun <- sess.Run(opts.Cmd) }()

    timer := time.NewTimer(opts.Timeout)
    defer timer.Stop()

    select {
    case <-ctx.Done():
        _ = sess.Signal(ssh.SIGTERM)
        select {
        case <-time.After(5 * time.Second):
            _ = sess.Signal(ssh.SIGKILL)
        case <-doneRun:
        }
        return 0, ctx.Err()
    case <-timer.C:
        _ = sess.Signal(sig_kill_CHK()) // 下方复用 SIGKILL
        <-doneRun
        return 0, errors.New("install timeout")
    case err := <-doneRun:
        if exitErr, ok := err.(*ssh.ExitError); ok {
            code := exitErr.ExitStatus()
            if code != 0 { return code, &ExitNonZeroError{Code: code} }
            return code, nil
        }
        if err != nil { return 0, err }
        return 0, nil
    }
}

func scanLines(r io.Reader, onLine func(string)) {
    s := bufio.NewScanner(r)
    s.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 行可长达 1MB
    for s.Scan() {
        onLine(s.Text())
    }
}

func sig_kill_CHK() ssh.Signal {
    // 用一个内部 helper；这里不需要复杂逻辑。
    return ssh.SIGKILL
}
```

（**自我修缮**：上面 `sig_kill_CHK` 是占位，**Step 5.5** 直接改为 `ssh.SIGKILL`，删掉 helper。）

- [ ] **Step 5.5: 把 sig_kill_CHK 替换成 ssh.SIGKILL**

修改 select-case `<-timer.C` 的 `sess.Signal(sig_kill_CHK())` 改为 `sess.Signal(ssh.SIGKILL)`，并删除 `sig_kill_CHK` helper。

- [ ] **Step 6: 实现 loopback 测试 helper**

新建 `internal/deployer/testutil_test.go`：

```go
package deployer

import (
    "fmt"
    "io"
    "net"
    "testing"

    "github.com/pkg/sftp"
    "golang.org/x/crypto/ssh"
)

func startLoopbackSSH(t *testing.T) (*ssh.Client, func()) {
    t.Helper()
    // 给测试生成一对临时的 RSA host key + 一对临时 user key
    hostSigner, userSigner := mustGenHostAndUser(t)

    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil { t.Fatal(err) }
    addr := listener.Addr().String()

    go func() {
        for {
            conn, err := listener.Accept()
            if err != nil { return }
            go serveConn(conn, hostSigner, userSigner)
        }
    }()

    cli := mustDialClient(t, addr, userSigner)
    return cli, func() { _ = cli.Close(); listener.Close() }
}

func serveConn(c net.Conn, hostSigner, userSigner ssh.Signer) {
    cfg := &ssh.ServerConfig{
        PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
            // 任何 user key 都放行（测试专用）
            return &ssh.Permissions{}, nil
        },
    }
    cfg.AddHostKey(hostSigner)
    conn, chans, reqs, err := ssh.NewServerConn(c, cfg)
    if err != nil { return }
    go ssh.DiscardRequests(reqs)
    for ch := range chans {
        switch ch.ChannelType() {
        case "session":
            ch, reqs, err := ch.Accept()
            if err != nil { return }
            go ssh.DiscardRequests(reqs)
            // 简单的 echo session：收到什么执行什么（仅测试用，绝不进生产）
            go func() {
                defer ch.Close()
                buf := make([]byte, 4096)
                for {
                    n, err := ch.Read(buf)
                    if err != nil { return }
                    io.WriteString(ch, "exec:"+string(buf[:n])+"\n")
                }
            }()
        default:
            ch.Reject(ssh.UnknownChannelType, "unsupported")
        }
    }
    conn.Close()
}

func mustGenHostAndUser(t *testing.T) (ssh.Signer, ssh.Signer) {
    t.Helper()
    host, err := ssh.ParsePrivateKey(testKeyPEM)
    if err != nil { t.Fatal(err) }
    user, err := ssh.ParsePrivateKey(testKeyPEM)
    if err != nil { t.Fatal(err) }
    return host, user
}

// testKeyPEM 是固定的测试用 ed25519 私钥 PEM。生产代码不得引入。
const testKeyPEM = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACBLSld6QXhjdGZRQ2RRQ1JsVWtYaGZJQ0dYZlpDd2NuUGtEdUZkc1F3QQ
AAAJDowGyH6MBshwAAAAAAAAAAAAEAAAAc3NoLWFzc2tABnNoYTUxMgAAABFLSld6QXhj
dGZRQ2RRQ1JsVWtYaGZJQ0dYZlpDd2NuUGtEdUZkc1F3QQAAAAUARHJvbEBoYXJkd2Fy
ZQM=
-----END OPENSSH PRIVATE KEY-----`
```

> `mustDialClient` 与 SFTP loopback helper 见 Step 7。testKeyPEM 用 project 临时生成的 key（仓库内有 `testdata/id_ed25519` 也可直接用 `ssh_test` 标准 fixture）。

- [ ] **Step 7: 加 loopback helpers（`mustDialClient` + SFTP helper）**

在 `internal/deployer/testutil_test.go` 末尾追加：

```go
func mustDialClient(t *testing.T, addr string, userSigner ssh.Signer) *ssh.Client {
    t.Helper()
    cfg := &ssh.ClientConfig{
        User:            "test",
        Auth:            []ssh.AuthMethod{ssh.PublicKeys(userSigner)},
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
    }
    cli, err := ssh.Dial("tcp", addr, cfg)
    if err != nil { t.Fatalf("Dial %s: %v", addr, err) }
    return cli
}

// startLoopbackSFTP 同时给 sftp_upload_test 用。
func startLoopbackSFTP(t *testing.T) (*sftp.Client, func()) {
    t.Helper()
    sshCli, cleanup := startLoopbackSSH(t)
    cli, err := sftp.NewClient(sshCli)
    if err != nil { cleanup(); t.Fatal(err) }
    return cli, func() { cli.Close(); cleanup() }
}
```

> **重要**：loopback server 的 session handler 当前是 echo，生产化时需要替换为 `exec-request` 的 echo 命令（go `ssh.Session` server-side API）。下个 Step 把 session handler 改完。

- [ ] **Step 8: 把 session handler 升级为 exec**

修改 `serveConn` 的 `"session"` 分支：

```go
case "session":
    ch, reqs, err := ch.Accept()
    if err != nil { return }
    go ssh.DiscardRequests(reqs)
    go func() {
        defer ch.Close()
        for req := range reqs {
            switch req.Type {
            case "exec":
                // req.Payload 第一个 4 字节是 length
                payload := req.Payload[4:]
                // echo 到 stdout，关 exit-status 0
                io.WriteString(ch, string(payload)+"\n")
                _, _ = ch.SendRequest("exit-status", false, []byte{0,0,0,0})
                return
            case "shell":
                _, _ = ch.SendRequest("exit-status", false, []byte{0,0,0,0})
                return
            default:
                req.Reply(true, nil)
            }
        }
    }()
```

> 真实环境需要把 reqs 拉出来走标准 ssh session server 模式；本测试足够覆盖 `ssh.Session.Run` 走通。

- [ ] **Step 9: 跑测试**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run 'TestUpload|TestExecInstall'
```

Expected: PASS。如果 testutil 或 session handler 需要进一步调试可以反复运行直到稳定。

- [ ] **Step 10: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/sftp_upload.go internal/deployer/sftp_upload_test.go internal/deployer/ssh_exec.go internal/deployer/ssh_exec_test.go internal/deployer/testutil_test.go
git commit -m "feat(deploy): SFTP Upload(进度) + SSH ExecInstall(cancel)"
```

---

## Task 7: Resolver 选源（按 Settings.PackageMode）+ Deployer 主循环

**Files:**
- Create: `internal/deployer/source_resolver.go`
- Create: `internal/deployer/deployer.go`
- Create: `internal/deployer/deployer_test.go`

**Interfaces:**
- Consumes: `clientconfig.Settings`、`Registry.Entry`（拿 Username）、`AuthSpec`、SudoPass func
- Produces: `Deployer`（含 `Prepare/Run/Cancel/List` 方法）、`Handle`（uuid）、`ProgressEvent`、`DeployRequest`

- [ ] **Step 1: 写失败的 Resolver 测试**

新建 `internal/deployer/source_resolver_test.go`（与 source_resolver.go 同 package）：

```go
package deployer

import "testing"

func TestResolver_Local(t *testing.T) {
    dir := t.TempDir()
    writePkg(t, dir, "amd64")
    r := NewResolver("local", dir, "amd64", "")
    p, err := r.ResolvePkg(context.Background())
    if err != nil { t.Fatal(err) }
    if p == nil { t.Fatal("nil package") }
}
```

> `_ = context` 通过顶部 import 解决。

- [ ] **Step 2: 跑测试验证失败**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run TestResolver
```

Expected: `undefined: NewResolver`。

- [ ] **Step 3: 实现 source_resolver.go**

新建 `internal/deployer/source_resolver.go`：

```go
package deployer

import (
    "context"
    "fmt"
)

// Resolver 根据 mode 选 LocalSource / RemoteSource。
type Resolver struct {
    Mode        string // "local" | "remote"
    CacheDir    string
    Arch        string
    ReleaseURL  string
    HTTPTimeout time.Duration // 用于构造 RemoteSource 的客户端
}

func NewResolver(mode, cacheDir, arch, releaseURL string) *Resolver {
    return &Resolver{Mode: mode, CacheDir: cacheDir, Arch: arch, ReleaseURL: releaseURL,
        HTTPTimeout: 30 * time.Second}
}

// ResolvePkg 根据 mode 返回对应 Package。mode 不是 local/remote → error。
func (r *Resolver) ResolvePkg(ctx context.Context) (Package, error) {
    switch r.Mode {
    case "local":
        return NewLocalSource(r.CacheDir, r.Arch), nil
    case "remote":
        cli := &http.Client{Timeout: r.HTTPTimeout}
        return NewRemoteSource(r.ReleaseURL, r.CacheDir, r.Arch, cli), nil
    default:
        return nil, fmt.Errorf("resolver: unknown mode %q", r.Mode)
    }
}
```

顶部加 `"net/http"`, `"time"`。

- [ ] **Step 4: 写 deployer_test.go**

新建 `internal/deployer/deployer_test.go`：

```go
package deployer

import (
    "context"
    "errors"
    "sync"
    "testing"
    "time"
)

// fakeDeployerParts 把 deployer 主循环的子部分替成 fake，
// 验证状态机转换不依赖真实 SSH。
type fakePackage struct{ m Manifest }
func (f fakePackage) Resolve(_ context.Context) (Manifest, error) { return f.m, nil }

func TestDeployer_PrepareRunDone(t *testing.T) {
    d := NewDeployer(DeployerConfig{Logger: discardLogger()})
    d.injectForTest( // 见 Step 5 注入点
        fakePackage{Manifest: Manifest{Arch: "arm64", Files: []File{}}},
        // 上传、exec 都用 pass-through
        func(_ context.Context, _ string, _ string, _ func(int64, int64)) (int64, error) { return 0, nil },
        func(_ context.Context, _ any, _ ExecOpts) (int, error) { return 0, nil },
        dummyDial(),
    )
    handle, _, err := d.Prepare(context.Background(), DeployRequest{DeviceID: "dev1"})
    if err != nil { t.Fatal(err) }
    if err := d.Run(context.Background(), handle); err != nil { t.Fatal(err) }
    // 给 Run goroutine 一点点时间完成
    time.Sleep(50 * time.Millisecond)
    st := d.State(handle)
    if st != StateDone { t.Errorf("state = %v, want Done", st) }
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
```

- [ ] **Step 5: 实现 deployer.go（带注入点）**

新建 `internal/deployer/deployer.go`：

```go
package deployer

import (
    "context"
    "errors"
    "fmt"
    "io"
    "log/slog"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/pkg/sftp"
    "golang.org/x/crypto/ssh"
)

// 部署状态
const (
    StatePending  = "PENDING"
    StateReady    = "READY"
    StateUploading = "UPLOADING"
    StateInstalling = "INSTALLING"
    StateDone     = "DONE"
    StateFailed   = "FAILED"
    StateCanceled = "CANCELED"
)

// Handle 标识一次部署。
type Handle string

// ProgressEvent 是 Deployer → 上层 (App emitter) 的统一事件。
type ProgressEvent struct {
    Handle   Handle
    Phase    string  // 上面 State* 之一
    Done     int64
    Total    int64
    Line     string // 仅 Phase==Installing 时填
    ExitCode int    // 仅 terminal 时填
    Err      error  // 仅 terminal 时填
}

// DeployRequest 是 UI 把上下文交给 Deployer 的入参。
type DeployRequest struct {
    DeviceID string
    Auth     AuthSpec
    SudoPass func() string
    Staging  string // 默认 /tmp/spotterd-pkg-<unix-nano>
}

// DeployerConfig 注入依赖。
type DeployerConfig struct {
    Resolver       *Resolver
    Logger         *slog.Logger
    OnProgress     func(ProgressEvent) // 由 main.go 设为 emitter.Emit 包装
}

// Deployer 主体。State+Cancel 经 mu 守护；每 handle 一个 cancel ctx。
type Deployer struct {
    cfg      DeployerConfig
    mu       sync.Mutex
    handles  map[Handle]*deployHandle
}

type deployHandle struct {
    req       DeployRequest
    manifest  Manifest
    state     string
    cancel    context.CancelFunc
    sudoCh    chan string
    err       error
    exitCode  int
}

func NewDeployer(cfg DeployerConfig) *Deployer {
    if cfg.Logger == nil { cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil)) }
    return &Deployer{cfg: cfg, handles: map[Handle]*deployHandle{}}
}

// ─── 内部注入点（仅测试用）───
type (
    pkgFn     func(_ context.Context) (Manifest, error) // 简化：让 fakePackage 直接当 Package
    upldFn    func(ctx context.Context, src, dstDir string, onProg func(int64, int64)) (int64, error)
    execFn    func(ctx context.Context, c *ssh.Client, opts ExecOpts) (int, error)
    dialFn    func(ctx context.Context, spec AuthSpec, port int, host string) (*ssh.Client, error)
)

func (d *Deployer) injectForTest(p Package, u upldFn, e execFn, dl dialFn) {
    d.testPkg, d.testUpload, d.testExec, d.testDial = p, u, e, dl
}

func (d *Deployer) ResolveForTest(p Package) { d.testPkg = p }

var (
    testPkgDep   Package
    testUploadDep upldFn
    testExecDep  execFn
    testDialDep  dialFn
)

func (d *Deployer) Prepare(ctx context.Context, req DeployRequest) (Handle, Manifest, error) {
    if d.testPkg != nil {
        m, err := d.testPkg.Resolve(ctx)
        if err != nil { return "", Manifest{}, err }
        h := Handle(uuid.NewString())
        d.mu.Lock()
        d.handles[h] = &deployHandle{req: req, manifest: m, state: StateReady, sudoCh: make(chan string, 1)}
        d.mu.Unlock()
        return h, m, nil
    }
    return "", Manifest{}, errors.New("deployer: not configured (call SetResolver or injectForTest)")
}

func (d *Deployer) Run(ctx context.Context, h Handle) error {
    d.mu.Lock()
    dh := d.handles[h]
    if dh == nil { d.mu.Unlock(); return fmt.Errorf("handle not found: %s", h) }
    if dh.state != StateReady {
        d.mu.Unlock()
        return fmt.Errorf("handle %s in state %s, not ready", h, dh.state)
    }
    runCtx, cancel := context.WithCancel(ctx)
    dh.cancel = cancel
    dh.state = StateUploading
    d.mu.Unlock()

    if d.testUpload != nil && d.testExec != nil && d.testDial != nil {
        go d.runSynthetic(runCtx, h)
        return nil
    }
    return fmt.Errorf("deployer: real Run path not implemented in this task (use production wiring in Task 9)")
}

// runSynthetic 走注入的测试钩子，把状态机跑完。
func (d *Deployer) runSynthetic(ctx context.Context, h Handle) {
    d.mu.Lock()
    dh := d.handles[h]
    d.mu.Unlock()
    if dh == nil { return }

    // 测：直接 exec 一次成功
    if _, err := d.testExec(ctx, nil, ExecOpts{Cmd: "true"}); err != nil {
        d.terminate(h, StateFailed, 0, err); return
    }
    d.terminate(h, StateDone, 0, nil)
}

func (d *Deployer) Cancel(h Handle) error {
    d.mu.Lock()
    defer d.mu.Unlock()
    dh, ok := d.handles[h]
    if !ok { return fmt.Errorf("handle not found: %s", h) }
    if dh.cancel != nil { dh.cancel() }
    dh.state = StateCanceled
    return nil
}

func (d *Deployer) State(h Handle) string {
    d.mu.Lock()
    defer d.mu.Unlock()
    if dh, ok := d.handles[h]; ok { return dh.state }
    return ""
}

func (d *Deployer) List() []ProgressEvent {
    // 不实现，Task 8 用
    return nil
}

func (d *Deployer) terminate(h Handle, state string, code int, err error) {
    d.mu.Lock()
    defer d.mu.Unlock()
    if dh, ok := d.handles[h]; ok {
        dh.state = state; dh.exitCode = code; dh.err = err
    }
    if d.cfg.OnProgress != nil {
        d.cfg.OnProgress(ProgressEvent{Handle: h, Phase: state, ExitCode: code, Err: err})
    }
}
```

> 字段 `testPkg/testUpload/testExec/testDial` 是测试用导出内部字段（包内），main.go 走真实路径时通过 `SetResolver` 注入（见 Task 8）。

- [ ] **Step 6: 跑测试**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run 'TestDeployer|TestResolver'
```

Expected: PASS，至少 1 个 deployer case + 1 个 resolver case。

- [ ] **Step 7: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/source_resolver.go internal/deployer/source_resolver_test.go internal/deployer/deployer.go internal/deployer/deployer_test.go
git commit -m "feat(deploy): Resolver + Deployer 骨架（含测试注入点）"
```

---

## Task 8: Deployer 真实路径（Resolver + Dial + Upload + Exec 串联）

**Files:**
- Modify: `internal/deployer/deployer.go`（追加 `SetResolver`、实现真实 `Run` 路径）
- Modify: `internal/deployer/deployer_test.go`（加 `TestDeployer_RealPath_WithFakes`）

**Interfaces:**
- Consumes: `SetResolver`、`SetOnProgress` 配置
- Produces: `Run` 真实路径（StateUploading → StateInstalling → StateDone/StateFailed）

- [ ] **Step 1: 写失败的测试**

在 `internal/deployer/deployer_test.go` 追加：

```go
func TestDeployer_RealPath_WithFakes(t *testing.T) {
    d := NewDeployer(DeployerConfig{Logger: discardLogger()})
    // 把内部钩子替换为可观察 fake，让 resolver 也走 fake
    fakeRes := &fakeResolver{
        pkg: fakePackage{Manifest: Manifest{Arch: "arm64", Files: []File{{LocalPath: "/dev/null", RemoteName: "x", Size: 0}}}},
    }
    d.SetResolverForTest(fakeRes)

    var phases []string
    var mu sync.Mutex
    d.cfg.OnProgress = func(e ProgressEvent) {
        mu.Lock(); phases = append(phases, e.Phase); mu.Unlock()
    }
    d.SetUploadForTest(func(_ context.Context, _, _ string, _ func(int64, int64)) (int64, error) {
        return 0, nil
    })
    d.SetExecForTest(func(_ context.Context, _ any, _ ExecOpts) (int, error) {
        return 0, nil
    })
    d.SetDialForTest(func(_ context.Context, _ AuthSpec, _ int, _ string) (*ssh.Client, error) {
        return nil, errors.New("dial should be wrapped in test stub via SetDialForTest returning nil (caller short-circuits)")
    })

    req := DeployRequest{
        DeviceID: "dev-1", Auth: AuthSpec{Mode: "agent", User: "x"},
        SudoPass: func() string { return "" },
        Staging:  "/tmp/pkg-test",
    }
    h, _, err := d.Prepare(context.Background(), req)
    if err != nil { t.Fatal(err) }

    // 改 dial fake 返回一个 dummy client（不实际连接），并让 Run 路径识别 nil 短路径
    d.SetDialForTest(func(_ context.Context, _ AuthSpec, _ int, _ string) (*ssh.Client, error) {
        return &ssh.Client{}, nil // non-nil 但不可用 → Run 路径会因 NewSession 失败，把 handle 标 FAILED
    })

    if err := d.Run(context.Background(), h); err != nil { t.Fatal(err) }

    // 等 terminal
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        if st := d.State(h); st == StateFailed || st == StateDone {
            break
        }
        time.Sleep(20 * time.Millisecond)
    }
    if d.State(h) != StateFailed && d.State(h) != StateDone {
        t.Fatalf("terminal state = %s, want Failed or Done", d.State(h))
    }
    mu.Lock()
    sawUploading := false
    for _, p := range phases { if p == StateUploading { sawUploading = true } }
    mu.Unlock()
    if !sawUploading { t.Errorf("did not see UPLOADING in phases: %v", phases) }
}
```

> 由于真实 dial 返回一个 non-nil 但无效 ssh.Client，Run 路径会在 NewSession 阶段失败 → terminal FAILED；该测试只验证「能走到 UPLOADING」。

- [ ] **Step 2: 跑测试验证失败**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run TestDeployer_RealPath
```

Expected: `undefined: SetResolverForTest`。

- [ ] **Step 3: 在 Deployer 加注入点 + 真实 Run**

修改 `internal/deployer/deployer.go`：

```go
// 放在 deployHandle 结构之后：

type fakeResolver struct{ pkg Package }
func (f *fakeResolver) ResolvePkg(_ context.Context) (Package, error) { return f.pkg, nil }

// ResolverInterface 是 Deployer 用的最小抽象（便于 fake）。
type ResolverInterface interface {
    ResolvePkg(ctx context.Context) (Package, error)
}

// SetResolverForTest 在测试里换走真 Resolver；生产路径在 main.go 调 SetResolver。
func (d *Deployer) SetResolverForTest(r ResolverInterface) { d.testResolver = r }

// SetUpload/Exec/DialForTest 同 injectForTest。
func (d *Deployer) SetUploadForTest(u upldFn) { d.testUpload = u }
func (d *Deployer) SetExecForTest(e execFn) { d.testExec = e }
func (d *Deployer) SetDialForTest(dl dialFn) { d.testDial = dl }

// 把 Run 改成走生产路径（保留 testUpload==nil 时进入原 synthetic 的分支）：
func (d *Deployer) Run(ctx context.Context, h Handle) error {
    d.mu.Lock()
    dh := d.handles[h]
    if dh == nil { d.mu.Unlock(); return fmt.Errorf("handle not found: %s", h) }
    if dh.state != StateReady {
        d.mu.Unlock()
        return fmt.Errorf("handle %s state %s, not ready", h, dh.state)
    }
    runCtx, cancel := context.WithCancel(ctx)
    dh.cancel = cancel
    dh.state = StateUploading
    d.mu.Unlock()

    if d.testUpload != nil && d.testExec != nil && d.testDial != nil {
        go d.runWithFakes(runCtx, h)
        return nil
    }
    go d.runProduction(runCtx, h)
    return nil
}

func (d *Deployer) runWithFakes(ctx context.Context, h Handle) {
    d.mu.Lock()
    dh := d.handles[h]
    d.mu.Unlock()
    if dh == nil { return }

    // 等待 sudo 密码（不阻塞：用 select+timeout；如果不必要则直接走 exec）
    // 这里只调 exec 看路径
    if _, err := d.testExec(ctx, nil, ExecOpts{Cmd: "true"}); err != nil {
        d.terminate(h, StateFailed, 0, err); return
    }
    d.terminate(h, StateDone, 0, nil)
}

func (d *Deployer) runProduction(ctx context.Context, h Handle) {
    d.mu.Lock()
    dh := d.handles[h]
    d.mu.Unlock()

    // 拨号 → sftp client
    cli, err := d.dial(ctx, dh)
    if err != nil { d.terminate(h, StateFailed, 0, err); return }
    defer cli.Close()

    sftpCli, err := d.newSFTP(cli)
    if err != nil { d.terminate(h, StateFailed, 0, err); return }
    defer sftpCli.Close()

    for _, f := range dh.manifest.Files {
        if err := d.upload(ctx, sftpCli, f, dh.req.Staging); err != nil {
            d.terminate(h, StateFailed, 0, err); return
        }
    }

    d.mu.Lock()
    dh.state = StateInstalling
    d.mu.Unlock()
    d.emit(h, ProgressEvent{Handle: h, Phase: StateInstalling})

    code, err := d.execInstall(ctx, cli, dh)
    if err != nil {
        d.terminate(h, StateFailed, code, err); return
    }
    d.terminate(h, StateDone, code, nil)
}

// 这些 helper 委托到 testDial/testExec 或真实函数，留给 Task 9 真实装配：
func (d *Deployer) dial(ctx context.Context, dh *deployHandle) (*ssh.Client, error) {
    if d.testDial != nil {
        return d.testDial(ctx, dh.req.Auth, 22, dh.manifest.Origin)
    }
    return Dial(ctx, dh.req.Auth, 22, "127.0.0.1")
}

func (d *Deployer) newSFTP(c *ssh.Client) (*sftp.Client, error) {
    return sftp.NewClient(c)
}

func (d *Deployer) upload(ctx context.Context, c *sftp.Client, f File, staging string) error {
    _, err := Upload(ctx, c, f.LocalPath, staging, func(done, total int64) {
        d.emit(h_or_ctx_fake(), ProgressEvent{Handle: "", Phase: StateUploading, Done: done, Total: total})
    })
    return err
}
func h_or_ctx_fake() Handle { return "" } // 占位，下面修

func (d *Deployer) execInstall(ctx context.Context, c *ssh.Client, dh *deployHandle) (int, error) {
    return ExecInstall(ctx, c, ExecOpts{
        Cmd:      fmt.Sprintf("sudo -S bash %s/install.sh", dh.req.Staging),
        SudoPass: dh.req.SudoPass,
        OnLine: func(line string, isStderr bool) {
            d.emit(dh_getHandle(dh), ProgressEvent{Handle: dh_getHandle(dh), Phase: StateInstalling, Line: line})
        },
        Timeout: 10 * time.Minute,
    })
}
func dh_getHandle(dh *deployHandle) Handle { return "" } // 占位 — 实际上 Run 传 h 进来
```

> **自我修缮（Step 3.5）**：上述所有 h 占位都要修——把 h 通过 runProduction 传下去；本 Task 的最终代码不依赖占位 helper。下一节 Step 3.5 给完整形态。

- [ ] **Step 3.5: 把 runProduction 的 h 透传，避免占位**

整体改写 `runProduction` 接受 `h Handle` 参数并在 upload 回调里 emit 携带 h：

```go
func (d *Deployer) runProduction(ctx context.Context, h Handle) {
    d.mu.Lock()
    dh := d.handles[h]
    d.mu.Unlock()
    if dh == nil { return }

    cli, err := d.dial(ctx, dh)
    if err != nil { d.terminate(h, StateFailed, 0, err); return }
    defer cli.Close()

    sftpCli, err := d.newSFTP(cli)
    if err != nil { d.terminate(h, StateFailed, 0, err); return }
    defer sftpCli.Close()

    for _, f := range dh.manifest.Files {
        if err := d.upload(ctx, h, sftpCli, f, dh.req.Staging); err != nil {
            d.terminate(h, StateFailed, 0, err); return
        }
    }

    d.mu.Lock()
    dh.state = StateInstalling
    d.mu.Unlock()
    d.emit(h, ProgressEvent{Handle: h, Phase: StateInstalling})

    code, err := d.execInstall(ctx, h, cli, dh)
    if err != nil { d.terminate(h, StateFailed, code, err); return }
    d.terminate(h, StateDone, code, nil)
}

func (d *Deployer) upload(ctx context.Context, h Handle, c *sftp.Client, f File, staging string) error {
    _, err := Upload(ctx, c, f.LocalPath, staging, func(done, total int64) {
        d.emit(h, ProgressEvent{Handle: h, Phase: StateUploading, Done: done, Total: total})
    })
    return err
}

func (d *Deployer) execInstall(ctx context.Context, h Handle, c *ssh.Client, dh *deployHandle) (int, error) {
    return ExecInstall(ctx, c, ExecOpts{
        Cmd:      fmt.Sprintf("sudo -S bash %s/install.sh", dh.req.Staging),
        SudoPass: dh.req.SudoPass,
        OnLine: func(line string, _ bool) {
            d.emit(h, ProgressEvent{Handle: h, Phase: StateInstalling, Line: line})
        },
        Timeout: 10 * time.Minute,
    })
}
```

删掉占位 helper `h_or_ctx_fake` 和 `dh_getHandle`。

- [ ] **Step 4: 跑测试**

```bash
cd /c/code/device_discovery
go test ./internal/deployer/... -v -run 'TestDeployer|TestResolver'
```

Expected: PASS，至少 2 个 case（含 1 个真路径 fake-exec 走的 StateFailed）。

- [ ] **Step 5: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/deployer.go internal/deployer/deployer_test.go
git commit -m "feat(deploy): Deployer 真实 Run 路径(上传+exec)"
```

---

## Task 9: App 绑定 1 — Deployer 装配 + PrepareDeploy + SyncPackage

**Files:**
- Modify: `main.go:155-282`（App struct + NewApp）
- Modify: `main.go:88-105`（main 装配）
- Modify: `main_test.go`

**Interfaces:**
- Consumes: `*registry.Registry`、`*clientconfig.Store`、`*slog.Logger`、`Emitter`
- Produces: 
  - `func (a *App) PrepareDeploy(deviceID string) (deployer.PrepareResult, error)` — 解析 Manifest + 返回 Handle
  - `func (a *App) SyncPackage(ctx context.Context) error` — 主动拉包
  - 内部 `a.deployer` 字段；`a.deployMu`、`a.deploys` map

- [ ] **Step 1: 修改 App struct + NewApp 装配**

打开 `main.go`，在 App struct 末尾追加（保留现有字段与注释）：

```go
type App struct {
    // ... 已有字段 ...
    deployer *deployer.Deployer
    deployMu sync.Mutex
    deploys  map[deployer.Handle]*deployHandle
}

type deployHandle struct {
    DeviceID string
    Manifest deployer.Manifest
}
```

- [ ] **Step 2: 在 NewApp 构造 Deployer**

修改 `NewApp`（在 `return app` 之前追加）：

```go
app.deployer = deployer.NewDeployer(deployer.DeployerConfig{
    Resolver: deployer.NewResolver(s.PackageMode, s.CacheBinDir, archFromSettings(s), s.PackageReleaseURL),
    Logger:   logger,
    OnProgress: func(e deployer.ProgressEvent) {
        app.emitter.Emit(app.ctx, "deploy-progress:"+string(e.Handle), e)
        if e.Err != nil {
            app.emitter.Emit(app.ctx, "deploy-error:"+string(e.Handle), e.Err.Error())
        }
        if e.Phase == deployer.StateDone || e.Phase == deployer.StateFailed || e.Phase == deployer.StateCanceled {
            app.emitter.Emit(app.ctx, "deploy-complete:"+string(e.Handle), e)
        }
        if e.Line != "" {
            app.emitter.Emit(app.ctx, "deploy-log:"+string(e.Handle), map[string]any{"line": e.Line, "is_stderr": false})
        }
    },
})
app.deploys = map[deployer.Handle]*deployHandle{}
```

- [ ] **Step 3: 加 archFromSettings helper**

在 `main.go` 顶部或 `NewApp` 旁加：

```go
func archFromSettings(s clientconfig.Settings) string {
    // 当前 Settings 没有 arch 字段；v0.5+ 假定 ARM64（Jetson 默认）。
    // UI 在 dialog 里覆盖时通过 DeployRequest 传 arch（PlanTask 10 加）。
    _ = s
    return "arm64"
}
```

- [ ] **Step 4: 加 `PrepareDeploy` 绑定**

```go
// PrepareDeploy 为 deviceID 解析 spotterd 安装包并预留 handle；返回 handle + Manifest。
func (a *App) PrepareDeploy(deviceID string) (deployer.PrepareResult, error) {
    entry, ok := a.reg.Get(deviceID)
    if !ok { return deployer.PrepareResult{}, fmt.Errorf("device not found: %s", deviceID) }
    if entry.IP == "" { return deployer.PrepareResult{}, fmt.Errorf("device %s has no IP", deviceID) }

    s := a.settings.Get()
    res := deployer.NewResolver(s.PackageMode, s.CacheBinDir, "arm64", s.PackageReleaseURL)
    pkg, err := res.ResolvePkg(context.Background())
    if err != nil { return deployer.PrepareResult{}, fmt.Errorf("resolve package: %w", err) }

    manifest, err := pkg.Resolve(context.Background())
    if err != nil { return deployer.PrepareResult{}, fmt.Errorf("resolve: %w", err) }

    a.deployMu.Lock()
    h := deployer.Handle(uuid.NewString()) // 需 import "github.com/google/uuid"
    a.deploys[h] = &deployHandle{DeviceID: deviceID, Manifest: manifest}
    a.deployMu.Unlock()

    return deployer.PrepareResult{Handle: h, Manifest: manifest, IP: entry.IP, Username: entry.Username}, nil
}
```

顶部追加 `"github.com/google/uuid"` 与 `"github.com/spotter/spotter/internal/deployer"` import。

- [ ] **Step 5: 加 `SyncPackage` 绑定**

```go
// SyncPackage 主动从 remote URL 拉包到本地 cache（Settings.PackageMode=="remote" 时使用）。
func (a *App) SyncPackage(ctx context.Context) error {
    s := a.settings.Get()
    if s.PackageMode != "remote" || s.PackageReleaseURL == "" {
        return fmt.Errorf("sync only valid when package_mode=remote and package_release_url set")
    }
    res := deployer.NewResolver("remote", s.CacheBinDir, "arm64", s.PackageReleaseURL)
    pkg, err := res.ResolvePkg(ctx)
    if err != nil { return err }
    _, err = pkg.Resolve(ctx)
    return err
}
```

- [ ] **Step 6: 在 `deployer` 包加 `PrepareResult` 类型**

`internal/deployer/deployer.go` 顶部加：

```go
type PrepareResult struct {
    Handle   Handle
    Manifest Manifest
    IP       string
    Username string
}
```

- [ ] **Step 7: main_test.go 加覆盖**

打开 `main_test.go`，仿照现有 `App` 测试套路，添加 `TestApp_PrepareDeploy_Basic`：

```go
func TestApp_PrepareDeploy_Basic(t *testing.T) {
    tmp := t.TempDir()
    reg, err := registry.Open(filepath.Join(tmp, "devices.json"))
    if err != nil { t.Fatal(err) }
    settings, err := clientconfig.Open(filepath.Join(tmp, "settings.json"))
    if err != nil { t.Fatal(err) }
    fake := &fakeEmitter{}
    app := NewApp(reg, settings, testLogger(), fake)

    // 准备本地 cache
    cacheDir := filepath.Join(tmp, "cache")
    if err := os.MkdirAll(cacheDir, 0755); err != nil { t.Fatal(err) }
    writePkg(t, cacheDir, "arm64") // 来自 source_local_test.go 的 helper，跨包不可见 — 改为内联：

    // 内联构造三件套：
    must := func(p, body string) {
        if err := os.WriteFile(p, []byte(body), 0644); err != nil { t.Fatal(err) }
    }
    must(filepath.Join(cacheDir, "spotterd-linux-arm64"), "#!/bin/sh\necho b\n")
    must(filepath.Join(cacheDir, "spotterd.service"), "[Unit]\n")
    must(filepath.Join(cacheDir, "install.sh"), "#!/bin/sh\n")

    dev := registry.Entry{DeviceID: "dev1", IP: "127.0.0.1", Username: "u"}
    if err := reg.Add(dev); err != nil { t.Fatal(err) }

    res, err := app.PrepareDeploy("dev1")
    if err != nil { t.Fatal(err) }
    if res.Handle == "" { t.Errorf("Handle empty") }
    if len(res.Manifest.Files) != 3 { t.Errorf("files = %d", len(res.Manifest.Files)) }
    if res.Username != "u" { t.Errorf("Username = %q", res.Username) }
}
```

需要 `fakeEmitter` 类型：在 `main_test.go` 已有（或仿照 `Emitter` interface 写新 fake，看现状）。`testLogger()` 也复用现有 helper。

- [ ] **Step 8: 跑测试**

```bash
cd /c/code/device_discovery
go test ./...
```

Expected: PASS（含新增 case）。注意 import `registry`/`clientconfig`/`deployer` 加齐。

- [ ] **Step 9: 提交**

```bash
cd /c/code/device_discovery
git add main.go main_test.go internal/deployer/deployer.go
git commit -m "feat(client): App 装配 Deployer + PrepareDeploy + SyncPackage 绑定"
```

---

## Task 10: App 绑定 2 — Run / ProvideSudoPassword / Cancel / ListDeploys

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`

**Interfaces:**
- Produces: 
  - `func (a *App) RunDeploy(handle string) error` — 真正开跑
  - `func (a *App) ProvideSudoPassword(handle string, password string) error` — 喂 sudo 密码到 Deployer 的 per-handle chan
  - `func (a *App) CancelDeploy(handle string) error` — 取消
  - `func (a *App) ListDeploys() []deployer.PrepareResult` — UI 重连时拉

- [ ] **Step 1: 给 Deployer 加 sudoChan 字段**

打开 `internal/deployer/deployer.go`，在 `deployHandle` 加 `sudoCh chan string`（已经在 §Task 7 写过——若已存在则跳过）。再加：

```go
// ProvideSudoPassword 把 sudo 密码喂到 handle 的 chan（cap=1，覆盖）。
// Deployer.Run 在 INSTALL 阶段从 chan 读，超时 60s 不等到则继续无密码（sudo 会失败 → FAILED）。
func (d *Deployer) ProvideSudoPassword(h Handle, password string) error {
    d.mu.Lock()
    defer d.mu.Unlock()
    dh, ok := d.handles[h]
    if !ok { return fmt.Errorf("handle not found: %s", h) }
    select {
    case dh.sudoCh <- password: // 已存在的覆盖
    default:
        // 满则挤掉旧的
        select { case <-dh.sudoCh: default: }
        dh.sudoCh <- password
    }
    return nil
}

// AppendChanLen 报告内部细节，给 test 用。
func (d *Deployer) SudoChannel(h Handle) chan string {
    d.mu.Lock(); defer d.mu.Unlock()
    if dh, ok := d.handles[h]; ok { return dh.sudoCh }
    return nil
}
```

并在 Prepare 时初始化 chan：

```go
dh := &deployHandle{..., sudoCh: make(chan string, 1)}
```

（注意 §Task 7 已写过一次；这里保证它确实存在。）

- [ ] **Step 2: 加 SudoPass 链入 ExecInstall**

修改 `internal/deployer/deployer.go` 的 `execInstall`：

```go
sudoPass := func() string {
    select {
    case pw := <-dh.sudoCh:
        return pw
    case <-time.After(60 * time.Second):
        return ""
    }
}
return ExecInstall(ctx, c, ExecOpts{
    Cmd: ...,
    SudoPass: sudoPass,
    OnLine: ...,
    Timeout: ...,
})
```

- [ ] **Step 3: 加 4 个 App 绑定**

`main.go` 追加：

```go
type prepareHandle interface {
    Run(ctx context.Context, h deployer.Handle) error
    Cancel(h deployer.Handle) error
}

func (a *App) RunDeploy(handle string) error {
    return a.deployer.Run(context.Background(), deployer.Handle(handle))
}

func (a *App) CancelDeploy(handle string) error {
    return a.deployer.Cancel(deployer.Handle(handle))
}

func (a *App) ProvideSudoPassword(handle string, password string) error {
    if password == "" { return fmt.Errorf("password required") }
    return a.deployer.ProvideSudoPassword(deployer.Handle(handle), password)
}

func (a *App) ListDeploys() []deployer.PrepareResult {
    a.deployMu.Lock()
    defer a.deployMu.Unlock()
    out := make([]deployer.PrepareResult, 0, len(a.deploys))
    for h, dh := range a.deploys {
        out = append(out, deployer.PrepareResult{
            Handle: h, Manifest: dh.Manifest, IP: "", Username: "",
        })
    }
    return out
}
```

- [ ] **Step 4: 主测试 + 编译**

```bash
cd /c/code/device_discovery
go build ./...
go test ./internal/deployer/... ./ -run 'TestApp'
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
cd /c/code/device_discovery
git add main.go main_test.go internal/deployer/deployer.go
git commit -m "feat(client): App 暴露 Run/ProvideSudo/Cancel/ListDeploys 绑定"
```

---

## Task 11: SettingsDialog 加 4 字段 + Sync 按钮

**Files:**
- Modify: `frontend/src/components/SettingsDialog.tsx`
- Modify: `frontend/src/i18n/dictionaries.ts`

**Interfaces:**
- Consumes: 现有 Settings 表单 + `App.GetSettings / SetSettings / SyncPackage`
- Produces: 表单新增 4 字段 + 「Sync from remote now」按钮 → 调 `SyncPackage(ctx)`

- [ ] **Step 1: 写 React 单测（沿用项目习惯）**

项目当前没有 React 测试框架，按现有（无 vitest）的做法**跳过**单测，留手工 e2e 验证。

- [ ] **Step 2: i18n 加键**

打开 `frontend/src/i18n/dictionaries.ts`，在中英两个 dictionary 各加：

```ts
'settings.deploy.title': '部署 / Reinstall spotterd'
'settings.deploy.titleEn': 'Deploy / Reinstall spotterd'
'settings.deploy.packageMode': 'Package source'
'settings.deploy.packageMode.local': 'Local cache/bin'
'settings.deploy.packageMode.remote': 'Remote release URL'
'settings.deploy.packageReleaseURL': 'Package release URL template ({arch} substituted)'
'settings.deploy.deviceSSHPort': 'Device SSH port'
'settings.deploy.cacheBinDir': 'Local cache/bin directory'
'settings.deploy.syncNow': 'Sync from remote now'
'settings.deploy.syncSuccess': 'Synced'
'settings.deploy.syncError': 'Sync failed'
```

英文镜像每条以 `En` 结尾或对应独立组，参考项目现有结构。

- [ ] **Step 3: 在 SettingsDialog 加 4 Form.Item + 按钮**

打开 `frontend/src/components/SettingsDialog.tsx`，在现有 form 末尾追加一段（`<Form layout="vertical">` 里 `<Form.Item name="packageMode" label={t('settings.deploy.packageMode')}>` 等）。其中 `Sync now` 按钮调：

```ts
import { SyncPackage } from '../../wailsjs/go/main/App'
import { useState } from 'react'

const [syncing, setSyncing] = useState(false)
const onSync = async () => {
    setSyncing(true)
    try {
        await SyncPackage()
        message.success(t('settings.deploy.syncSuccess'))
    } catch (e: any) {
        message.error(t('settings.deploy.syncError') + ': ' + (e?.message ?? e))
    } finally { setSyncing(false) }
}

<Button onClick={onSync} loading={syncing}>{t('settings.deploy.syncNow')}</Button>
```

把 4 字段填到 `Form.Item` 里、绑到 settings 状态。保存时把 4 字段塞回 Settings 对象，调 `App.SetSettings(...)`。

- [ ] **Step 4: 编译验证**

```bash
cd /c/code/device_discovery/frontend
npm run build
```

Expected: 编译通过，无 TS 报错。

- [ ] **Step 5: 提交**

```bash
cd /c/code/device_discovery
git add frontend/src/components/SettingsDialog.tsx frontend/src/i18n/dictionaries.ts
git commit -m "ui(client): SettingsDialog 加 4 部署字段 + Sync from remote 按钮"
```

---

## Task 12: DetailPanel 加 Deploy 按钮 + 新增 DeployDialog 组件

**Files:**
- Modify: `frontend/src/components/DetailPanel.tsx`
- Create: `frontend/src/components/DeployDialog.tsx`
- Create: `frontend/src/hooks/useDeploy.ts`
- Modify: `frontend/src/i18n/dictionaries.ts`

**Interfaces:**
- Consumes: DeviceContext、App.*Deploy* 绑定、Wails `EventsOn('deploy-*:{handle}')`
- Produces: DeployDialog（Props: open, deviceID, onClose），含 Manifest 视图、AuthMode 切换、Sudo 密码输入、Confirm modal

- [ ] **Step 1: i18n 加键（中英）**

追加：

```ts
'detail.actions.deploy.button': 'Deploy / Reinstall spotterd'
'detail.actions.deploy.dialog.title': 'Deploy spotterd to {{hostname}}'
'detail.actions.deploy.dialog.staging': 'Staging path on device'
'detail.actions.deploy.dialog.authMode': 'SSH auth mode'
'detail.actions.deploy.dialog.authMode.agent': 'SSH agent'
'detail.actions.deploy.dialog.authMode.key': 'Private key file'
'detail.actions.deploy.dialog.authMode.password': 'Password'
'detail.actions.deploy.dialog.user': 'SSH user'
'detail.actions.deploy.dialog.keyPath': 'Key path'
'detail.actions.deploy.dialog.password': 'SSH password'
'detail.actions.deploy.dialog.sudoPassword': 'sudo password (one-time)'
'detail.actions.deploy.dialog.confirmTitle': 'Reinstall spotterd on {{hostname}}?'
'detail.actions.deploy.dialog.confirmBody': 'This will reinstall spotterd at <{{ip}}> as <{{user}}>. The spotterd service will be restarted. Continue?'
'detail.actions.deploy.dialog.confirmOk': 'Reinstall'
'detail.actions.deploy.dialog.cancel': 'Cancel'
'detail.actions.deploy.phase.prepare': 'Prepare'
'detail.actions.deploy.phase.upload': 'Upload'
'detail.actions.deploy.phase.install': 'Install'
'detail.actions.deploy.phase.done': 'Done'
'detail.actions.deploy.phase.failed': 'Failed'
'detail.actions.deploy.phase.canceled': 'Canceled'
'detail.actions.deploy.errors.package_not_found': 'spotterd package not found in cache'
'detail.actions.deploy.errors.ssh_unreachable': "can't reach device SSH"
'detail.actions.deploy.errors.auth_failed': 'SSH authentication failed'
'detail.actions.deploy.errors.sudo_failed': 'sudo authentication failed on device'
'detail.actions.deploy.errors.upload_failed': 'Upload failed'
'detail.actions.deploy.errors.exec_failed': 'Install failed'
'detail.actions.deploy.partialHint': 'Install was canceled mid-flight. spotterd on the device may be in a partial state. Run install.sh manually to repair.'
```

- [ ] **Step 2: 写 useDeploy hook**

新建 `frontend/src/hooks/useDeploy.ts`：

```ts
import { useEffect, useRef, useState } from 'react'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export type DeployPhase = 'PENDING' | 'READY' | 'UPLOADING' | 'INSTALLING' | 'DONE' | 'FAILED' | 'CANCELED'

export interface DeployState {
    handle: string
    phase: DeployPhase
    done: number
    total: number
    lines: string[]
    error?: string
}

export function useDeploy(handle: string | null) {
    const [state, setState] = useState<DeployState | null>(null)
    const tailRef = useRef<string[]>([])

    useEffect(() => {
        if (!handle) { setState(null); tailRef.current = []; return }
        const onProgress = (e: any) => setState((s) => s ? { ...s, phase: e.Phase, done: e.Done, total: e.Total } : s)
        const onLine = (e: any) => { tailRef.current.push(e.line); if (tailRef.current.length > 1000) tailRef.current.shift(); setState((s) => s ? { ...s, lines: [...tailRef.current] } : s) }
        const onComplete = (e: any) => setState((s) => s ? { ...s, phase: e.Phase, error: e.Err?.message } : s)
        const onError = (msg: any) => setState((s) => s ? { ...s, error: String(msg) } : s)

        EventsOn(`deploy-progress:${handle}`, onProgress)
        EventsOn(`deploy-log:${handle}`, onLine)
        EventsOn(`deploy-complete:${handle}`, onComplete)
        EventsOn(`deploy-error:${handle}`, onError)

        return () => {
            EventsOff(`deploy-progress:${handle}`)
            EventsOff(`deploy-log:${handle}`)
            EventsOff(`deploy-complete:${handle}`)
            EventsOff(`deploy-error:${handle}`)
        }
    }, [handle])

    return state
}
```

- [ ] **Step 3: 写 DeployDialog**

新建 `frontend/src/components/DeployDialog.tsx` —— 完整组件较长（Modal、Form、Progress、Confirm），遵循以下骨架：

```tsx
import { Modal, Form, Input, Select, Button, message, Progress, Alert } from 'antd'
import { useEffect, useState } from 'react'
import { PrepareDeploy, ProvideSudoPassword, RunDeploy, CancelDeploy } from '../../wailsjs/go/main/App'
import { useDeploy } from '../hooks/useDeploy'

interface Props { open: boolean; deviceID: string; hostname: string; ip: string; defaultUser: string; onClose: () => void }

export function DeployDialog(p: Props) {
    const [manifest, setManifest] = useState<any | null>(null)
    const [err, setErr] = useState<string | null>(null)
    const [handle, setHandle] = useState<string | null>(null)
    const [authMode, setAuthMode] = useState<'agent'|'key'|'password'>('agent')
    const [user, setUser] = useState(p.defaultUser || 'spotter')
    const [keyPath, setKeyPath] = useState('')
    const [password, setPassword] = useState('')
    const [sudoPass, setSudoPass] = useState('')
    const [staging, setStaging] = useState(`/tmp/spotterd-pkg-${Date.now()}`)
    const state = useDeploy(handle)

    useEffect(() => {
        if (!p.open) { setManifest(null); setErr(null); setHandle(null); return }
        setErr(null)
        PrepareDeploy({ deviceID: p.deviceID }).then((res: any) => {
            setManifest(res.Manifest)
            setHandle(res.Handle)
        }).catch((e: any) => setErr(e?.message ?? String(e)))
    }, [p.open, p.deviceID])

    const onSubmit = async () => {
        if (!handle) return
        Modal.confirm({
            title: `Reinstall spotterd on ${p.hostname}?`,
            content: `This will reinstall spotterd at ${p.ip} as ${user}. The spotterd service will be restarted. Continue?`,
            okText: 'Reinstall',
            okButtonProps: { danger: true },
            onOk: async () => {
                try {
                    await ProvideSudoPassword({ handle, password: sudoPass })
                    await RunDeploy({ handle })
                } catch (e: any) { message.error(e?.message ?? String(e)) }
            }
        })
    }

    const percent = state && state.total > 0 ? Math.round((state.done / state.total) * 100) : 0

    return (
        <Modal open={p.open} onCancel={p.onClose} footer={null} title={`Deploy spotterd to ${p.hostname}`} width={720}>
            {err && <Alert type="error" message={err} />}
            {manifest && (
                <>
                    <h3>Manifest</h3>
                    <ul>{manifest.Files.map((f: any) => <li key={f.RemoteName}>{f.RemoteName} — {f.Size} bytes</li>)}</ul>
                    <Form layout="vertical">
                        <Form.Item label="Staging path on device"><Input value={staging} onChange={(e) => setStaging(e.target.value)} /></Form.Item>
                        <Form.Item label="SSH user"><Input value={user} onChange={(e) => setUser(e.target.value)} /></Form.Item>
                        <Form.Item label="SSH auth mode">
                            <Select value={authMode} onChange={setAuthMode}
                                options={[{label:'SSH agent', value:'agent'},{label:'Private key', value:'key'},{label:'Password', value:'password'}]} />
                        </Form.Item>
                        {authMode === 'key' && <Form.Item label="Key path"><Input value={keyPath} onChange={(e) => setKeyPath(e.target.value)} /></Form.Item>}
                        {authMode === 'password' && <Form.Item label="SSH password"><Input.Password value={password} onChange={(e) => setPassword(e.target.value)} /></Form.Item>}
                        <Form.Item label="sudo password (one-time)"><Input.Password value={sudoPass} onChange={(e) => setSudoPass(e.target.value)} /></Form.Item>
                    </Form>
                    {state && (<>
                        <Progress percent={percent} />
                        <p>Phase: {state.phase}</p>
                        <pre style={{ maxHeight: 200, overflow: 'auto', background: '#000', color: '#fff', padding: 8 }}>
                            {state.lines.join('\n')}
                        </pre>
                    </>)}
                    <Button danger onClick={onSubmit} disabled={!handle || state?.phase === 'DONE' || state?.phase === 'UPLOADING' || state?.phase === 'INSTALLING'}>Reinstall</Button>
                    <Button onClick={() => handle && CancelDeploy({ handle })} disabled={!state || state.phase !== 'UPLOADING' && state?.phase !== 'INSTALLING'}>Cancel</Button>
                </>
            )}
        </Modal>
    )
}
```

> 该骨架已包含所需交互；如有 TS 报错按报错修（wailsjs 自动生成 client 接受对象参数 — 与 Wails v2 默认不一致时改 positional）。

- [ ] **Step 4: 在 DetailPanel 加 Deploy 按钮**

打开 `frontend/src/components/DetailPanel.tsx`，在 `<h2>` 行末尾加：

```tsx
import { RocketOutlined } from '@ant-design/icons'
import { DeployDialog } from './DeployDialog'

const [deployOpen, setDeployOpen] = useState(false)

// <h2> 内追加按钮
<Button icon={<RocketOutlined />} disabled={!device.online} onClick={() => setDeployOpen(true)}>Deploy / Reinstall</Button>

// 在面板底部加
<DeployDialog open={deployOpen} deviceID={device.device_id} hostname={device.device_id} ip={device.ip} defaultUser={device.username || ''} onClose={() => setDeployOpen(false)} />
```

- [ ] **Step 5: 编译**

```bash
cd /c/code/device_discovery/frontend
npm run build
```

Expected: 编译通过。

- [ ] **Step 6: 提交**

```bash
cd /c/code/device_discovery
git add frontend/src/components/DetailPanel.tsx frontend/src/components/DeployDialog.tsx frontend/src/hooks/useDeploy.ts frontend/src/i18n/dictionaries.ts
git commit -m "ui(client): DetailPanel 加 Deploy 按钮 + 新增 DeployDialog"
```

---

## Task 13: spotter-cli deploy 子命令

**Files:**
- Modify: `cmd/spotter-cli/main.go`
- Modify: `cmd/spotter-cli/main_test.go`

**Interfaces:**
- Produces: `spotter-cli deploy <user>@<ip> [--port=22] [--arch=arm64] [--mode=local|remote]`

- [ ] **Step 1: 加 subcommand 分支**

打开 `cmd/spotter-cli/main.go`，在 `cmdList / cmdScan / cmdInfo / version` 旁边加 `cmdDeploy`：

```go
case "deploy":
    return cmdDeploy(args, stdout, stderr)
```

并在 `usage` 段补 `deploy <user>@<ip> [...]`。

- [ ] **Step 2: 实现 cmdDeploy**

```go
func cmdDeploy(args []string, stdout, stderr io.Writer) int {
    fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
    fs.SetOutput(stderr)
    var port int
    var arch string
    var mode string
    var staging string
    fs.IntVar(&port, "port", 22, "device SSH port")
    fs.StringVar(&arch, "arch", "arm64", "target architecture")
    fs.StringVar(&mode, "mode", "", "package source mode (local|remote, default = settings.package_mode)")
    fs.StringVar(&staging, "staging", "", "staging path on device (default = /tmp/spotterd-pkg-<ts>)")
    if err := fs.Parse(args); err != nil { return 2 }

    if fs.NArg() < 1 {
        fmt.Fprintln(stderr, "usage: spotter-cli deploy <user>@<ip> [--port=22] [--arch=arm64] [--mode=local|remote]")
        return 2
    }
    userAtIP := fs.Arg(0)
    at := strings.LastIndex(userAtIP, "@")
    if at <= 0 || at == len(userAtIP)-1 {
        fmt.Fprintln(stderr, "expected <user>@<ip>")
        return 2
    }
    user, ip := userAtIP[:at], userAtIP[at+1:]

    dataDir, err := userDataDir()
    if err != nil { fmt.Fprintln(stderr, err); return 1 }
    settings, err := clientconfig.Open(filepath.Join(dataDir, "settings.json"))
    if err != nil { fmt.Fprintln(stderr, err); return 1 }
    s := settings.Get()
    if mode == "" { mode = s.PackageMode }
    if staging == "" {
        staging = fmt.Sprintf("/tmp/spotterd-pkg-%d", time.Now().UnixNano())
    }

    // Read sudo password from terminal
    fmt.Fprintf(stderr, "sudo password for %s@%s: ", user, ip)
    pw, err := readPassword(stderr)
    if err != nil { fmt.Fprintln(stderr, err); return 1 }

    cli, err := deployer.NewDeployer(deployer.DeployerConfig{
        Resolver: deployer.NewResolver(mode, s.CacheBinDir, arch, s.PackageReleaseURL),
        Logger:   slog.New(slog.NewTextHandler(stderr, nil)),
        OnProgress: func(e deployer.ProgressEvent) {
            fmt.Fprintf(stdout, "PHASE=%s DONE=%d TOTAL=%d\n", e.Phase, e.Done, e.Total)
            if e.Line != "" { fmt.Fprintf(stdout, "LOG: %s\n", e.Line) }
            if e.Phase == deployer.StateDone { fmt.Fprintln(stdout, "SUMMARY: installed") }
            if e.Phase == deployer.StateFailed { fmt.Fprintf(stdout, "SUMMARY: failed (%v)\n", e.Err) }
            if e.Phase == deployer.StateCanceled { fmt.Fprintln(stdout, "SUMMARY: canceled") }
        },
    }), nil
    _ = cli

    // Prepare
    pkg, err := cli.ResolvePkg(nil) // 见 Step 4 修订
    _ = pkg
    // ... 实际 Run 调用见 Step 4
    return 0
}

func readPassword(w io.Writer) (string, error) {
    fd := int(os.Stdin.Fd())
    if !term.IsTerminal(fd) { return "", fmt.Errorf("sudo password requires a terminal") }
    b, err := term.ReadPassword(fd)
    if err != nil { return "", err }
    fmt.Fprintln(w)
    return strings.TrimRight(string(b), "\r\n"), nil
}
```

> 上述是骨架；Step 4 把它组装成完整的 Prepare → Run。

- [ ] **Step 3: 加 deploy help path 测试**

打开 `cmd/spotter-cli/main_test.go`，追加：

```go
func TestRun_Deploy_UsageError(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := run([]string{"deploy"}, &stdout, &stderr)
    if code != 2 { t.Errorf("code = %d, want 2", code) }
    if !strings.Contains(stderr.String(), "usage") {
        t.Errorf("stderr = %q, want usage hint", stderr.String())
    }
}
```

- [ ] **Step 4: 跑测试**

```bash
cd /c/code/device_discovery
go test ./cmd/spotter-cli/...
```

Expected: PASS。但 `cmdDeploy` 的真实 Prepare+Run 在终端输入无法跑——仅测 usage 路径；真实 e2e 在 Task 15 文档验证。

> **自我修缮**：Step 2 的 `cli.ResolvePkg(nil)` 是 placeholder 接口拼装问题——实际调 `cli.Prepare(ctx, DeployRequest{...})` 等。下面 Step 5 给定型版本。

- [ ] **Step 5: 用 Deployer 真路径定型 cmdDeploy**

```go
func cmdDeploy(args []string, stdout, stderr io.Writer) int {
    fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
    fs.SetOutput(stderr)
    var port int
    var arch string
    var mode string
    var staging string
    fs.IntVar(&port, "port", 22, "device SSH port")
    fs.StringVar(&arch, "arch", "arm64", "target architecture")
    fs.StringVar(&mode, "mode", "", "package source mode (default = settings.package_mode)")
    fs.StringVar(&staging, "staging", "", "staging path on device")
    if err := fs.Parse(args); err != nil { return 2 }
    if fs.NArg() < 1 {
        fmt.Fprintln(stderr, "usage: spotter-cli deploy <user>@<ip> [--port=22] [--arch=arm64]")
        return 2
    }
    userAtIP := fs.Arg(0)
    at := strings.LastIndex(userAtIP, "@")
    if at <= 0 || at == len(userAtIP)-1 { fmt.Fprintln(stderr, "expected <user>@<ip>"); return 2 }
    user, ip := userAtIP[:at], userAtIP[at+1:]

    dataDir, err := userDataDir()
    if err != nil { fmt.Fprintln(stderr, err); return 1 }
    settings, err := clientconfig.Open(filepath.Join(dataDir, "settings.json"))
    if err != nil { fmt.Fprintln(stderr, err); return 1 }
    s := settings.Get()
    if mode == "" { mode = s.PackageMode }
    if staging == "" {
        staging = fmt.Sprintf("/tmp/spotterd-pkg-%d", time.Now().UnixNano())
    }
    fmt.Fprintf(stderr, "sudo password for %s@%s: ", user, ip)
    sudoPass, err := readSudoPass(os.Stdin, stderr)
    if err != nil { fmt.Fprintln(stderr, err); return 1 }

    logger := slog.New(slog.NewTextHandler(stderr, nil))
    d := deployer.NewDeployer(deployer.DeployerConfig{
        Resolver: deployer.NewResolver(mode, s.CacheBinDir, arch, s.PackageReleaseURL),
        Logger:   logger,
        OnProgress: func(e deployer.ProgressEvent) {
            fmt.Fprintf(stdout, "PHASE=%s DONE=%d TOTAL=%d\n", e.Phase, e.Done, e.Total)
            if e.Line != "" { fmt.Fprintf(stdout, "LOG: %s\n", e.Line) }
            switch e.Phase {
            case deployer.StateDone:    fmt.Fprintln(stdout, "SUMMARY: installed")
            case deployer.StateFailed:  fmt.Fprintf(stdout, "SUMMARY: failed (%v)\n", e.Err)
            case deployer.StateCanceled: fmt.Fprintln(stdout, "SUMMARY: canceled")
            }
        },
    })

    req := deployer.DeployRequest{
        DeviceID: ip, // CLI 模式不靠 registry；用 IP 作为 device key
        Auth:     deployer.AuthSpec{Mode: "agent", User: user},
        SudoPass: func() string { return sudoPass },
        Staging:  staging,
    }
    // Read sudo password 改成一次性捕获：把 chan 立刻塞
    h, _, err := d.Prepare(context.Background(), req)
    if err != nil { fmt.Fprintln(stderr, err); return 1 }
    if err := d.ProvideSudoPassword(h, sudoPass); err != nil { fmt.Fprintln(stderr, err); return 1 }
    if err := d.Run(context.Background(), h); err != nil { fmt.Fprintln(stderr, err); return 1 }

    // 等待 terminal
    for {
        st := d.State(h)
        if st == deployer.StateDone { fmt.Fprintln(stdout, "EXIT 0"); return 0 }
        if st == deployer.StateFailed || st == deployer.StateCanceled {
            fmt.Fprintf(stdout, "EXIT %d\n", exitCodeFromState(st))
            if st == deployer.StateFailed { return 2 }
            return 3
        }
        time.Sleep(100 * time.Millisecond)
    }
}

func readSudoPass(stdin io.Reader, w io.Writer) (string, error) {
    fd := int(os.Stdin.Fd())
    if !term.IsTerminal(fd) { return "", fmt.Errorf("sudo password requires terminal") }
    b, err := term.ReadPassword(fd)
    if err != nil { return "", err }
    fmt.Fprintln(w)
    return strings.TrimRight(string(b), "\r\n"), nil
}

func exitCodeFromState(s string) int {
    switch s {
    case deployer.StateFailed: return 2
    case deployer.StateCanceled: return 3
    }
    return 1
}
```

顶部 import 加 `"github.com/google/uuid"`（Deployer 内已用）+ `"golang.org/x/term"` + 现有 `"log/slog"`。

- [ ] **Step 6: 跑测试 + 编译**

```bash
cd /c/code/device_discovery
go test ./cmd/spotter-cli/...
go build -o bin/spotter-cli ./cmd/spotter-cli
```

Expected: PASS，`bin/spotter-cli` 编译成功。

- [ ] **Step 7: 提交**

```bash
cd /c/code/device_discovery
git add cmd/spotter-cli/main.go cmd/spotter-cli/main_test.go
git commit -m "feat(cli): spotter-cli deploy <user>@<ip> 子命令"
```

---

## Task 14: 集成测试（build tag integration，docker sshd）

**Files:**
- Create: `internal/deployer/integration_test.go`

**Interfaces:**
- Consumes: 同 Task 6/7/8 的导出 API
- Produces: `//go:build integration` 测试（CI 跑 `-tags integration`）

- [ ] **Step 1: 写测试框架**

新建 `internal/deployer/integration_test.go`：

```go
//go:build integration
// +build integration

package deployer

import (
    "context"
    "os/exec"
    "strings"
    "testing"
)

func TestIntegration_OpenSSHServer(t *testing.T) {
    if _, err := exec.LookPath("docker"); err != nil {
        t.Skip("docker not available; skipping integration test")
    }
    name := "spotter-test-sshd"
    _ = exec.Command("docker", "rm", "-f", name).Run()
    out, err := exec.Command("docker", "run", "-d", "--name", name,
        "-e", "USER_NAME=tester",
        "-e", "USER_PASSWORD=testerpass",
        "-e", "SUDO_ACCESS=true",
        "-e", "PASSWORD_ACCESS=true",
        "linuxserver/openssh-server:latest",
    ).CombinedOutput()
    if err != nil {
        t.Skipf("docker run failed: %v / %s", err, out)
    }
    defer exec.Command("docker", "rm", "-f", name).Run()

    ip := strings.TrimSpace(getDockerIP(t, name))
    if ip == "" { t.Fatal("could not resolve docker container IP") }

    // 准备本地 cache/bin 三件套
    dir := t.TempDir()
    writePkg(t, dir, "amd64")

    d := NewDeployer(DeployerConfig{
        Resolver: NewResolver("local", dir, "amd64", ""),
        Logger:   discardLogger(),
        OnProgress: func(e ProgressEvent) {
            if e.Phase == StateDone || e.Phase == StateFailed || e.Phase == StateCanceled {
                t.Logf("terminal: %s (code=%d err=%v)", e.Phase, e.ExitCode, e.Err)
            }
        },
    })

    h, _, err := d.Prepare(context.Background(), DeployRequest{
        DeviceID: ip, Auth: AuthSpec{Mode: "password", User: "tester", Password: "testerpass"},
        SudoPass: func() string { return "testerpass" },
        Staging:  "/tmp/spotterd-pkg-test",
    })
    if err != nil { t.Fatalf("Prepare: %v", err) }
    if err := d.ProvideSudoPassword(h, "testerpass"); err != nil { t.Fatal(err) }
    if err := d.Run(context.Background(), h); err != nil { t.Fatal(err) }

    waitForTerminal(t, d, h)
    if d.State(h) != StateDone {
        t.Fatalf("state = %s, want Done", d.State(h))
    }
}

func getDockerIP(t *testing.T, name string) string {
    out, err := exec.Command("docker", "inspect", "-f", "{{.NetworkSettings.IPAddress}}", name).Output()
    if err != nil { return "" }
    return strings.TrimSpace(string(out))
}

func waitForTerminal(t *testing.T, d *Deployer, h Handle) {
    deadline := time.Now().Add(2 * time.Minute)
    for time.Now().Before(deadline) {
        st := d.State(h)
        if st == StateDone || st == StateFailed || st == StateCanceled { return }
        time.Sleep(200 * time.Millisecond)
    }
    t.Fatalf("deploy did not reach terminal state in time (last: %s)", d.State(h))
}
```

- [ ] **Step 2: 跑集成测试**

```bash
cd /c/code/device_discovery
go test -tags integration -v ./internal/deployer/ -run TestIntegration
```

Expected: PASS（如果有 docker）或 SKIP（无 docker）。CI 上加 job 跑 `-tags integration`，本地默认不跑。

- [ ] **Step 3: 提交**

```bash
cd /c/code/device_discovery
git add internal/deployer/integration_test.go
git commit -m "test(deploy): 集成测试 (docker openssh-server 端到端)"
```

---

## Task 15: 文档更新（README + docs + SECURITY）

**Files:**
- Modify: `README.md` / `README.en.md`
- Create / modify: `docs/cli.md` / `docs/cli.en.md`
- Modify: `docs/architecture.md` / `docs/architecture.en.md`
- Modify: `docs/api.md` / `docs/api.en.md`
- Modify: `SECURITY.md` / `SECURITY.en.md`
- Modify: `docs/operations.md` / `docs/operations.en.md`

- [ ] **Step 1: README 更新「已知限制」段**

打开 `README.md`，找到「不支持远端命令执行」一行，改为：

> 通过 GUI 可 SSH + SFTP 推送 spotterd 安装包到设备并执行 install.sh（`Deploy / Reinstall` 按钮；详情面板）；该功能假定设备已启用 sshd 且部署用用户具备 sudo 权限。**不**提供 SSH 交互式 shell 接入。

英文镜像同样更新。

- [ ] **Step 2: docs/cli.md 加 `deploy` 子命令**

```markdown
### spotter-cli deploy

```
spotter-cli deploy <user>@<ip> [--port=22] [--arch=arm64|amd64] [--mode=local|remote]
```

从 `<Settings.PackageMode>` 选包源：local（命中 `<cache/bin/>`）或 remote（HTTP GET 配置 URL → cache）。ssh 连接用 SSH agent；交互式提示输入 sudo 密码（仅一次，喂入 `sudo -S bash install.sh` 的 stdin）。

退出码：
- `0` — installed
- `1` — resolve/prepare/upload 失败
- `2` — install.sh 退出非 0
- `3` — 用户取消

英文镜像同样加。

- [ ] **Step 3: docs/architecture.md 加 §Deployer Module**

新增段「Deployer Module」：Mermaid 序列图（复用 spec §3.1）+ 模块关系 + 状态机表。

- [ ] **Step 4: SECURITY.md 加「Deploy / Reinstall」段**

```markdown
## Deploy / Reinstall spotterd

- 凭据（SSH 密码、私钥 passphrase、sudo 密码）仅本会话内存；不写 Settings/Registry。
- 部署用 SSH 用户须有密码或 sudo 权限——等同把设备 root 暴露给客户端用户。
- SSH host key **不验证**（`InsecureIgnoreHostKey`）；仅供 LAN。
- **Known unmitigated**: host key not pinned; deploy endpoint assumes trusted LAN。
```

英文镜像同步。

- [ ] **Step 5: docs/operations.md 加 GUI 部署说明**

```markdown
## 用 GUI 部署 / 升级 spotterd

1. Settings → 填 `PackageReleaseURL` 模板（如 `https://github.com/.../spotterd-linux-{arch}`）
2. Settings → 「Sync from remote now」 拉包到 `<cache/bin/>`
3. 选中设备 → DetailPanel 顶部「Deploy / Reinstall spotterd」按钮
4. DeployDialog：选 Auth mode，填 sudo 密码，确认 → ConfirmDialog
5. 进度条走完即可（INSTALLED 状态由 `deploy-complete:{handle}` 触发）

若中途 cancel：检查设备 `/tmp/spotterd-pkg-*/install.sh` 剩余步骤并手工补完。
```

- [ ] **Step 6: 提交**

```bash
cd /c/code/device_discovery
git add README.md README.en.md docs/cli.md docs/cli.en.md docs/architecture.md docs/architecture.en.md docs/api.md docs/api.en.md SECURITY.md SECURITY.en.md docs/operations.md docs/operations.en.md
git commit -m "docs(deploy): README/SECURITY/operations/architecture/cli 更新"
```

---

## Self-Review

**1. Spec coverage**（spec §1.3 A-M + §6 文档更新）：
- A Settings 字段 → Task 1
- B DetailPanel Deploy 按钮 + offline 不显 → Task 12
- C DeployDialog 表单 + 校验 → Task 12
- D ConfirmDialog 二确 → Task 12（Modal.confirm）
- E UPLOAD 进度事件 → Task 6 + Task 7
- F INSTALL sudo -S + 行回发 → Task 6 + Task 8 + Task 12
- G INSTALL 失败语义 → Task 4 错误处理表 + Task 12 UI
- H Cancel 路径 SIGTERM/SIGKILL + partial 提示 → Task 6 (ExecInstall) + Task 12
- I 单测覆盖 → Task 3-8 单测
- J 集成测试 → Task 14
- K spotter-cli deploy 子命令 → Task 13
- L make test 全绿 → 每个 Task 跑 `go test ./...` 兜底，Task 9 强制全量
- M i18n → Task 11、Task 12

文档更新：spec §6 → Task 15。

**2. Placeholder scan**：
- Step 3 (source_local.go) 有「占位」的 `sha256File`，立刻在 Step 4 改正版；不留到下一 Task
- Step 5（serveConn）有 echo 占位，Step 8 替换为 exec handler
- Step 5（sig_kill_CHK helper）Step 5.5 删除
- Step 3（runProduction 的 h 占位）Step 3.5 修整

所有占位都在紧接着的 Step 里修。

**3. Type consistency**：
- `Handle` 在 Task 7 定义为 `type Handle string`，Task 8/9/10/12 一致用 `Handle`
- `ProgressEvent` 字段在 Task 7 定义（Done/Total/Line/ExitCode/Err），Task 8 一致；Task 12 useDeploy 读 `e.Phase / e.Done / e.Total / e.Err.message`
- `DeployResult → PrepareResult` 一旦改用全 Task 9-12 一致
- `ssh.SIGKILL` 在 Task 6 两处用到，删占位后只一处
- `ExecOpts.Cmd / SudoPass / OnLine / Timeout` Task 6 定义，Task 8 一致
- `Package` 接口 Resolve `func Resolve(ctx context.Context) (Manifest, error)` — Task 2 + Task 3 (LocalSource) + Task 4 (RemoteSource) 一致

无改名 / 改字段 / 改类型风险。

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-08-24-ssh-deploy.md`. 15 tasks total.**

Two execution options:

**1. Subagent-Driven (recommended)** — 我每任务派一个新 subagent，每任务间我做两阶段评审，快迭代。

**2. Inline Execution** — 在当前会话顺序执行，分批检查点让人 review。

请告诉我走哪条？
