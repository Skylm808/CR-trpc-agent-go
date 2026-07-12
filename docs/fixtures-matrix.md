# Fixture 预期矩阵

本文档定义 `testdata/fixtures/` 中每个 diff 的审查场景、预期 rule_id、severity 与 status。`scripts/eval.sh` 对完整矩阵断言 rule_id/severity/status；可选 Go harness 还会逐项断言报告内容，而不仅是文件存在。

## Fixture 与 Holdout 的职责

- Fixture 是开发集：规则作者会直接查看样本，用它驱动实现和日常回归。例如用 `os.Open` 后缺少 `Close` 的 diff 开发资源生命周期规则。
- Holdout 是本地评估集：规则先在 fixture 上完成，再运行结构、变量名和组合方式不同的 holdout，检查泛化与误报。例如 `os.OpenFile` 使用另一变量名且数行后安全 `defer Close`，可以暴露只匹配固定文本或只看上一行的过拟合规则。
- 仓库内 holdout 仍对维护者可见，只能依靠开发纪律减少针对性调参；外部验收方持有的 hidden matrix 才是真正不可见测试集。

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
| `http-body.diff` | HTTP response body 未关闭 | `http-body-close` | high | finding | ✅ |
| `sql-string-concat.diff` | SQL 字符串拼接 | `sql-string-concat` | critical | finding | ✅ |
| `command-injection.diff` | shell / 动态命令执行 | `command-injection` | critical | finding | ✅ |
| `context-background.diff` | 已有 ctx 的函数中使用 `context.Background()` | `context-background-misuse` | medium | finding | ✅ |
| `mutex-unlock.diff` | Mutex Lock 后无可见 Unlock | `mutex-unlock-missing` | high | finding | ✅ |
| `defer-in-loop.diff` | loop 内 defer | `defer-in-loop` | medium | finding | ✅ |
| `bare-return-err.diff` | 裸 `return err` 无上下文 | `bare-return-err` | medium | finding | ✅ |
| `string-concat-loop.diff` | loop 内字符串 `+=` 拼接 | `string-concat-loop` | low | needs_human_review | ✅ |
| `dedupe.diff` | 同一文件不同行出现同类问题 | `panic-direct` | high | finding | ✅ 保留 2 条；只去除同一行同一类重复 |
| `realistic-service-risk.diff` | 多文件 PR 形态，组合 secret、panic、goroutine、context、resource、DB、TODO、缺测试和低置信性能风险 | `secret-leak`、`panic-direct`、`goroutine-leak`、`context-leak`、`resource-leak`、`db-lifecycle`、`todo-marker`、`missing-test-hint`、`string-concat-loop` | critical/high/medium/low | finding/warning/needs_human_review | ✅ |
| `sandbox-fail.diff` | Skill 脚本退出非 0 | — | — | needs_human_review warning | ✅ 不崩溃 |
| `sandbox-timeout.diff` | Skill 脚本超时 | — | — | needs_human_review warning | ✅ 不崩溃 |

## Holdout / Adversarial 样本

`testdata/holdout/` 是提交到仓库的自包含 holdout matrix，用于覆盖 public fixture 之外的误报边界、组合风险和无 API key 的 fake provider 增量路径。

