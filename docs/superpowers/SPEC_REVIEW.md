# Spec 同步流程（SPEC REVIEW）

## 目标

让 specs/ 始终是 source of truth。当代码默认值、字段或行为与 spec 不一致时，要么改 spec，要么改代码。所有偏差必须先在本流程登记，不允许"代码悄悄改默认值却 spec 没动"。

## 流程图

```
发现偏差  
  ↓
打开 PR，标签 `spec-drift`  
  ↓
1. 决定方向（改 spec 或回滚代码）  
2. 在 SPEC_DEVIATIONS.md 加一条 DEVIATION-xxx  
3. 同步更新对应 spec 文件  
4. 在 PR 描述引用 DEVIATION-xxx 与 spec 行号  
  ↓
reviewer 确认 spec ↔ code ↔ DEVIATIONS.md 三者一致  
  ↓
merge → CI 跑 `make spec-check` 验证不再漂移  
```

## CI: `make spec-check`

CI（`.github/workflows/go.yml` 的 `spec-check` job）跑：

```bash
# 1. 比对 docs/superpowers/SPEC_DEVIATIONS.md 是否存在
test -f docs/superpowers/SPEC_DEVIATIONS.md || { echo "missing"; exit 1; }

# 2. 抓 spec 中所有出现 "默认 false" 的行，要求 DEVIATIONS.md 有对应条目
grep -nrE '默认\s*(false|关|off)' docs/superpowers/specs/ \
  | while IFS=: read -r f l t; do
    if ! grep -qF "$t" docs/superpowers/SPEC_DEVIATIONS.md; then
      echo "spec drift: $f:$l  $t"
      exit 1
    fi
done

# 3. 抓代码中 default-defaulted-to-X 的 TOML 行，要求 spec 中描述一致
grep -nrE 'defaultLogTail|maxLogTail' internal/agentd/ \
  | while IFS=: read -r f l t; do
    if ! grep -qF "$t" docs/superpowers/specs/; then
      echo "code drift: $f:$l"
      exit 1
    fi
done
```

`spec-check` 失败即不允许 merge。

## 字段语义约定

为减少后续偏差，spec 文件统一使用下列措辞：

- **默认值**：`默认值：true/false` —— single line，便于 grep 比对。
- **是否 opt-in**：`opt-in` 跟"默认 false"等价；写 `opt-out` 跟"默认 true"等价。
- **新字段**：spec 必须先在 §"Protocol Schema" 加字段定义，再 commit 代码。

## 工具支持

- `make spec-check`：CI 比对（必跑）。
- `make spec-lint`：本地跑，editor 集成可选。
- 本地 PR 模板要求勾选 "已同步 SPEC_DEVIATIONS.md"。
