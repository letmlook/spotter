# Spotter 通过 SSH/SFTP 上传并安装 / 更新设备端软件

| 项 | 值 |
|---|---|
| 项目名 | **Spotter** |
| 文档日期 | 2026-08-24 |
| 状态 | 设计草案，待用户审核 |
| 范围 | `spotter-client`（Wails GUI）增加「通过 SSH + SFTP 推送 spotterd 安装包并在目标机上执行 `install.sh`」的功能 |
| 改动文件 | `internal/deployer/`（新包）, `main.go`, `internal/registry/registry.go`, `internal/clientconfig/store.go`, `internal/scanner/scanner.go`, `frontend/src/...`, `go.mod`, `go.sum` |
| 文档更新 | `README.md`, `docs/cli.md`, `docs/architecture.md`, `docs/api.md`, `SECURITY.md`, `docs/operations.md` |

---

## 1. 目标与范围

### 1.1 目标

让客户端用户能通过 GUI **「Deploy / Reinstall spotterd」** 按钮把对应架构的安装包**（`spotterd-linux-<arch>`、`spotterd.service`、`install.sh`）** 通过 SFTP 推送到设备上 `/tmp/spotterd-pkg-<ts>/`，然后通过 SSH 在设备上跑 `sudo bash <staging>/install.sh` 完成安装或升级。

- 在选中设备的 `DetailPanel` 顶部加一个主按钮「Deploy / Reinstall spotterd」。
- 点击后弹出 `DeployDialog`：选择包源（本地 `bin/` 镜像 / 远程 release URL）、填 SSH 凭据 + 临时 sudo 密码、看 Manifest 文件清单与目标 staging 路径。
- 走「二次确认」：DeployDialog 关闭后弹 `ConfirmDialog`，再次列明「会重启 spotterd 服务」等不可逆动作，确认才走。
- 上传 + 安装阶段实时出进度（百分比 + 当前阶段 UPLOAD / INSTALL）。
- 同时为 `spotter-cli` 添加 `spotter-cli deploy <user>@<ip>` 子命令，复用同一 `internal/deployer` 包以便服务器 / headless 场景。

### 1.2 非目标（明确排除）

- 在设备端启动一个 SSH 服务（spotterd 仍只走 HTTP）。本功能假定设备已有 sshd 在标准 22 端口监听，且部署用的 SSH 用户已存在于该设备上。
- 不做 versioned / 多版本管理（release 选择器、OTA-like 概念）。客户端只推「同一份」包，由 release 流程做版本切换。
- 不在客户端嵌入 spotterd 二进制（`//go:embed`）。本设计采用「本地 cache/bin/ 镜像 + 可选远程 release URL」两步式来源，避免客户端体积膨胀。
- 不实现 SSH host key 验证 UI 引导（首次连接弹「Trust this host?」由 `known_hosts` 走 OpenSSH 标准行为；用 `~/.ssh/known_hosts` 不存在时静默接受首次。
- 不引入 rsync / scp 协议。SFTP 推三个文件够用。
- 不做密钥托管 / OS keychain / 凭据持久化。SSH 凭据 + sudo 密码都是本次会话内一次性（详见 §1.4）。

### 1.3 验收标准

