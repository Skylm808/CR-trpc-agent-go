# 数据契约

本文档定义 CR Agent 的核心实体。当前实现以 SQLite 为默认审计后端，并在 `internal/storage/store.go` 保留 `Store` interface；它不是官方 Session Service。后续接 Runner/Event 或多轮评审时，可以迁移到独立 SQL 后端，或把审计任务映射到官方 `session/sqlite`。

## ReviewTask

表示一次审查任务。

| 字段 | 类型 | 当前状态 |
|------|------|----------|
| `task_id` | string | ✅ `task-<diff_digest>-<unix_nano>` |
| `input_type` | string | ✅ 当前为 `diff` |
| `input_ref` | string | ✅ diff path / fixture path / repo path |
| `input_digest` | string | ✅ SHA-256 |
| `repo_path` | string | ✅ |
| `status` | string | ✅ `running` / `done` / `failed` |
| `mode` | string | ✅ `rule-only` / `dry-run` / `sandbox` / `fake-model` |
| `created_at` | time | ✅ |
| `started_at` | time | ✅ |
| `finished_at` | time | ✅ |

## ReviewInput

归一化输入目前不单独落库。

| 字段 | 当前状态 |
|------|----------|
| `diff_text` | ✅ |
| `fixture` | ✅ |
| `workspace_path` | ✅ `--repo-path` |
| `file_paths` | ⬜ |
| `base_ref` / `head_ref` | ⬜ |
| `parsed_files` / `parsed_hunks` | ✅ 由 parser 和 Skill 脚本处理 |

## PermissionDecision

命令执行前的治理决策，落库到 `permission_decisions`。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `command` | ✅ |
| `action` | ✅ `allow` / `ask` / `deny` / `dry_run` |
| `reason` | ✅ |
| `created_at` | ✅ |
| `policy_name` | ⬜ 可后续补充 |

**约束：** 非 `allow` 的命令不得进入 executor。

## FilterDecision

内容过滤或脱敏决策，落库到 `filter_decisions`。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `target` | ✅ 例如 `finding.evidence` |
| `action` | ✅ 当前主要为 `redact` |
| `reason` | ✅ |
| `created_at` | ✅ |

当前只在出现 redaction 时记录。后续可扩展为输入过滤、stdout/stderr 过滤、artifact 过滤。

## SandboxRun

每次 Skill、workspaceexec 或 codeexec fallback 执行记录，落库到 `sandbox_runs`。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `command` | ✅ |
| `runtime` | ✅ `container` / `local-fallback` |
| `status` | ✅ `ok` / `failed` / `error` / `timed_out` / `skipped` / permission action |
| `timeout_ms` | ✅ |
| `output_limit_bytes` | ✅ |
| `env_whitelist` | ✅ 当前统一记录 `PATH,HOME,TMPDIR,GOCACHE` |
| `exit_code` | ✅ |
| `stdout_digest` | ✅ |
| `stderr_digest` | ✅ |
| `duration_ms` | ✅ |
| `output` | 🔶 保留兼容字段，正常路径应优先 digest |
| `created_at` | ✅ |
| `finished_at` | ✅ |
| `artifact_count` | ✅ |

## Finding

结构化发现项。

| 字段 | 当前状态 |
|------|----------|
| `severity` | ✅ |
| `category` | ✅ |
| `file` | ✅ |
| `line` | ✅ |
| `title` | ✅ |
| `evidence` | ✅ 写入前脱敏 |
| `recommendation` | ✅ |
| `confidence` | ✅ |
| `source` | ✅ `skill_run` / `rule` / `sandbox` / `mode` / `permission` |
| `rule_id` | ✅ |
| `dedupe_key` | ✅ `file + line + category + rule_id` |
| `status` | ✅ `finding` / `warning` / `needs_human_review` |

当前 SQLite 会把 `result.Findings`、`result.Warnings` 和 `result.HumanReviewItems` 都写入 `findings` 表，并通过 `status` 区分 `finding`、`warning`、`needs_human_review`。这样报告和数据库回放使用同一份结构化 review item 数据。