运行入口：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/holdout_eval.sh
```

| Fixture | 场景 | 预期 rule_id | 预期 severity | 预期 status | 当前检出 |
|---------|------|-------------|---------------|-------------|----------|
| `holdout-safe-refactor.diff` | 干净 helper refactor | — | — | — | ✅ 零 finding |
| `holdout-placeholder-secret.diff` | placeholder secret-like 名称和值 | — | — | — | ✅ 零 critical finding |
| `holdout-secret-private-key.diff` | private key 形态泄漏 | `secret-leak` | critical | finding | ✅ |
| `holdout-lifecycle-combo.diff` | context、资源和 DB 生命周期组合风险 | `context-leak`、`resource-leak`、`db-lifecycle` | high | finding | ✅ |
| `holdout-expanded-go-risks.diff` | 扩展 Go CR 规则组合风险 | `http-body-close`、`sql-string-concat`、`command-injection`、`context-background-misuse`、`mutex-unlock-missing`、`defer-in-loop`、`bare-return-err`、`string-concat-loop` | critical/high/medium/low | finding/needs_human_review | ✅ |
| `holdout-expanded-safe-patterns.diff` | 扩展 Go CR 规则 false-positive guardrail | — | — | — | ✅ 零 finding |
| `model-semantic.diff` | generic deterministic fake model 语义增量路径 | `fake-model-semantic-risk` | medium | finding | ✅ `source=fake_model` |
| `model-authz-bypass.diff` | 非 admin 空 owner 授权绕过 | `fake-model-authz-bypass` | high | finding | ✅ `source=fake_model` |
| `model-nil-boundary.diff` | nil/zero-value 边界语义变化 | `fake-model-nil-boundary` | medium | finding | ✅ `source=fake_model` |
| `model-state-inconsistency.diff` | 内存状态和持久化状态不一致 | `fake-model-state-inconsistency` | medium | finding | ✅ `source=fake_model` |
| `model-transaction-semantic.diff` | 语义失败路径仍可能提交事务 | `fake-model-transaction-semantic` | high | finding | ✅ `source=fake_model` |
| `model-error-swallow.diff` | 错误被吞掉并返回成功 | `fake-model-error-swallow` | high | finding | ✅ `source=fake_model` |
| `model-safe-semantic.diff` | 安全边界收紧的 semantic guardrail | — | — | — | ✅ 零 finding |

## Issue 8 类公开样本映射

Issue 要求至少 8 条公开 diff 样本必须全部可运行并生成报告。当前 23 条 fixture 覆盖如下：

| Issue 样本类别 | 对应 fixture | 证明点 |
|----------------|--------------|--------|
| 无问题 diff | `safe.diff` | 零 finding / warning |
| 安全问题 | `secret.diff` | `secret-leak` critical finding |
| goroutine / context 泄漏 | `goroutine.diff`、`context.diff` | 生命周期风险 high finding |
| 资源未关闭 | `resource.diff` | `resource-leak` high finding |
| 数据库连接生命周期问题 | `db-lifecycle.diff` | `db-lifecycle` high finding |
| HTTP body 生命周期 | `http-body.diff` | `http-body-close` high finding |
| SQL / 命令注入 | `sql-string-concat.diff`、`command-injection.diff` | critical security findings |
| context / mutex / loop / error 细节 | `context-background.diff`、`mutex-unlock.diff`、`defer-in-loop.diff`、`bare-return-err.diff`、`string-concat-loop.diff` | medium/high findings 和 low human-review warning |
| 测试缺失 | `test-missing.diff`、`missing-test.diff` | `missing-test-hint` low warning |
| 重复 finding | `dedupe.diff` | 不同行同类问题均保留；相同 dedupe key 才合并 |
| 沙箱执行失败 | `sandbox-fail.diff`、`sandbox-timeout.diff` | failed / timed_out 不导致任务崩溃 |
| 敏感信息脱敏 | `secret-shapes.diff` | 多形态 secret 脱敏，placeholder 不误报 |
| 额外错误处理样本 | `panic.diff` | `panic-direct` high finding |
| 真实 PR 组合样本 | `realistic-service-risk.diff` | 多文件 patch 中同时覆盖多类高风险和 warning |

## 断言模式

能力评测入口是 `scripts/eval.sh`。`cmd/review-agent/fixtures_test.go` 保留冗余 Go harness，仅在设置 `CR_AGENT_RUN_FIXTURE_MATRIX_TEST=1` 时运行，避免 integration 与 eval 重复执行完整矩阵。

当前每个 fixture 都通过 CLI/Agent 链路生成 `review_report.json`，再断言：

- findings 数量。
- warnings 数量。
- `rule_id`。
- `severity`。
- `status`。
- secret evidence 中不包含原始密钥。
- `safe.diff` 零 finding。
- `dedupe.diff` 保留两个不同行 finding，并由通用 dedupe 测试覆盖相同 key 合并。
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

### expanded Go review fixtures

`http-body.diff`、`sql-string-concat.diff`、`command-injection.diff`、`context-background.diff`、`mutex-unlock.diff`、`defer-in-loop.diff`、`bare-return-err.diff` 和 `string-concat-loop.diff` 分别锁定扩展 Go CR 规则。`string-concat-loop` 是低置信性能信号，预期进入 `warnings` / `needs_human_review`，不进入高置信 findings。

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

## Hidden-like / External 样本与评测

Issue 要求隐藏/holdout 样本检出率 ≥ 80%、误报 ≤ 15%。本仓库用 `testdata/holdout/` 作为 self-contained holdout/adversarial matrix，并用 `scripts/hidden_matrix_smoke.sh` 证明 external matrix 注入机制。需要扩展更多本地样本时，`scripts/eval.sh` 支持外部 fixture root、external expected TSV、阈值门禁和报告保留目录：

```bash
CR_AGENT_EVAL_FIXTURES_ROOT=/path/to/local-fixtures \
CR_AGENT_EVAL_FIXTURES="case-001.diff case-002.diff" \
CR_AGENT_EVAL_MATRIX=/path/to/expected.tsv \
CR_AGENT_EVAL_REPORT_ROOT=/tmp/cr-agent-eval-reports \
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

- [issue-2004-traceability.md](issue-2004-traceability.md)
- [data-contract.md](data-contract.md)