| # | 标准 |
|---|------|
| A | Settings 增加 `PackageMode ∈ {"local","remote"}`、`PackageReleaseURL`、`DeviceSSHPort`（默认 22）、`CacheBinDir`（默认 `<dataDir>/cache/bin/`），UI 设置页可改并落盘 |
| B | 客户端选中一个 online 设备，DetailPanel 出现「Deploy / Reinstall」按钮。设备 offline 或未注册时按钮不显示 |
| C | DeployDialog 显示 Manifest 文件清单（3 个文件名 + 字节数 + 哈希）、staging 路径、SSH 用户名（默认 `Registry.Entry.Username`）、认证方式（agent / key / password）、sudo 密码输入框；任何无效输入本地校验不通过则禁用提交 |
| D | 走「ConfirmDialog」二次确认后才进入上传阶段，文案含设备 hostname + ip + 部署用户名 + 警告「this will restart the spotterd service」 |
| E | UPLOAD 阶段：SFTP 推送三文件，UI 进度条按累计字节走 0-100%。每 200ms 或每 1MB 发一次 `deploy-progress:{handle}` |
| F | INSTALL 阶段：SSH session 跑 `sudo -S bash <staging>/install.sh`，sudo 密码通过 stdin 一次性喂入；install.sh 的 stdout / stderr 逐行回发 `deploy-log:{handle}`；命令退出 → 发 `deploy-complete:{handle}` payload 含 phase + exit code |
| G | 安装失败（exit code ≠ 0）→ 弹错误条；继续发 `deploy-complete` 终止 handle，并把 staging 路径留给用户清理 |
| H | UPLOAD/INSTALL 任意阶段用户点取消 → ctx cancel；INSTALL 阶段取消分 SIGTERM→5s→SIGKILL；完成后发 `deploy-canceled` 与「device may be in a partial state」软提示 |
| I | `internal/deployer` 包有完整单测覆盖 Package resolver、3 种 auth mode、SFTP loopback、SSH exec loopback（不依赖外网） |
| J | `internal/deployer` 集成测试（build tag `integration`）：docker 容器 `linuxserver/openssh-server` 内 user 有 passwordless sudo，端到端跑通 Prepare→Upload→Install，exit 0 |
| K | `spotter-cli deploy <user>@<ip>` 子命令复用 `internal/deployer`，交互式提示输入 sudo 密码（不读取 stdin 全量），上传 + 安装完打印 SUMMARY |
| L | 已有的 `make test` 全绿，未破坏 `internal/agentd`, `internal/scanner`, `internal/registry`, `main.go` 的现有测试 |
| M | UI 增加中英 i18n 条目；现有所有 antd i18n 流程不受影响 |

### 1.4 凭据处理规则（明确）

- **SSH 密码 / 私钥 passphrase / sudo 密码 全部仅在内存**。任何字段不留盘、不进 `Settings.json`、不进 `Registry.json`、不进日志。
- SSH 私钥**路径**仅在用户选择「Private key file」模式时被记录在 DeployDialog 的本地 React state，关掉对话框即丢；不持久。
- `Registry.Entry.Username` 已是开源字段，作为 SSH 用户名默认值复用，不引入新密码字段。

---

## 2. 现状与障碍

### 2.1 现状

**部署外置流程 `scripts/deploy.sh`：**
- 已经实现了完整的 SSH 上传 + install 流程（22 端口、`sshpass` 走密码 / 默认 key、上传 `spotterd` + `spotterd.service` + `install.sh`、ssh 跑 `sudo bash /tmp/spotterd-pkg-<ts>/install.sh`）。
- 与项目主客户端**完全独立**：用户需要单独调脚本，不在 GUI 范围内。
- 假定部署用 SSH 用户有 passwordless sudo（这是必须的人手配线）。

**`spotterd`（agent）端：**
- 仅暴露 HTTP 端点 `/healthz`, `/api/v1/info`, `/api/v1/reboot`, `/api/v1/shutdown`, `/api/v1/logs`。
- 不暴露 SSH / SFTP。spotterd 也不应是 SSH server——设备本身 sshd 是 OS 自带。

**`spotter-client`（Wails GUI）：**
- `main.go` 已绑定的方法：`GetSettings / SetSettings / ScanSubnet / ProbeByIP / AcceptUnknownDevice / RebootDevice / ShutdownDevice / StartLogStream / StopLogStream / ClearRegistry / RefreshNow / LocalSubnets`。
- 现有 Wails 事件流模式：`OnEvent` → `wailsEmitter.Emit(ctx, tag, payload)`。日志流已有 `device-log:{deviceID}` 范式可借鉴。
- 前端组件目录见 `frontend/src/components/`，现有 DetailPanel 顶部无部署按钮。

**`spotter-cli`：**
- 子命令：`list / scan / info / version`。无 `deploy`。

**`go.mod`：**
- 不含 `golang.org/x/crypto/ssh` 或 `pkg/sftp`，本次首次引入。

### 2.2 与 MVP 限制的关系

