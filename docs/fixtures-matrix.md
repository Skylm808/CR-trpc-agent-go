# Fixture 预期矩阵

本文档定义 `testdata/fixtures/` 中每个 diff 的审查场景、预期 rule_id、severity 与 status。当前 fixture 测试已经断言报告中的 rule_id/severity/status，而不仅是报告文件存在。

## 公开样本

| Fixture | 场景 | 预期 rule_id | 预期 severity | 预期 status | 当前检出 |
|---------|------|-------------|---------------|-------------|----------|
| `safe.diff` | 干净 Go 变更，无风险 | — | — | — | ✅ 零 finding |
| `secret.diff` | 硬编码 API Key / token | `secret-leak` | critical | finding | ✅ |
| `secret-shapes.diff` | API key、LLM key、OpenAI key、Bearer、GitHub token、password 多形态 | `secret-leak` | critical | finding | ✅ 6 条，placeholder 不报 |
| `panic.diff` | 新增函数直接 panic | `panic-direct` | high | finding | ✅ |
| `todo.diff` | 新增 TODO/FIXME 标记 | `todo-marker` | medium | finding | ✅ |
| `test-missing.diff` | 新增无 error 返回的函数，缺测试 | `missing-test-hint` | low | warning | ✅ |
| `missing-test.diff` | 同上，验证稳定性 | `missing-test-hint` | low | warning | ✅ |
| `goroutine.diff` | 裸 `go func()` 无生命周期管理 | `goroutine-leak` | high | finding | ✅ |
| `context.diff` | context 派生后未 cancel | `context-leak` | high | finding | ✅ |
| `resource.diff` | 打开资源未关闭 | `resource-leak` | high | finding | ✅ |
| `db-lifecycle.diff` | DB 连接/事务未 Close/Rollback | `db-lifecycle` | high | finding | ✅ |
| `dedupe.diff` | 同一文件同一类重复触发 | `panic-direct` | high | finding | ✅ 只保留 1 条 |
| `realistic-service-risk.diff` | 多文件 PR 形态，组合 secret、panic、goroutine、context、resource、DB、TODO、缺测试 | `secret-leak`、`panic-direct`、`goroutine-leak`、`context-leak`、`resource-leak`、`db-lifecycle`、`todo-marker`、`missing-test-hint` | critical/high/medium/low | finding/warning | ✅ |
| `sandbox-fail.diff` | Skill 脚本退出非 0 | — | — | needs_human_review warning | ✅ 不崩溃 |
| `sandbox-timeout.diff` | Skill 脚本超时 | — | — | needs_human_review warning | ✅ 不崩溃 |

## Issue 8 类公开样本映射

Issue 要求至少 8 条公开 diff 样本必须全部可运行并生成报告。当前 15 条 fixture 覆盖如下：

| Issue 样本类别 | 对应 fixture | 证明点 |
|----------------|--------------|--------|
| 无问题 diff | `safe.diff` | 零 finding / warning |
| 安全问题 | `secret.diff` | `secret-leak` critical finding |
| goroutine / context 泄漏 | `goroutine.diff`、`context.diff` | 生命周期风险 high finding |
| 资源未关闭 | `resource.diff` | `resource-leak` high finding |
| 数据库连接生命周期问题 | `db-lifecycle.diff` | `db-lifecycle` high finding |
| 测试缺失 | `test-missing.diff`、`missing-test.diff` | `missing-test-hint` low warning |
| 重复 finding | `dedupe.diff` | 同类重复只保留 1 条 |
| 沙箱执行失败 | `sandbox-fail.diff`、`sandbox-timeout.diff` | failed / timed_out 不导致任务崩溃 |
| 敏感信息脱敏 | `secret-shapes.diff` | 多形态 secret 脱敏，placeholder 不误报 |
| 额外错误处理样本 | `panic.diff` | `panic-direct` high finding |
| 真实 PR 组合样本 | `realistic-service-risk.diff` | 多文件 patch 中同时覆盖多类高风险和 warning |

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
- `secret-shapes.diff` 对多形态 secret 生成多条脱敏 finding，同时不把 placeholder 当高危。

## 样本摘要

### safe.diff

预期：0 findings，0 warnings。

### secret.diff

预期：`secret-leak` critical；evidence 中密钥脱敏。

### secret-shapes.diff

预期：6 条 `secret-leak` critical；覆盖 `apiKey`、`llmkey`、`openaiKey`、`client_secret`、Bearer token 和 password；`tokenPlaceholder = "dummy"` 与 `retryTokenTimeoutSeconds` 不报 critical。

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

### realistic-service-risk.diff

预期：7 条 finding、1 条 warning。该样本用一个顶层 `service.go` 承载高风险 Go 改动，同时包含一个无风险文档文件，模拟真实 PR 的多文件 patch 形态。报告必须检出：

- `secret-leak` critical finding，且 evidence 中密钥脱敏。
- `context-leak`、`resource-leak`、`panic-direct`、`db-lifecycle`、`goroutine-leak` high finding。
- `todo-marker` medium finding。
- `missing-test-hint` low warning。

### sandbox-fail.diff

预期：review 不崩溃；报告生成；SQLite `sandbox_runs.status=failed`；metrics `exception_counts` 包含 `sandbox_failed`。

### sandbox-timeout.diff

预期：review 不崩溃；报告生成；SQLite `sandbox_runs.status=timed_out`；metrics `exception_counts` 包含 `sandbox_failed`。

## 隐藏样本与评测

Issue 要求隐藏样本检出率 ≥ 80%、误报 ≤ 15%。当前不提交隐藏样本本体，但 `scripts/eval.sh` 已支持外部 fixture root、external expected TSV、阈值门禁和报告保留目录：

```bash
CR_AGENT_EVAL_FIXTURES_ROOT=/path/to/hidden-fixtures \
CR_AGENT_EVAL_FIXTURES="hidden-001.diff hidden-002.diff" \
CR_AGENT_EVAL_MATRIX=/path/to/expected.tsv \
CR_AGENT_EVAL_REPORT_ROOT=/tmp/cr-agent-hidden-reports \
scripts/eval.sh
```

默认门禁是 `CR_AGENT_EVAL_MIN_RECALL=0.800`、`CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE=0.150`。公开内置矩阵额外要求 `missing_findings=0` 和 `unexpected_findings=0`。

## 示例报告输出

已提交：

```text
examples/review_report.json
examples/review_report.md
examples/review_diagnostics.json
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