## Artifact

当前 artifact 包含本地报告和诊断产物引用，落库到 `artifacts`；配置 `ArtifactService` 时同步写入官方 artifact service。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `name` | ✅ `review_report.json` / `review_report.md` / `review_diagnostics.json` |
| `kind` | ✅ `report` / `diagnostic` |
| `path` | ✅ |
| `digest` | ✅ |
| `size_bytes` | ✅ |
| `created_at` | ✅ |

当前 Agent 默认限制单个本地产物最大 1MiB，可通过 Config 调整。后续可增加更细粒度 artifact filter。

## MetricsSummary

审查监控摘要，落库到 `metrics`。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `total_duration_ms` | ✅ |
| `sandbox_duration_ms` | ✅ |
| `tool_call_count` | ✅ |
| `permission_block_count` | ✅ 统计所有非 allow / 非 dry-run 决策 |
| `finding_count` | ✅ |
| `severity_counts_json` | ✅ |
| `exception_counts_json` | ✅ |
| `redaction_count` | ✅ |
| `created_at` | ✅ |

当前 metrics 是本地聚合，官方 telemetry trace span 同步记录 mode、输入类型、task、finding、artifact、权限拦截、工具调用、沙箱和异常摘要。后续可把外部 telemetry hook 输出映射到同一 schema。

## ReviewReport

`review_report.json` 与 `review_report.md` 都由 `internal/report` 生成。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `summary` | ✅ |
| `findings` | ✅ |
| `warnings` | ✅ |
| `human_review_items` | ✅ |
| `governance_summary` | ✅ |
| `sandbox_summary` | ✅ |
| `metrics` | ✅ |
| `artifacts` | ✅ |
| `conclusion` | ✅ |

## 存储规则

1. 每个 task 对应唯一 `review_tasks` 行；报告或 artifact 阶段失败会标记为 `failed`。
2. decisions、runs、findings、artifacts、metrics、reports 均通过 `task_id` 关联。
3. Permission 非 allow 决策必须落库，即使命令未执行。
4. sandbox 失败或超时必须落库，不能导致 review 整体失败。
5. evidence、report、stdout/stderr 明文进入存储前必须脱敏或摘要化。
6. 按 `task_id` 必须能查询 task、findings/warnings、report、decisions、filter decisions、sandbox runs、artifacts、metrics。

## Store Interface

当前 `Store` interface 位于 `internal/storage/store.go`。Agent 依赖该接口，SQLite 只是默认实现：

```go
type Store interface {
    SaveTask(ctx context.Context, task Task) error
    TaskByID(ctx context.Context, id string) (Task, error)

    SaveFinding(ctx context.Context, taskID string, f Finding) error
    FindingsByTaskID(ctx context.Context, taskID string) ([]Finding, error)

    SaveReport(ctx context.Context, taskID string, json, md []byte) error
    ReportByTaskID(ctx context.Context, taskID string) (Report, error)

    SaveDecision(ctx context.Context, rec PermissionDecision) error
    DecisionsByTaskID(ctx context.Context, taskID string) ([]PermissionDecision, error)

    SaveFilterDecision(ctx context.Context, rec FilterDecision) error
    FilterDecisionsByTaskID(ctx context.Context, taskID string) ([]FilterDecision, error)

    SaveSandboxRun(ctx context.Context, rec SandboxRun) error
    SandboxRunsByTaskID(ctx context.Context, taskID string) ([]SandboxRun, error)

    SaveArtifact(ctx context.Context, rec Artifact) error
    ArtifactsByTaskID(ctx context.Context, taskID string) ([]Artifact, error)

    SaveMetrics(ctx context.Context, rec MetricsSummary) error
    MetricsByTaskID(ctx context.Context, taskID string) (MetricsSummary, error)
}
```

## 相关文档

- [architecture.md](architecture.md)
- [implementation-plan.md](implementation-plan.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