README 当前文案：
- 「不支持远端命令执行」 → 本次引入的是 SSH 触发的 install（不是 ssh shell 直通车），需更新文案。
- 「HTTP 端点无身份认证」 → 保留；本次新增的 SSH 认证是**用 OS sshd** 的认证，不绕开 spotterd 模型。

---

## 3. 设计

### 3.1 端到端流程

```
[GUI: DetailPanel 点 'Deploy / Reinstall spotterd']
   │
   ▼
[DeployDialog]                                                  ← Wails bound
   - 解析包源（local bin/ 命中？或 remote URL 下载？）
   - 探测 SSH TCP <ip>:22（10s）
   - 显示 Manifest 三件套（文件名 / 字节 / SHA-256）
   - 用户填 SSH AuthSpec { mode, user, keyPath?, password? } + 临时 sudo 密码
   - 用户确认 staging 路径（默认 /tmp/spotterd-pkg-<unix-nano>）
   ▼
[ConfirmDialog: 'This will reinstall spotterd on <hostname> at <ip> via <user>. The spotterd service on the device will be restarted. Continue?']
   │
   ▼  点 Confirm
[App.RunDeploy(handle)  → 后台 goroutine]
   ├─> ssh.Dial → *ssh.Client   (Phase: AUTH)
   ├─> sftp.NewClient → *sftp.Client
   ├─> Phase UPLOAD: loop 推 3 个文件，分 progress 回调
   │     onProgress → emit 'deploy-progress:{handle}'
   ├─> Phase INSTALL: SSH session
   │     $ sudo -S bash <staging>/install.sh
   │     stdin 一次性喂 sudo 密码 + '\n'
   │     onLine(stdout/stderr) → emit 'deploy-log:{handle}'
   │     exit code → terminal state
   │
   ├─> terminal emit 'deploy-complete:{handle}' {phase, exit_code, error?}
   ▼
[UI: 进度条到 100%，成功 toast；失败 antd Modal 显示 install.sh 末尾输出]
```

### 3.2 新增包 `internal/deployer`

```
internal/deployer/
  manifest.go         // Manifest / File struct
  package.go          // Package interface (Resolve(ctx) (Manifest, error))
  source_local.go     // LocalSource  从 <dataDir>/cache/bin/ 读
  source_remote.go    // RemoteSource GET <url> → <dataDir>/cache/bin/
  source_resolver.go  // Settings.PackageMode 决定走哪条
  auth.go             // AuthSpec struct + Dial(ctx, *AuthSpec) (*ssh.Client, error)
  auth_agent.go       // AgentSigners() 从 SSH_AUTH_SOCK 拉可用 signer
  auth_key.go         // ParsePrivateKey(path, passphrase?) (ssh.Signer, error)
  auth_password.go    // ssh.PasswordAuth (Password func() string)
  sftp_upload.go      // Upload(ctx, *sftp.Client, src string, dst string, onProgress func(int64)) (int64, error)
  ssh_exec.go         // ExecCmd(ctx, *ssh.Client, cmd string, stdin func(w io.Writer), onLine func(line, isStderr)) (exitCode int, err error)
  deployer.go         // Deployer: 串联 Package + Dial + Upload + Exec，回传状态机 + Progress 回调
  deployer_test.go    // 单测
  integration_test.go // build tag integration，docker 端到端
```

**Package 接口：**

```go
type Package interface {
    Resolve(ctx context.Context) (Manifest, error)
}
type Manifest struct {
    Arch       string
    Files      []File
    ResolvedAt time.Time
    Origin     string  // "local:<dir>" | "remote:<url>"
    StagingDir string  // 生成于设备端，由 Deployer 写入
}
type File struct {
    LocalPath  string  // 客户端路径
    RemoteName string  // 推上去后的文件名
    Size       int64
    SHA256     string
}
```

**两个具体源实现：**

| Source | 解析行为 | 失败语义 |
|---|---|---|
| `LocalSource` | `os.Stat` `<dataDir>/cache/bin/spotterd-linux-<arch>`、`spotterd.service`、`install.sh` 三件；任一缺失 → 报 `'spotterd-linux-<arch> not found in <cache/bin>'` | 直接报错，提示用户切到 remote 或手工 sync |
| `RemoteSource` | 用 `net/http.Get` 拉 `Settings.PackageReleaseURL`（模板按 arch 替换），落到 `<dataDir>/cache/bin/spotterd-linux-<arch>` | 网络错 / 404 直接报；下载完成后下一次调用 hit cache |

