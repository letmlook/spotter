# Release 签名与 provenance（cosign + SLSA）

从 v1.0.0 起，每个 GitHub Release 都附带 **sigstore cosign 签名** +
**SLSA Level 3 provenance**。消费者可以用 `cosign verify-blob` 离线
验证任一二进制确实来自本仓库的某个 commit。

## 产物

每个 release assets 旁边会出现：

| 文件 | 内容 |
| --- | --- |
| `SHA256SUMS.sig` | cosign 对 `SHA256SUMS` 的数字签名 |
| `SHA256SUMS.cosign` | cosign bundle JSON（含 Fulcio 短期证书 + Rekor 透明度日志条目） |
| `SHA256SUMS.pem` | 用于验证的签发证书（短期 Fulcio 证书，OIDC-issued） |
| `<binary>.sig` / `<binary>.cosign` / `<binary>.pem` | 每个二进制自己的签名三元组 |
| `intoto.attestation.jsonl` | SLSA provenance attestation，NDJSON 格式 |

## 工作原理

`release.yml` 在 release 之后跑两个 job：

1. **`sign`** — 下载所有 artifact + SHA256SUMS
   - 装 `sigstore/cosign-installer@v3`（v2.4.1）
   - `cosign sign-blob --bundle` 每个文件
   - 短期 Fulcio 证书通过 OIDC 从 GitHub Actions runner 申请（需要 `id-token: write` 权限）
   - 签名上传到 sigstore 公共 Rekor 透明度日志
   - 生成 SLSA provenance

2. **`sign-attach`** — 把 `.sig` / `.cosign` / `.pem` 三元组 + provenance 二次 attach 到 release

**为什么 keyless 而不是 KMS？**

- 仓库不再持有私钥（消除"私钥泄露 = 全部 release 被冒充"风险）
- Fulcio 证书自动过期（默认 10 分钟），即使签名被复制也很快失效
- Rekor 提供不可篡改的审计链
- 消费者侧只需要 `cosign` CLI + 公开的 Fulcio 信任根 + Rekor 公钥

**为什么不直接用 sigstore 公共 Rekor？**

- Rekor 公共实例是免费的，吞吐足够本项目使用
- 未来如果 release 频率上升或合规要求本地 Rekor，可在 `sign` job 里加
  `--rekor-url https://rekor.internal.example.com` + Rekor 公钥 env

## 验证

### 验证 SHA256SUMS 的签名

```bash
# 1. 下载 cosign（macOS: brew install cosign；Linux: 见 sigstore/cosign）
cosign version  # v2.0+

# 2. 下载 SHA256SUMS + .sig + .cosign + .pem 到同一目录
# 3. 验证
cosign verify-blob \
  --signature SHA256SUMS.sig \
  --certificate SHA256SUMS.pem \
  --bundle SHA256SUMS.cosign \
  --certificate-identity-regexp 'https://github.com/spotter/spotter' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  SHA256SUMS
```

成功输出：
```
Verified OK
```

### 验证某个二进制

```bash
cosign verify-blob \
  --signature spotterd-linux-arm64.sig \
  --certificate spotterd-linux-arm64.pem \
  --bundle spotterd-linux-arm64.cosign \
  --certificate-identity-regexp 'https://github.com/spotter/spotter' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  spotterd-linux-arm64
```

### 验证 SLSA provenance

```bash
# 1. 装 slsa-verifier（Go 二进制）
go install github.com/slsa-framework/slsa-verifier/v2/cli/verifier@latest

# 2. 下载 binary + provenance
# 3. 验证
slsa-verifier verify-artifact \
  --provenance-path intoto.attestation.jsonl \
  --source-uri "github.com/spotter/spotter" \
  --source-tag v1.0.0 \
  spotterd-linux-arm64
```

## 故障排查

**`cosign sign-blob` 失败：tlog upload failed**

- Rekor 公共实例偶发限流。重试 2-3 次
- 长期方案：自托管 Rekor

**`cosign verify-blob` 失败：certificate identity does not match**

- 确认 `--certificate-identity-regexp` 包含实际 GitHub repo URL
- 确认 `--certificate-oidc-issuer` 是 `https://token.actions.githubusercontent.com`（GitHub Actions 固定 issuer）

**SLSA provenance 不匹配**

- provenance 里钉死的 source URI 必须和实际 release tag 一致
- 如果 re-tag 同一个 commit，provenance 仍然指向旧 tag — 这正是我们想要的（不可篡改）

## 密钥管理

**本项目不持有任何 release 私钥。** Fulcio 短期证书（10 分钟 TTL）由 GitHub OIDC 颁发，
签名自动提交到 Rekor 透明度日志。这意味着：

- ✅ 没有私钥需要轮换 / 备份
- ✅ 没有 .env / secret 泄露风险
- ✅ 即使有人复用了 release artifact，签名在数十分钟后自动失效
- ⚠️ 依赖 sigstore 公共服务 — 如果 Rekor 不可用，签名仍可生成（`--tlog-upload=false`）但不被透明度日志背书

## 续期 / 迁移

- 短期 Fulcio 证书 → 信任根 `https://accounts.google.com/...` 来自 Google Trust Services
- cosign 工具 → 跟 `sigstore/cosign-installer@v3` 升级，pin 固定版本
- 未来如果要切到自托管 PKI：在 `sign` job 加 `--fulcio-url` + `--rekor-url` + 对应 trust root
