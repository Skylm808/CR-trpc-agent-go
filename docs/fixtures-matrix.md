# Fixture 预期矩阵

本文档定义 `testdata/fixtures/` 中每个 diff 的审查场景、预期 rule_id、severity 与 status。当前 fixture 测试已经断言报告中的 rule_id/severity/status，而不仅是报告文件存在。

## 公开样本

| Fixture | 场景 | 预期 rule_id | 预期 severity | 预期 status | 当前检出 |
|---------|------|-------------|---------------|-------------|----------|
| `safe.diff` | 干净 Go 变更，无风险 | — | — | — | ✅ 零 finding |
| `secret.diff` | 硬编码 API Key / token | `secret-leak` | critical | finding | ✅ |
| `panic.diff` | 新增函数直接 panic | `panic-direct` | high | finding | ✅ |
| `todo.diff` | 新增 TODO/FIXME 标记 | `todo-marker` | medium | finding | ✅ |
| `test-missing.diff` | 新增无 error 返回的函数，缺测试 | `missing-test-hint` | low | warning | ✅ |
| `missing-test.diff` | 同上，验证稳定性 | `missing-test-hint` | low | warning | ✅ |
| `goroutine.diff` | 裸 `go func()` 无生命周期管理 | `goroutine-leak` | high | finding | ✅ |
| `context.diff` | context 派生后未 cancel | `context-leak` | high | finding | ✅ |
| `resource.diff` | 打开资源未关闭 | `resource-leak` | high | finding | ✅ |
| `db-lifecycle.diff` | DB 连接/事务未 Close/Rollback | `db-lifecycle` | high | finding | ✅ |
| `dedupe.diff` | 同一文件同一类重复触发 | `panic-direct` | high | finding | ✅ 只保留 1 条 |
| `sandbox-fail.diff` | Skill 脚本退出非 0 | — | — | needs_human_review warning | ✅ 不崩溃 |
| `sandbox-timeout.diff` | Skill 脚本超时 | — | — | needs_human_review warning | ✅ 不崩溃 |

## 断言模式

测试入口：`cmd/review-agent/fixtures_test.go`。

当前每个 fixture 都通过 CLI/Agent 链路生成 `review_report.json`，再断言：

- findings 数量。
- warnings 数量。
- `rule_id`。
- `severity`。
- `status`。
- secret evidence 中不包含原始密钥。
- `safe.diff` 零 finding。
- `dedupe.diff` 只保留一条 finding。

## 样本摘要

### safe.diff

预期：0 findings，0 warnings。

### secret.diff

预期：`secret-leak` critical；evidence 中密钥脱敏。

### panic.diff

预期：`panic-direct` high。

### todo.diff

预期：`todo-marker` medium。

### test-missing.diff / missing-test.diff

预期：`missing-test-hint` low warning，不混入 findings。

### goroutine.diff

预期：`goroutine-leak` high。

启发式：新增行含 `go func` 或 `go `，同 hunk 无 WaitGroup、ctx.Done、errgroup、done、sync 等生命周期信号。

### context.diff

预期：`context-leak` high。

启发式：新增行含 `context.WithCancel/WithTimeout/WithDeadline`，同 hunk 无 `defer cancel()`、`ctx.Done` 或 `cancel()`。

### resource.diff

预期：`resource-leak` high。

启发式：新增行含 `os.Open` / `os.OpenFile` / `os.Create`，同 hunk 无 `defer` 或 `Close()`。

### db-lifecycle.diff

预期：`db-lifecycle` high。

启发式：新增行含 `sql.Open` / `.BeginTx` / `.Begin(`，同 hunk 无 `Rollback()` 或 `Close()`。

### sandbox-fail.diff

预期：review 不崩溃；报告生成；SQLite `sandbox_runs.status=failed`；metrics `exception_counts` 包含 `sandbox_failed`。

### sandbox-timeout.diff

预期：review 不崩溃；报告生成；SQLite `sandbox_runs.status=timed_out`；metrics `exception_counts` 包含 `sandbox_failed`。

## 隐藏样本与评测

Issue 要求隐藏样本检出率 ≥ 80%、误报 ≤ 15%。当前还缺：

| 目录/脚本 | 用途 | 状态 |
|----------|------|------|
| `testdata/hidden/` | 隐藏样本，评测/CI 注入 | ⬜ |
| `scripts/eval.sh` 或 Go eval command | 输出 recall / precision / 耗时 | ⬜ |

建议隐藏样本不要提交到公开仓库，或通过 CI secret/artifact 注入。

## 示例报告输出

已提交：

```text
examples/review_report.json
examples/review_report.md
```

如需 fixture 级 golden report，后续可新增：

```text
testdata/expected/
  safe.review_report.json
  secret.review_report.json
  ...
```

## 相关文档

- [implementation-plan.md](implementation-plan.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [data-contract.md](data-contract.md)