> YAGNI：不实现 HTTP 下载进度条（GitHub release 几 MB，几秒）；不做 release 版本切换（用户改 URL）。

**AuthSpec：**

```go
type AuthSpec struct {
    Mode       string  // "agent" | "key" | "password"
    User       string  // 默认 *Registry.Entry.Username
    Password   string  // Mode=="password"
    KeyPath    string  // Mode=="key"
    KeyPass    string  // Mode=="key" 且 encrypted
}
```

`Dial` 实现要点：
- `Mode=="agent"`：`net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))` → `agent.NewClient(conn).Signers()` → 用第一个 signer。`SSH_AUTH_SOCK` 空 → 报错 `agent not available`。
- `Mode=="key"`：`os.ReadFile(KeyPath)` → `ssh.ParsePrivateKey`（支持 ed25519 / RSA / ECDSA）。Encryped → 用 KeyPass。
- `Mode=="password"`：`ssh.Password(func() string { return Password })`。
- Host key callback：固定 `ssh.InsecureIgnoreHostKey()`（首次部署时由 UI 弹「Trust this host?」确认已在外部手工 ssh 过；本设计不做交互式信任 prompt）。
- `ssh.ClientConfig`：超时 10s、握手 10s、连接 10s。

**SFTP Upload：**

```go
func Upload(ctx context.Context, c *sftp.Client, localPath, remoteName, remoteDir string, onProgress func(done, total int64)) (int64, error)
```

- `c.MkdirAll(remoteDir)` 建 staging
- `c.Create(remoteDir + "/" + remoteName)`
- `io.Copy(dst, src)` 包外层 `progressReader{src, total, onProgress}` 每 200ms / 1MB 触发
- `c.Chmod(remotePath, 0755)` 对二进制 + shell 文件给执行位
- `c.Close()` 关闭连接

**SSH Exec：**

```go
type ExecOpts struct {
    Cmd       string
    SudoPass  func() string  // 调用时取一次；用户在 Exec 阶段可能已关闭 dialog，函数提前捕获 string
    OnLine    func(line string, isStderr bool)
    Timeout   time.Duration  // 默认 10m
}
func ExecInstall(ctx context.Context, c *ssh.Client, opts ExecOpts) (exitCode int, err error)
```

- `c.NewSession()`，`session.RequestPty("dumb", 80, 24, ssh.TerminalModes{ssh.ECHO:0})` 关闭 echo（sudo -S 输出避免被回显）
- `session.StdinPipe()` 起 goroutine：`fmt.Fprintf(stdin, "%s\n", opts.SudoPass())` → `stdin.Close()`
- `session.Stdout / Stderr` 合并 → pipe → bufio.Scanner → `opts.OnLine`
- `session.Run(cmd)` 阻塞至退出
- ctx 取消 → `session.Signal(ssh.SIGTERM)` → sleep 5s → `session.Signal(ssh.SIGKILL)` → `session.Close()`

### 3.3 Deployer 主循环

```go
type Deployer struct { /* inject Package resolver + Dialer + Logger + emitter */ }

type Handle string  // uuid

type ProgressEvent struct {
    Handle Handle
    Phase  string  // "PREPARE" | "UPLOAD" | "INSTALL" | terminal
    Done   int64
    Total  int64
    Line   string  // 仅 INSTALL 时填
    ExitCode int   // 仅 terminal 时填
    Err     error  // 仅 terminal 时填
}

type DeployRequest struct {
    DeviceID   string
    Auth       AuthSpec
    SudoPass   func() string
    Staging    string  // 用户可改，默认 /tmp/spotterd-pkg-<unix-nano>
}

func (d *Deployer) Prepare(ctx, *DeployRequest) (Handle, Manifest, error)
func (d *Deployer) Run(ctx, Handle)  // 后台运行，Progress 通过回调喂 App emitter
func (d *Deployer) Cancel(Handle)     // 取消运行中的 handle
```

