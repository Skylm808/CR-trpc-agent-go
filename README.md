# CR-trpc-agent-go

基于官方 [trpc-agent-go](https://github.com/trpc-group/trpc-agent-go) 的 Go 自动代码评审 Agent 原型。仓库不是框架 fork，而是框架之上的应用层示例：用 `trpc-agent-go/tool/skill` 加载并执行 `skills/code-review`，用 `tool.PermissionPolicy` 做执行前治理，用 `tool/workspaceexec` 执行工作区级 Go 检查，用 `tool/codeexec` 做兜底，用 `codeexecutor/container` 做默认沙箱，用 `artifact` 保存报告和诊断产物，用 telemetry 记录审查摘要，用 SQLite 保存任务、权限决策、沙箱运行、发现项、产物引用、指标和最终报告。

当前是基于 trpc-agent-go Tool/Skill/CodeExecutor/workspaceexec/artifact/telemetry/Runner 的 CLI Agent 原型，并已接入 LLM Review Provider 边界、deterministic fake provider、显式 opt-in 的 generic HTTP provider，以及基于官方 `trpc-agent-go/model/openai` 的 OpenAI-compatible / DeepSeek provider。默认不启用真实网络模型，不绑定 OpenAI、Claude、Gemini 等真实厂商 SDK，不需要 API Key；当前 provider 已通过官方 `trpc-agent-go/model.Model` 接口，CLI 兼容入口通过官方 `runner.NewRunner(...).Run(...)` 消费 `event.Event` 流。Session/Memory 更适合跨 PR 经验复用；E2B 当前是显式 unsupported 入口，后续可替换为真实远端 runtime adapter。

本项目的第一版目标是可验证链路，不依赖真实模型 API Key：fixture / diff / repo 输入可以在 `rule-only`、`dry-run`、`sandbox`、`fake-model` 模式下生成 `review_report.json`、`review_report.md`，并可按 task id 查询审计记录。CLI 在没有传 `--diff-file`、`--file-list`、`--repo-path` 或 `--fixture` 时，会把当前工作目录推断为 `--repo-path .`，方便在待审仓库内少参数运行 review。

## 第一版 MVP 范围

- 基于官方 `trpc-agent-go` 的 `skill_load` / `skill_run` / `tool/workspaceexec` / `tool/codeexec` / `PermissionPolicy`。
- 默认 container runtime，local fallback 仅显式用于开发与测试。
- 结构化 findings、warnings、human review items、治理摘要、沙箱摘要、metrics、artifacts。
- SQLite 审计库可按 task id 查询 task、decision、run、finding、artifact、report。
- 公开 fixture、sample 输出、SQLite 回放测试和 `scripts/eval.sh` 已能跑通，适合作为第一版验收基线。

## Current Status

已实现：

- `internal/agent` 编排层，CLI 只调用 Agent，不直接绕过框架。
- 无输入参数时 CLI 默认审查当前目录，完整输入 flags 仍保留用于验收和调试。
- 自动读取当前目录 `cr-agent.yaml`，也可用 `--config` 指定配置文件；CLI flags 覆盖 YAML。
- `skill_load` 加载 `skills/code-review/SKILL.md`。
- `skill_run` 执行 `skills/code-review/scripts/check.sh`，脚本输出 JSON findings。
- `fake-model` 模式会在 deterministic Skill 后进入 `ModelReviewProvider` 边界，默认使用无网络、无 API Key 的 fake provider；显式传 `--model-provider http|openai|deepseek` 时才调用外部 provider。
- `--file-list` 路径列表输入会转换为新增文件 diff，复用同一审查链路。
- `tool.PermissionPolicy` 决策，`deny` / `ask` / `needs_human_review` 不进入 executor。
- `codeexecutor/container` 为默认 runtime；`local-fallback` 只能显式用于开发和测试。
- `sandbox` 模式下优先通过官方 `tool/workspaceexec` 在工作区内执行 `go test ./...`、`go vet ./...`，`--staticcheck` 显式开启 `staticcheck ./...`；失败时保留 `tool/codeexec` 兜底。
- SQLite 保存 task、permission decision、filter decision、sandbox run、finding、artifact 引用、metrics、report。
- `review_report.json`、`review_report.md`、`review_diagnostics.json` 会写入本地输出目录，并同步进入官方 artifact service；SQLite 只保存引用、摘要和大小。
- 报告包含 findings、warnings、human_review_items、severity counts、governance_summary、sandbox_summary、metrics、artifacts 和修复建议。
- metrics / diagnostics / telemetry / SQLite 记录 model 是否运行、model finding 数、model duration 和 model exception 数。
- `review_diagnostics.json` 包含 Go input metadata：changed_go_files、package_names、module_path、has_tests、touched_test_files。
- `--base-ref` / `--head-ref` 会进入 input metadata、报告、diagnostics、SQLite report blob 和 telemetry；`--repo-path` 同时传 refs 时使用 `git diff base...head`，不自动 fetch。
- `--runtime e2b` 是最小 unsupported/adapter 入口：不会静默 fallback 到 local/container，会在报告、diagnostics 和 SQLite sandbox run 中记录 `runtime=e2b status=unsupported`。
- `--model-provider deepseek` 走官方 `trpc-agent-go/model/openai` 的 `VariantDeepSeek` 路线；API key 只从 env 读取，缺 key 时降级为人工复核，不中断 review。
- 沙箱非零退出和 timeout 不会中断 review，会写入 failed / timed_out run 与 `exception_counts`。
- 敏感信息在报告和 DB 写入前脱敏。
- 公开 fixture 覆盖安全、secret 多形态脱敏、panic、TODO、测试缺失、goroutine/context/resource/db lifecycle、去重、sandbox failure、sandbox timeout。
- 早期 `internal/governance` / `internal/sandbox` 本地包装已删除；主链路只使用官方 `tool.PermissionPolicy`、`tool/codeexec` 和 `codeexecutor/container` / `codeexecutor/local`。

仍需完善：

- 用真实 hidden fixture matrix 跑一次验收；当前仓库只提交外部注入契约，不提交 hidden 样本本体。
- 将 E2B unsupported placeholder 替换为真实 E2B/Cube adapter。
- Docker `codeexecutor/container` 真实端到端验证已在 Docker Desktop 上跑通；宿主 CI 中仍建议显式开启 env-gated 测试。
- 官方 artifact service 默认使用 inmemory 保存报告和诊断产物；SQLite 继续保留 artifact 引用记录。
- 官方 `session/sqlite` 尚未直接接入；当前 SQLite 是审计 store，后续接 Runner/Event 或多轮评审时再映射 session/history。
- 官方 telemetry trace span 已记录审查摘要属性；SQLite metrics 表保留可查询聚合指标。官方 metric exporter/OTLP dashboard 属于后续部署集成项。
- Codex / Claude Code skill 可以作为后续包装入口，但不是 Issue #2004 的主线交付物。

## Architecture

```text
CLI
  -> internal/agent
  -> trpc-agent-go/tool/skill skill_load
  -> tool.PermissionPolicy
  -> trpc-agent-go/tool/skill skill_run
  -> official runner.Run / event.Event stream
  -> optional official model.Model adapter over ModelReviewProvider fake-model/http boundary
  -> optional trpc-agent-go/tool/workspaceexec go checks
  -> official event.Event for input/skill/model/sandbox/report/task phases
  -> fallback trpc-agent-go/tool/codeexec go checks
  -> report JSON/Markdown
  -> SQLite audit store
```

runtime 策略：

- 默认 `--runtime container`，通过 `codeexecutor/container` 创建隔离执行器，当前默认容器镜像为 `golang:1.25-bookworm`。
- `--runtime local-fallback` 仅用于开发和测试。
- container 模式下 `--repo-path` 会 bind mount 到容器内 `/workspace/repo`，Go check 命令通过官方 `workspaceexec` 在容器 workspace 执行；Agent 会用容器内 Go 二进制绝对路径避免 PATH 被治理策略清理后失效。

## Quick Start

运行测试：

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
```

运行完整本地验收：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

`scripts/acceptance.sh` 会运行 `go test ./...`、`scripts/eval.sh`、`git diff --check`，并在 Docker daemon 可用时自动追加 container E2E。可通过 `CR_AGENT_ACCEPTANCE_DOCKER=skip|auto|always` 控制 Docker 步骤。

在本仓库内少参数运行一次 review：

```bash
go run ./cmd/review-agent
```

该命令等价于把当前目录当作 `--repo-path .`，使用默认 `--mode rule-only`、`--runtime container` 和 `--output-dir .`。没有 Docker daemon 或只想本地开发验证时，可显式切到 local fallback：

```bash
go run ./cmd/review-agent --runtime local-fallback --output-dir /tmp/review-out
```

推荐本地配置：

```yaml
# cr-agent.yaml
mode: fake-model
runtime: container
output_dir: .cr-agent/reports
sqlite: .cr-agent/review.db
skills_root: skills
staticcheck: false

model:
  provider: deepseek
  name: deepseek-chat
  api_key_env: DEEPSEEK_API_KEY
```

然后在仓库内运行：

```bash
export DEEPSEEK_API_KEY=replace-me
go run ./cmd/review-agent
```

没有 `cr-agent.yaml` 时保持内置默认行为；有配置文件时，显式 CLI flags 仍可覆盖 YAML，例如：

```bash
go run ./cmd/review-agent --config ./cr-agent.yaml --runtime local-fallback --output-dir /tmp/review-out
```

运行真实 Docker container 集成测试：

```bash
docker info
CR_AGENT_RUN_CONTAINER_TESTS=1 \
GOCACHE=/private/tmp/cr-agent-gocache \
go test ./internal/agent -run TestAgentRunContainerRuntimeExecutesGoChecks -count=1
```

该测试需要 Docker Desktop、OrbStack 或 Colima 等 Docker daemon 正常运行，并且测试进程能访问 Docker socket。

运行公开 fixture 评测：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
```

评测输出包含 `recall`、`precision`、`false_positive_rate`、`missing_findings` 和 `unexpected_findings`。可用 `CR_AGENT_EVAL_FIXTURES_ROOT` 指向外部/隐藏样本目录，用 `CR_AGENT_EVAL_FIXTURES` 选择样本子集，用 `CR_AGENT_EVAL_MATRIX` 指向外部 expected matrix；`CR_AGENT_EVAL_EXPECTED` 仍作为兼容别名保留。

默认验收阈值贴合 Issue：`CR_AGENT_EVAL_MIN_RECALL=0.800`、`CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE=0.150`。公开内置矩阵额外要求零漏检、零未声明 finding。

```bash
CR_AGENT_EVAL_FIXTURES_ROOT=/path/to/hidden-fixtures \
CR_AGENT_EVAL_FIXTURES="hidden-001.diff hidden-002.diff" \
CR_AGENT_EVAL_MATRIX=/path/to/expected.tsv \
CR_AGENT_EVAL_REPORT_ROOT=/tmp/cr-agent-hidden-reports \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/eval.sh
```

隐藏样本的推荐契约见 [docs/eval-matrix.md](docs/eval-matrix.md)。

CI 和 hidden sample 接入见 [docs/ci.md](docs/ci.md)。
沙箱安全边界矩阵见 [docs/sandbox-safety.md](docs/sandbox-safety.md)；后续迁移到官方 `trpc-agent-go/examples` 的准备说明见 [docs/upstream-example-migration.md](docs/upstream-example-migration.md)。

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
- [examples/review_diagnostics.json](examples/review_diagnostics.json)

重新生成示例输出：

```bash
GOCACHE=/private/tmp/cr-agent-gocache go run ./cmd/review-agent \
  --fixture secret-shapes.diff \
  --fixtures-root testdata/fixtures \
  --skills-root skills \
  --runtime local-fallback \
  --mode rule-only \
  --sqlite examples/review.db \
  --output-dir examples
```

`examples/review.db` 是本地回放用 SQLite 文件，受 `.gitignore` 的 `*.db` 规则忽略；文本交付物是三个 report 文件。

## CLI

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `cr-agent.yaml` if present | YAML config file. Missing default config is ignored; explicit missing path returns an error. |
| `--diff-file` | empty | Unified diff input. |
| `--file-list` | empty | Newline-delimited changed file list; relative paths resolve from `--repo-path` or the list file directory. |
| `--repo-path` | inferred `.` when no input flag is set | Git repo or plain directory input. |
| `--fixture` | empty | Fixture file name under `--fixtures-root`. |
| `--base-ref` | empty | Base git ref for `--repo-path` diff and review metadata. |
| `--head-ref` | empty | Head git ref for `--repo-path` diff and review metadata. |
| `--fixtures-root` | `testdata/fixtures` | Fixture directory. |
| `--skills-root` | `skills` | Skill repository root. |
| `--runtime` | `container` | `container`, `local-fallback`, or `e2b`; `e2b` currently records an explicit unsupported audit entry. |
| `--mode` | `rule-only` | `rule-only`, `dry-run`, `sandbox`, `fake-model`. |
| `--staticcheck` | `false` | Run optional `staticcheck ./...` in sandbox mode. |
| `--sqlite` | empty | SQLite DB path. |
| `--output-dir` | `.` | Report output directory. |
| `--model-provider` | empty | Optional model provider: `http`, `openai`, `openai-compatible`, or `deepseek`; default empty keeps fake provider/no network behavior. |
| `--model-endpoint` | empty | HTTP model provider endpoint, required when `--model-provider http` is enabled. |
| `--model-api-key-env` | provider default | Environment variable name containing the optional provider API key. The key value is never written to reports, diagnostics, SQLite or telemetry. |
| `--model-name` | provider default where available | Optional model name included in the model provider request. DeepSeek defaults to `deepseek-chat`. |
| `--model-base-url` | provider default | OpenAI-compatible base URL override. |
| `--model-variant` | inferred from provider | OpenAI-compatible variant, currently `openai` or `deepseek`. |

## Modes

| Mode | Behavior |
|------|----------|
| `rule-only` | Loads the skill and runs deterministic `scripts/check.sh`. |
| `dry-run` | Loads the skill, records a `dry_run` permission decision and a skipped sandbox run, but does not execute. |
| `sandbox` | Runs `skill_run`, then permission-gated `go test ./...`, `go vet ./...`, and optional `staticcheck ./...`. |
| `fake-model` | Runs deterministic Skill, then the LLM Review Provider boundary. Default uses the built-in fake provider; `--model-provider http|openai|deepseek` explicitly switches to an external provider. No model API Key is required unless an external provider is selected. |

## LLM Review Provider Boundary

`internal/agent.ModelReviewProvider` receives only redacted prompt inputs: diff summary, input metadata, existing findings, sandbox summary and governance summary. The provider is wrapped through a thin official `trpc-agent-go/model.Model` adapter before review results are merged back into the CR report. Provider output reuses the normal `review.Finding` shape. High confidence model findings enter `findings`; low confidence or uncertain model signals become `warnings` with `needs_human_review`. Model findings dedupe against rule findings by `file + line + category + rule_id`.

The default built-in provider is deterministic and only exists to exercise the boundary in `fake-model` mode. It does not call OpenAI, Claude, Gemini, DeepSeek, or any network API. The optional HTTP provider is a minimal generic adapter using Go's standard `net/http`; it is disabled unless `--model-provider http` is provided. The `openai` / `openai-compatible` / `deepseek` providers use official `trpc-agent-go/model/openai` and remain opt-in.

DeepSeek example:

```bash
DEEPSEEK_API_KEY=replace-me \
go run ./cmd/review-agent \
  --mode fake-model \
  --model-provider deepseek \
  --model-name deepseek-chat \
  --runtime local-fallback \
  --output-dir /tmp/review-out
```

OpenAI-compatible endpoint example:

```bash
MODEL_API_KEY=replace-me \
go run ./cmd/review-agent \
  --mode fake-model \
  --model-provider openai-compatible \
  --model-name your-model \
  --model-base-url https://model.example/v1 \
  --model-api-key-env MODEL_API_KEY \
  --runtime local-fallback \
  --output-dir /tmp/review-out
```

HTTP provider request and response shape:

```json
{
  "model": "optional-model-name",
  "input": {
    "diff_summary": "redacted unified diff",
    "input_metadata": {},
    "existing_findings": [],
    "sandbox_summary": {},
    "governance_summary": {}
  }
}
```

The endpoint must return:

```json
{
  "findings": [
    {
      "severity": "medium",
      "category": "logic",
      "file": "example.go",
      "line": 42,
      "title": "Semantic risk",
      "evidence": "redacted evidence",
      "recommendation": "Review the branch condition.",
      "confidence": "high",
      "source": "model",
      "rule_id": "model-review"
    }
  ]
}
```

Example explicit HTTP provider invocation:

```bash
CR_AGENT_MODEL_API_KEY=replace-me \
go run ./cmd/review-agent \
  --diff-file testdata/fixtures/panic.diff \
  --skills-root skills \
  --runtime local-fallback \
  --mode fake-model \
  --model-provider http \
  --model-endpoint https://model.example/review \
  --model-api-key-env CR_AGENT_MODEL_API_KEY \
  --model-name review-model \
  --output-dir /tmp/review-out
```

Provider input is redacted before the HTTP request; provider output evidence is redacted again before reports, diagnostics and SQLite writes. HTTP transport errors, non-2xx responses, invalid JSON and deadline failures do not abort the review. They increment model exception metrics, add a `model-provider-failed` human review item and keep report generation going. Deterministic rules and sandbox checks remain the base safety path.

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

Example task-id replay query:

```sql
SELECT task_id, status, mode FROM review_tasks ORDER BY created_at DESC;
SELECT command, action, reason FROM permission_decisions WHERE task_id = ?;
SELECT target, action, reason FROM filter_decisions WHERE task_id = ?;
SELECT command, status, timeout_ms, output_limit_bytes, duration_ms FROM sandbox_runs WHERE task_id = ?;
SELECT severity, category, file, line, title, status FROM findings WHERE task_id = ? ORDER BY file, line;
SELECT name, kind, path, digest, size_bytes FROM artifacts WHERE task_id = ?;
SELECT total_duration_ms, sandbox_duration_ms, model_duration_ms, tool_call_count, model_call_count, permission_block_count, finding_count, model_finding_count, model_exception_count, redaction_count FROM metrics WHERE task_id = ?;
SELECT json_report, markdown_report FROM reports WHERE task_id = ?;
```

`cmd/review-agent` 的 `TestAcceptanceEvidenceReportsAndSQLiteReplay` 会读取报告中的 `task_id`，并按上述数据类别查询 SQLite，保证报告和审计库可回放。

## Fixtures

`testdata/fixtures/` includes more than 8 runnable samples:

| Fixture | Expected focus |
|---------|----------------|
| `safe.diff` | No finding. |
| `secret.diff` | `secret-leak` critical finding with redacted evidence. |
| `secret-shapes.diff` | API key、LLM key、Bearer、GitHub token、password 多形态脱敏和 placeholder 降噪。 |
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
scripts/eval.sh         public fixture recall / precision evaluator
```

## Documentation

- [docs/README.md](docs/README.md)
- [docs/architecture.md](docs/architecture.md)
- [docs/implementation-plan.md](docs/implementation-plan.md)
- [docs/data-contract.md](docs/data-contract.md)
- [docs/issue-2004-traceability.md](docs/issue-2004-traceability.md)
- [docs/sandbox-safety.md](docs/sandbox-safety.md)
- [docs/ci.md](docs/ci.md)
- [docs/eval-matrix.md](docs/eval-matrix.md)
- [docs/fixtures-matrix.md](docs/fixtures-matrix.md)

## License

See LICENSE if present in the repository root.
