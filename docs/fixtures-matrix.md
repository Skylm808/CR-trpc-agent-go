# Fixture 预期矩阵

本文档定义 `testdata/fixtures/` 中每个 diff 的审查场景、预期 rule_id、severity 与是否需沙箱，供端到端测试从「报告存在」升级为「结果符合预期」。

## 公开样本（Issue 交付物）

| Fixture | 场景 | 预期 rule_id | 预期 severity | 预期 status | 需沙箱 | 当前检出 |
|---------|------|-------------|---------------|-------------|--------|---------|
| `safe.diff` | 干净 Go 变更，无风险 | — | — | — | 否 | ✅ 零 finding |
| `secret.diff` | 硬编码 API Key / token | `secret-leak` | critical | finding | 否 | ✅ |
| `panic.diff` | 新增函数直接 panic | `panic-direct` | high | finding | 否 | ✅ |
| `todo.diff` | 新增 TODO/FIXME 标记 | `todo-marker` | medium | finding | 否 | ✅ |
| `test-missing.diff` | 新增无 error 返回的函数，缺测试 | `missing-test-hint` | low | warning | 否 | ✅ |
| `missing-test.diff` | 同上（重复场景，验证稳定） | `missing-test-hint` | low | warning | 否 | ✅ |
| `goroutine.diff` | 裸 `go func()` 无生命周期管理 | `goroutine-leak` | high | finding | 可选 | ⬜ 待实现 |
| `context.diff` | context 传入但未 cancel/超时 | `context-leak` | high | finding | 可选 | ⬜ 待实现 |
| `resource.diff` | 打开资源未关闭（如 os.Open 无 defer Close） | `resource-leak` | high | finding | 可选 | ⬜ 待实现 |
| `db-lifecycle.diff` | DB 连接/事务未 Close/Rollback | `db-lifecycle` | high | finding | 可选 | ⬜ 待实现 |

## 待新增样本

| Fixture | 场景 | 预期行为 | 用途 |
|---------|------|---------|------|
| `dedupe.diff` | 同一文件同一行触发同一 rule 两次 | 只保留 1 条 finding | 验收标准 7（去重） |
| `sandbox-fail.diff` | 触发沙箱 timeout 或命令失败 | review 不崩溃；DB 有 sandbox run status=failed/timeout | 验收标准 4 |

## 各 Fixture 内容摘要

### safe.diff

```go
func Add(a, b int) int { return a + b }
```

预期：0 findings，0 warnings。

### secret.diff

```go
const apiKey = "sk-1234567890abcdef"
```

预期：`secret-leak` critical；evidence 中密钥已脱敏为 `[REDACTED]`。

### panic.diff

```go
func Crash() { panic("boom") }
```

预期：`panic-direct` high。

### todo.diff

```go
// TODO(you): wire the real implementation
```

预期：`todo-marker` medium。

### test-missing.diff / missing-test.diff

```go
func Serve() {}          // 或 func Add(a, b int) int { ... }
```

预期：`missing-test-hint` low warning（非 finding）。

### goroutine.diff

```go
func Start() {
    go func() {}  // 无 WaitGroup/channel/ctx 管理
}
```

预期（待实现）：`goroutine-leak` high。

启发式：新增行含 `go func` 且同 hunk 无 `WaitGroup`/`done`/`ctx.Done`/`sync.`。

### context.diff

```go
func Handle(ctx context.Context) {
    _ = ctx  // 无 defer cancel、无 WithTimeout
}
```

预期（待实现）：`context-leak` high。

启发式：参数或赋值含 `context.WithCancel/WithTimeout/WithDeadline` 但同函数无 `defer cancel()`。

### resource.diff

```go
func Open() *os.File {
    return nil  // 或 os.Open 无 defer Close
}
```

预期（待实现）：`resource-leak` high。

启发式：新增行含 `os.Open`/`OpenFile`/`conn` 但同 hunk 无 `defer .*Close`。

### db-lifecycle.diff

```go
func Query() error {
    return nil  // 或 db.Begin/sql.Open 无 defer Close/Rollback
}
```

预期（待实现）：`db-lifecycle` high。

启发式：新增行含 `sql.Open`/`Begin`/`BeginTx` 但无 `defer.*Close`/`defer.*Rollback`。

## 测试断言模式（Target）

当前测试（`fixtures_test.go`）仅断言报告文件存在。目标升级为：

```go
t.Run("secret.diff", func(t *testing.T) {
    result := runFixture("secret.diff")
    assertFinding(t, result, "secret-leak", "critical", "finding")
    assertRedacted(t, result) // evidence 不含明文 sk-
    assertZeroFindings(t, result, "safe.diff") // 对照
})
```

## 隐藏样本（评测用）

Issue 验收要求隐藏样本检出率 ≥ 80%、误报 ≤ 15%。

| 目录 | 用途 | 状态 |
|------|------|------|
| `testdata/fixtures/` | 公开样本，CI 必跑 | ✅ |
| `testdata/hidden/` | 隐藏样本，评测/CI 注入 | ⬜ 待建 |
| `scripts/eval.sh` | 输出 recall / precision / 耗时 | ⬜ 待建 |

隐藏样本不提交到公开仓库，或通过 CI secret 注入。

## 示例报告输出（Target）

规则补全并稳定后，将 golden 报告写入：

```
testdata/expected/
  safe.review_report.json
  secret.review_report.json
  ...
  safe.review_report.md
  secret.review_report.md
```

用于回归测试：实际输出与 expected diff 比较（或只比较 findings 列表）。

## 相关文档

- [implementation-plan.md](implementation-plan.md) — M2 任务清单
- [issue-2004-traceability.md](issue-2004-traceability.md) — 验收标准追踪
- [data-contract.md](data-contract.md) — Finding 字段定义