**状态机：**

```
PENDING ─prep→ READY ─run→ UPLOADING ─complete→ INSTALLING ─complete→ DONE
                                          │
                                          └─fail→ FAILED    (terminal)
任意阶段 ─cancel→ CANCELED  (terminal)
DONE / FAILED / CANCELED 之间无转换
```

### 3.4 App（Wails 绑定）

新增到 `main.go` `App`：

```go
type App struct {
    // ... 已有字段 ...
    deployer *deployer.Deployer
    deployMu sync.Mutex
    deploys  map[string]*deployer.Handle  // handle → internal
}

func (a *App) PrepareDeploy(deviceID string) (deployer.PrepareResult, error)  // 返回 Manifest + handle
func (a *App) CancelDeploy(handle string) error
func (a *App) ListDeploys() []deployer.PrepareResult  // UI 重连时拉当前活跃 deploy
```

`RunDeploy` **不**显式作为 Wails bound 方法——它在 `PrepareDeploy` 后由 UI 端的 `RunDeploy(handle, AuthSpec, SudoPass)` 触发；为安全（密码不进入 binding 序列化），把 `Run` 改成从 React state 直接传 Wails 后端，实质是同一个 `App.RunDeploy` 但 `SudoPass` 走 `Inject` 而非序列化。下方案：

```go
// UI 把 SudoPass 写进 React state → 调 RunDeploySudo(handle, sudo_password)
// 该方法把 sudo_password 立刻塞到 handle 的 chan 中，立刻退出；Deployer loop 从 chan 读
func (a *App) ProvideSudoPassword(handle, password string) error  // 不落内存外；用完即丢
func (a *App) RunDeploy(handle string) error                       // 真正开跑
```

**事件名（沿用现有 `{tag}:{deviceID|handle}` 范式）：**

- `deploy-progress:{handle}` → `{phase, done, total, percent}`
- `deploy-log:{handle}` → `{line, is_stderr}`  
- `deploy-complete:{handle}` → `{phase, exit_code, error}`
- `deploy-canceled:{handle}` → `{phase, partial: bool, hint: string}`

### 3.5 前端

**3.5.1 `DetailPanel.tsx` 顶部按钮**

- 选中设备时在 `<h2>` 行最右侧加 `<Button>` "Deploy / Reinstall spotterd"（antd `<RocketOutlined />` icon + label）
- `disabled` 当 `!device.online`（命中离线不算），按钮渲染上保持可见
- 点击打开 `DeployDialog`

**3.5.2 新增组件 `frontend/src/components/DeployDialog.tsx`**

Props：
```ts
interface Props {
  open: boolean
  deviceID: string
  onClose: () => void
}
```

行为：
1. `open=true` 时立刻调 `PrepareDeploy(deviceID)`：
   - 成功 → state 切换到「Ready」展示 Manifest 三行 + 设置面板
   - 失败 → antd `alert` 显示原因（包缺失 / SSH TCP 不通），按钮 disable 「Deploy」
2. 用户在表单中调：
   - PackageMode（本地 bin 镜像命中 / 远程 URL）的可视切换；remote 模式调 `RefreshFromRemote()`
   - SSH AuthMode 三选一（agent / key file / password），各自动态显示对应字段
   - User 默认值，来自 `device.lastInfo.username || device.username || 'spotter'`
   - Sudo 密码（必填，独立字段）
3. 表单内嵌一个 `<Progress>` 组件 + 「Phase: PREPARE / UPLOAD / INSTALL / DONE」 caption
4. 订阅 Wails events `deploy-progress:{handle}`、`deploy-log:{handle}`、`deploy-complete:{handle}`、`deploy-canceled:{handle}`，把回调应用到 UI state
5. Cancel 按钮：调用 `App.CancelDeploy(handle)`
6. 「Deploy」按钮：先 pop 一个 `Modal.confirm` 二次确认框（确认文案含 hostname / ip / user / 「spotterd service will be restarted」），用户点 Confirm 才调 `App.RunDeploy(handle)` + `App.ProvideSudoPassword(handle, pwd)`

**3.5.3 i18n**

