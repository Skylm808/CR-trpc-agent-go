# CR-trpc-agent-go

基于官方 [trpc-agent-go](https://github.com/trpc-group/trpc-agent-go) 的 Go 自动代码评审 Agent 原型。仓库不是框架 fork，而是框架之上的应用层示例：用 `trpc-agent-go/tool/skill` 加载并执行 `skills/code-review`，用 `tool.PermissionPolicy` 做执行前治理，用 `codeexecutor/container` 作为生产默认执行器，用 SQLite 保存任务、权限决策、沙箱运行、发现项、产物、指标和最终报告。

本项目的第一版目标是可验证链路，不依赖真实模型 API Key：fixture / diff / repo 输入可以在 `rule-only`、`dry-run`、`sandbox`、`fake-model` 模式下生成 `review_report.json`、`review_report.md`，并可按 task id 查询审计记录。

## Current Status

已实现：

- `internal/agent` 编排层，CLI 只调用 Agent，不直接绕过框架。
- `skill_load` 加载 `skills/code-review/SKILL.md`。
- `skill_run` 执行 `skills/code-review/scripts/check.sh`，脚本输出 JSON findings。
- `tool.PermissionPolicy` 决策，`deny` / `ask` / `needs_human_review` 不进入 executor。
- `codeexecutor/container` 为默认 runtime；`local-fallback` 只能显式用于开发和测试。
- `sandbox` 模式下通过官方 `tool/codeexec` 执行 `go test ./...`、`go vet ./...`，`--staticcheck` 显式开启 `staticcheck ./...`。
- SQLite 保存 task、permission decision、filter decision、sandbox run、finding、artifact、metrics、report。
- 报告包含 findings、warnings、human_review_items、severity counts、governance_summary、sandbox_summary、metrics、artifacts 和修复建议。
- 沙箱非零退出和 timeout 不会中断 review，会写入 failed / timed_out run 与 `exception_counts`。
- 敏感信息在报告和 DB 写入前脱敏。
- 公开 fixture 覆盖安全、panic、TODO、测试缺失、goroutine/context/resource/db lifecycle、去重、sandbox failure、sandbox timeout。

仍需完善：

- Docker `codeexecutor/container` 真实端到端验证。
- 官方 artifact service 接入；当前 artifact 为本地 SQLite 记录。
- `session/sqlite` 作为 Agent session/history 的直接使用。
- 更完整的 telemetry hook 和外部观测集成。

## Architecture

```text
CLI
  -> internal/agent
  -> trpc-agent-go/tool/skill skill_load
  -> tool.PermissionPolicy
  -> trpc-agent-go/tool/skill skill_run
  -> optional trpc-agent-go/tool/codeexec go checks
  -> report JSON/Markdown
  -> SQLite audit store
```

runtime 策略：

- 默认 `--runtime container`，通过 `codeexecutor/container` 创建隔离执行器。
- `--runtime local-fallback` 仅用于开发和测试。
- container 模式下 `--repo-path` 会 bind mount 到容器内 `/workspace/repo`，Go check 命令在容器路径执行。

## Quick Start

运行测试：

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
```

用 fixture 生成报告：

```bash
go run ./cmd/review-agent \
  --fixture secret.diff \
  --fixtures-root testdata/fixtures \
  --skills-root skills \
  --runtime local-fallback \
  --mode rule-only \
  --output-dir /tmp/review-out
```

从 diff 文件生成报告：

```bash
go run ./cmd/review-agent \
  --diff-file testdata/fixtures/panic.diff \
  --skills-root skills \
  --runtime local-fallback \
  --mode fake-model \
  --sqlite /tmp/review.db \
  --output-dir /tmp/review-out
```

对本地 Go repo 运行 sandbox checks：

```bash
go run ./cmd/review-agent \
  --repo-path /path/to/go/repo \
  --skills-root skills \
  --runtime container \
  --mode sandbox \
  --staticcheck \
  --sqlite /tmp/review.db \
  --output-dir /tmp/review-out
```

示例输出见：

- [examples/review_report.json](examples/review_report.json)
- [examples/review_report.md](examples/review_report.md)

## CLI

| Flag | Default | Description |
|------|---------|-------------|
| `--diff-file` | empty | Unified diff input. |
| `--repo-path` | empty | Git repo or plain directory input. |
| `--fixture` | empty | Fixture file name under `--fixtures-root`. |
| `--fixtures-root` | `testdata/fixtures` | Fixture directory. |
| `--skills-root` | `skills` | Skill repository root. |
| `--runtime` | `container` | `container` or `local-fallback`. |
| `--mode` | `rule-only` | `rule-only`, `dry-run`, `sandbox`, `fake-model`. |
| `--staticcheck` | `false` | Run optional `staticcheck ./...` in sandbox mode. |
| `--sqlite` | empty | SQLite DB path. |
| `--output-dir` | `.` | Report output directory. |

## Modes

| Mode | Behavior |
|------|----------|
| `rule-only` | Loads the skill and runs deterministic `scripts/check.sh`. |
| `dry-run` | Loads the skill, records a `dry_run` permission decision and a skipped sandbox run, but does not execute. |
| `sandbox` | Runs `skill_run`, then permission-gated `go test ./...`, `go vet ./...`, and optional `staticcheck ./...`. |
| `fake-model` | Same deterministic skill path as `rule-only`; no model API Key required. |

## SQLite Audit Data

SQLite tables are created automatically:

- `review_tasks`
- `findings`
- `permission_decisions`
- `filter_decisions`
- `sandbox_runs`
- `artifacts`
- `metrics`
- `reports`

Example query:

```sql
SELECT task_id, status, mode FROM review_tasks ORDER BY created_at DESC;
SELECT command, action, reason FROM permission_decisions WHERE task_id = ?;
SELECT command, status, timeout_ms, output_limit_bytes, duration_ms FROM sandbox_runs WHERE task_id = ?;
```

## Fixtures

`testdata/fixtures/` includes more than 8 runnable samples:

| Fixture | Expected focus |
|---------|----------------|
| `safe.diff` | No finding. |
| `secret.diff` | `secret-leak` critical finding with redacted evidence. |
| `panic.diff` | `panic-direct` high finding. |
| `todo.diff` | `todo-marker` medium finding. |
| `test-missing.diff` | `missing-test-hint` warning. |
| `goroutine.diff` | `goroutine-leak` high finding. |
| `context.diff` | `context-leak` high finding. |
| `resource.diff` | `resource-leak` high finding. |
| `db-lifecycle.diff` | `db-lifecycle` high finding. |
| `dedupe.diff` | Duplicate finding noise reduction. |
| `sandbox-fail.diff` | Nonzero sandbox exit is recorded and non-fatal. |
| `sandbox-timeout.diff` | Timeout is recorded as `timed_out` and non-fatal. |

## Project Layout

```text
cmd/review-agent/       CLI adapter
internal/agent/         framework-first orchestration
internal/report/        JSON and Markdown rendering
internal/review/        shared finding/diff types and legacy parser helpers
internal/storage/sqlite SQLite audit store
skills/code-review/     SKILL.md, rule docs, scripts/check.sh
testdata/fixtures/      runnable diff fixtures
docs/                   architecture, data contract, traceability docs
examples/               sample review reports
```

## Documentation

- [docs/framework-first-mvp.md](docs/framework-first-mvp.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/implementation-plan.md](docs/implementation-plan.md)
- [docs/data-contract.md](docs/data-contract.md)
- [docs/issue-2004-traceability.md](docs/issue-2004-traceability.md)
- [docs/fixtures-matrix.md](docs/fixtures-matrix.md)
- [docs/design-summary.md](docs/design-summary.md)

## License

See LICENSE if present in the repository root.
