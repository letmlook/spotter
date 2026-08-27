# macOS 代码签名与公证（Codesign + Notarize）

Spotter 在 macOS 上以 `.app` bundle 形式分发，从 v1.0.0 起启用
**Apple Developer ID 签名 + Notarization**。没有签名的 `.app`
在 macOS Catalina+ 上首次启动会弹「无法确认开发者」警告；未公证
的 binary 在 Gatekeeper 强化策略下会被直接拒绝。

本文件面向 release operator 描述完整流程；终端用户见
[`install-mac.md`](install-mac.md)。

## 前置条件

1. **Apple Developer Program** 账号（$99/年），`developer.apple.com` 注册
2. **Developer ID Application** 证书（Xcode → Accounts → Manage Certificates → +）
3. **App-Specific Password**（用于 `notarytool`），在
   [appleid.apple.com](https://appleid.apple.com/account/manage) → App-Specific Passwords 生成
4. **Keychain Profile**（推荐）或 Apple ID + 上面那个 App-Specific Password

## 一次性配置（CI 场景）

### 1. 导出 Developer ID 证书为 .p12

在 macOS 本地：

```bash
# 打开 Keychain Access → 找到 "Developer ID Application: <name> (<teamid>)"
# 右键 → Export "Developer ID Application..." → 存为 cert.p12
# 设置一个强密码（后面会作为 secret 用）

# base64 编码后存入 GitHub secret APPLE_DEVELOPER_IDENTITY_P12
base64 -i cert.p12 | pbcopy
```

### 2. 在 GitHub repo 设置以下 secrets

| Secret | 含义 |
| --- | --- |
| `APPLE_DEVELOPER_ID_APP` | `Developer ID Application: Your Name (TEAMID)` |
| `APPLE_TEAM_ID` | 10 字符 team id |
| `APPLE_KEYCHAIN_PASSWORD` | 随机串（解锁临时 keychain），如 `openssl rand -hex 16` |
| `APPLE_DEVELOPER_IDENTITY_P12` | 上一步 base64 后的 p12 |
| `APPLE_NOTARYTOOL_PROFILE` | `xcrun notarytool store-credentials <name>` 后填的 profile 名 |

（替代方案：使用 `APPLE_ID` + `APPLE_APP_SPECIFIC_PASSWORD` 代替 profile，
但 profile 更安全——不需要在 CI 里明文存密码。）

### 3. 验证配置

PR 到 master 后，`.github/workflows/client-cross-compile.yml` 的
macOS job 会在 `Sign + notarize` 步骤自动激活并运行
`scripts/macos/sign-and-notarize.sh`。失败时 step 是
`continue-on-error: true`（避免阻塞 PR），但合并到 master 后
正式 release job 仍会失败并阻止 release 上传，提示
operator 修复。

## 本地 dry-run

想在本地完整跑一遍签名 + 公证（不阻塞 release）：

```bash
brew install create-dmg

# 准备好 secrets（同上）
export APPLE_DEVELOPER_ID_APP="Developer ID Application: ..."
export APPLE_TEAM_ID="ABCDE12345"
export APPLE_KEYCHAIN_PASSWORD="$(openssl rand -hex 16)"
export APPLE_DEVELOPER_IDENTITY_P12="$(base64 -i ~/certs/cert.p12)"
export APPLE_NOTARYTOOL_PROFILE="my-notary-profile"  # 或 APPLE_ID + APPLE_APP_SPECIFIC_PASSWORD

# 编译 + 签名 + 公证 + DMG
make client
./scripts/macos/sign-and-notarize.sh  # APP_BUNDLE=build/bin/Spotter.app 默认
./scripts/macos/make-dmg.sh

# 验证
spctl --assess --type execute --verbose=2 build/bin/Spotter.app
xcrun stapler validate build/bin/Spotter.app
```

## 故障排查

**`codesign` 失败：no identity found**

- 确认 `APPLE_DEVELOPER_ID_APP` 完全匹配 Keychain 里的证书名（含括号里的 team id）
- 确认 `security list-keychain -d user -s "$KEYCHAIN"` 在脚本里成功（否则 codesign 找不到 cert）

**`notarytool` 失败：`Package Invalid`**

- 最常见：xattrs 缺失。`ditto` 会保留 xattrs，但 `zip` 不会——脚本里用 `ditto -c -k --keepParent`
- 第二个常见：签名时缺 `--options runtime`——脚本里已加

**`stapler staple` 失败：The staple and validate action failed**

- Apple notary 状态偶尔滞后。`xcrun notarytool history -p <profile>` 查 submission id 状态
- 等待几分钟后重跑（脚本里 `xcrun notarytool submit --wait` 已等到位）

**`create-dmg` 失败：could not find Spotlight**

- 在 CI sandbox 里跑 `mdimport` 之前会随机失败
- 加 `MDIMPORT_DISABLE_CACHE=1 create-dmg ...` 试试

## 不签名会怎样？

`spotter-client-cross-compile.yml` 的 `Sign + notarize` 步骤
**有 secrets 时**才执行，`continue-on-error: true` 防止 CI 阻塞。
未签名 `.app` 仍会上传 artifact + release 附件。

对个人用户：通过 `right-click → Open` 第一次可以绕过 Gatekeeper
（之后会记住）。但**对组织分发的用户**，未签名 binary 在
default settings 下会被永久拒——所以 v1.0.0 起 release 流程
要求 secrets 配置完整。

## 续期 / 重新生成

- 证书每年需在 Apple Developer 后台续期
- `.p12` 在续期后必须重新导出 + 更新 `APPLE_DEVELOPER_IDENTITY_P12`
- 旧 release 不需要重新签名——已公证的二进制带时间戳，cert 过期后 Gatekeeper 仍认可