`frontend/src/i18n/dictionaries.ts` 新增：
- `detail.actions.deploy.button`
- `detail.actions.deploy.dialog.title`
- `detail.actions.deploy.dialog.confirmTitle`, `...confirmBody`, `...confirmOk`
- `detail.actions.deploy.phase.{prepare,upload,install,done,failed,canceled}`
- `detail.actions.deploy.errors.{package_not_found, ssh_unreachable, auth_failed, sudo_failed, upload_failed, exec_failed}`
- `detail.actions.deploy.fields.{authModeAgent, authModeKey, authModePassword, user, keyPath, password, sudoPassword}`

中英双语均完整。

### 3.6 Settings 扩展

`internal/clientconfig/store.go` 新增字段：

```go
type Settings struct {
    // ... 已有 ...
    PackageMode      string `json:"package_mode,omitempty"`        // "local" | "remote"
    PackageReleaseURL string `json:"package_release_url,omitempty"` // 模板 e.g. https://github.com/.../spotterd-linux-{arch}
    DeviceSSHPort    int    `json:"device_ssh_port,omitempty"`      // 默认 22
    CacheBinDir      string `json:"cache_bin_dir,omitempty"`        // 默认 <dataDir>/cache/bin/
}
```

`fillDefaults` 增加兜底：
- `PackageMode == ""` → `"local"`
- `DeviceSSHPort == 0` → `22`
- `CacheBinDir == ""` → `<dataDir>/cache/bin/`

`cmd/clientconfig/store.go` 的 `defaultSettings()` 同步更新。

**SettingsDialog** 增加对应 form field；并暴露「Sync from remote now」按钮，直接调 `deployer.SyncRemote(ctx)`，不进 deploy handle。

### 3.7 `spotter-cli deploy` 子命令

`cmd/spotter-cli/main.go` 新增：

```
spotter-cli deploy <user>@<ip> [--port=22] [--arch=arm64|amd64] [--staging=/tmp/spotterd-pkg-N] [--mode=local|remote] [--auth=agent|key|password]
```

- 不支持 sudo 密码参数 —— 用 stdin 一次性读（密码模式下 echo off；silent 模式用 `terminal.ReadPassword` Go stdlib）
- 调用 `deployer.Deployer` 同一代码路径；`ProgressEvent` 通过 stdout 行式打印（`PHASE=upload DONE=.../TOTAL=...`），方便脚本与日志转发
- exit code：`0` = installed, `1` = upload/prepare failed, `2` = install failed, `3` = canceled

> YAGNI：不实现 `--package-url` CLI flag override 远程 URL（settings 已配 + UI 用）。

---

## 4. 错误处理与边界

| 场景 | 失败点 | 错误字段 | UI 反馈 |
|------|--------|---------|---------|
| local bin 缺失 | PREPARE | `package_not_found` | dialog alert: '"spotterd-linux-<arch>" not found in <cache/bin>.' |
| remote URL 未配 | PREPARE | `package_not_configured` | dialog alert: 'No remote URL configured in Settings.' |
| TCP 22 不通 | PREPARE | `ssh_unreachable` | dialog alert: 'can\'t reach <ip>:22 (timeout 10s)' |
| SSH 认证失败 | UPLOAD | `auth_failed` | 终止 handle；UI 重置 dialog；显示 SSH 服务器回送的 reason |
| 单文件 SFTP 失败 | UPLOAD | `upload_failed` | 进度条冻结；保留 staging 路径让用户清理 |
| UPLOAD 用户取消 | UPLOAD | `canceled` | 进度条撤回；UI 显示「upload canceled」 |
| sudo 密码错 | INSTALL | `sudo_failed` | exit 1；UI 显示 install.sh 最后一行 + 'sudo: authentication failed' |
| install.sh 退出非 0 | INSTALL | `install_failed` | UI 显示完整 stdout tail；保留 staging 路径 |
| INSTALL 用户取消 | INSTALL | `canceled` (partial) | UI 显示 'install canceled mid-flight — spotterd on <device> may be in partial state. Run install.sh manually on <staging> to repair.' |
| ctx cancel 在 PREPARE | PREPARE | `canceled` | dialog 关闭；UI 静默重置 |
| 网络抖动至 download 缓 | REMOTE | `remote_slow` (不阻断) | dialog 显示进度 caption「downloading 12 MB…」 |

**幂等性：** deploy 不是幂等；UI 在 handle 还在运行时禁用 `device.online` 的 power actions，但**不**禁用该 dialog（用户可能想取消）。

**Logging：** App.logger 全程 slog，每阶段一行：`device_id, ip, user, arch, phase, bytes, exit_code`；密码字段不进日志。

**审计：** 不持久化部署审计文件；如未来需要，由 `scripts/deploy.sh` 现有的归档流程承担。

---

## 5. 测试

### 5.1 `internal/deployer` 单测（不依赖网络）

`internal/deployer/deployer_test.go`：
- `TestLocalSource_OK / TestLocalSource_MissingFile`：用 `t.TempDir()` 摆三件套验证 Resolve
- `TestRemoteSource_CacheHit`：第一次走远端、第二次命中本地 cache（mock `http.Client` 仅需 1 次实际 GET，用 `httptest.NewServer` 替身）
- `TestAuthSpec_Agent / _Key / _Password`：用 `ssh.NewServerConn` 本地端到端；Password 模式用 mock `PasswordKeyboardInteractive` 验证回调
- `TestSFTP_Upload_Progress`：loopback 起 `sftp.NewServer`，验证 `Upload` 落字节数与 onProgress 至少 1 次回调
- `TestSSH_Exec_OK / _NonZero / _StdinWrite`：loopback 跑 `cat` / `exit 7` / `sh -c 'read X; echo got=$X'` 分别覆盖成功 / 非零退出 / stdin 路径
- `TestDeployer_Run_StateMachine`：用 fake `Package + Dialer + Uploader + Executor`，覆盖 PREPARE→UPLOAD→INSTALL→DONE 与 cancel 路径

### 5.2 `internal/deployer` 集成测试（build tag `integration`）

`internal/deployer/integration_test.go`：
- `testutil` 起 docker `linuxserver/openssh-server:latest`（CI 跳过无 docker 环境），容器内预置 `user:spotter sudo NOPASSWD:ALL`
- 跑 `Prepare → Run`，验证 staging 三文件齐、`install.sh` exit 0、容器内 `systemctl is-active spotterd` = active
- `--tags integration go test` 才会跑

### 5.3 App 绑定单测

`main_test.go`：
- `TestApp_PrepareDeploy_BindArgs`：验证 Wails-bound 方法签名稳定、Manifest 字段正确流入；用现有 `Emitter` mock 模式
- `TestApp_ProvideSudoPassword_NeverPersists`：跑完后检查 `Settings.json` / `Registry.json` 内容不变

### 5.4 `spotter-cli` 子命令

`cmd/spotter-cli/main_test.go`：
- `TestRun_Deploy_Help`：参数错误路径
- 真实部署通过 e2e 集成测试覆盖（不写在这里，留给 deployer 集成测试）

### 5.5 手工 e2e 清单（在 PR description 列出）

- [ ] Windows / macOS / Linux 三端客户端分别能调出 DeployDialog；OS native 文件选择器可关可开
- [ ] 本地 bin/ 模式：mock `cache/bin/spotterd-linux-arm64` 命中，SSH device 上有临时 passwordless-sudo user，部署成功
- [ ] remote 模式：首次下载到 cache、再调一次 hit cache（验收标准 J）
- [ ] 错密码（SSH）：弹 `auth_failed`，handle 进 FAILED，重试只能改密码
- [ ] 错密码（sudo）：install.sh 报 `sudo: authentication failure`；确认按钮带 exit 1 标
- [ ] install 时按 Cancel：device 端 install.sh 收到 SIGTERM、能落回（不必然保证，因 install.sh 不一定响应）；UI 显示 partial 状态
- [ ] 同一设备二次 deploy：上一次 DONE 后 handle 列表里不残留（除非失败需清理）

### 5.6 已验证不变行为

- `make test` 全绿（本包改动不触 `internal/agentd`, `internal/scanner`, `internal/registry` 测试）
- README「已知限制」段更新文案由文档 PR 处理

---

## 6. 文档更新

### 6.1 `README.md`

- 「客户端支持远端命令执行」段：新增一行「Spotter Client 可通过 SSH + SFTP 上传 spotterd 安装包到设备并触发 install.sh；该功能假定设备有 sshd 且部署用用户已存在」
- 「不支持 SSH 直通 shell」保留：用户不能打开交互式 SSH terminal 跑任意命令
- 「已知限制」段：「不支持远端命令执行」改为「**不**支持 SSH 交互式 shell；可通过 GUI 触发预打包的 spotterd 安装 / 升级（DeployDialog）」

### 6.2 `docs/cli.md`

新增 `spotter-cli deploy` 子命令说明：

```
spotter-cli deploy <user>@<ip> [--port=22] [--auth=agent|key|password] [--mode=local|remote]
```

参数、env vars、stdout 输出格式、exit codes。

### 6.3 `docs/architecture.md`

新增段「Deployer module」描述：
- 数据流、状态机、事件流、与既存 `internal/agentd` HTTP / `internal/scanner` 的边界
- 设备端要具备 sshd + 部署用户存在的部署前置条件

### 6.4 `docs/api.md`

无新增端点（spotterd agent HTTP 接口不变）；新增段说明 spotter-client→device SSH 不是 spotterd 协议的一部 分，是经 OS sshd。

### 6.5 `SECURITY.md`

- 「Deploy / Reinstall spotterd」段：
  - 凭据处理（仅内存）
  - 设备上该用户必须用 sudo 权限；等同「让该用户能 root 设备」
  - host key 不验证（仅供小规模部署；大网络请手动维护 `~/.ssh/known_hosts`）
- 「Known unmitigated」段新增一条「host key not pinned; deploy endpoint assumes trusted LAN」

### 6.6 `docs/operations.md`

设备部署段补一句：「用 GUI 客户端 Deploy/Reinstall spotterd：Settings 配置 `PackageReleaseURL`，保证 `<cache/bin>` 中有对应架构安装包，选设备 → Deploy」。

---

## 7. 风险与回退

| 风险 | 缓解 |
|------|------|
| 客户端登录用户（部署用 SSH user）持有 sudo 权限，等于把设备 root 暴露 | UI 文案 + SECURITY.md 明确警告；二次确认强制 |
| install.sh 中途取消，设备处半装状态 | UI 提示 + 保留 staging 路径；用户能 SSH 进设备手动跑 install.sh |
| host key 不验证，理论上 MITM | 仅可信 LAN 假定；SECURITY.md 显式标注；后续 milestone 可加 `known_hosts` 交互 |
| SFTP 大文件（spotterd ~10-30MB）慢链路上传超时 | 单文件超时 5 分钟；可整 handle 重试从头推 |
| install.sh 输出量大（每秒上千行）阻塞 UI | OnLine 缓冲 100ms / 最多 200 条 batch 上 emit；UI 用 Buffer（环形 cap 1000） |
| `pkg/sftp` 上游 API 变更 / 弃用 | 当前最新稳定 v1.13.6；写 adapter 接口留切换余地；现版本非必要不升级 |
| 一个设备同时两次 deploy（用户开两次 dialog） | Deployer 按 handle 隔离；UI 用一个 dialog；并发起两个会得到两个独立 handle，但底层 SSH 串行等待 connect 即可 |

**回退路径：** 把 `internal/deployer` 整体拆离（`go mod` 中删除两个依赖）回退；client 不显示 Deploy 按钮即退化到 v0.3 行为，无强制依赖。

---

## 8. 范围之外（明确推迟）

- 凭据持久化 / OS keychain / per-device saved passwords
- 多设备批推 UI
- 与 systemd inhibit / shutdown coordination 集成
- SSH host key 验证 UI prompt
- 部署审计日志（TSV / JSONL）
- 远程 install 进度可视化（仅依赖 install.sh 自有输出）
- OTA 风格的版本选择器 / 多版本并存
- 部署前的 dry-run / lint
- spotter-cli 的 `--from-url` 覆盖（一直从 settings 读）
- 自动识别设备 `Arch` 失败时的 UX 兜底（先默认 `arm64`，仅协议层 hint——见 §1 注）
