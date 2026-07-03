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
| `file_paths` | ✅ `--file-list` 输入会转换为新增文件 diff |
| `base_ref` / `head_ref` | ⬜ |
| `parsed_files` / `parsed_hunks` | ✅ 由 parser 和 Skill 脚本处理 |

## ModelReviewInput

`fake-model` 模式会构造脱敏后的模型审查输入，但当前不单独落库，也不发送到真实模型 API。

| 字段 | 当前状态 |
|------|----------|
| `diff_summary` | ✅ 由 unified diff 脱敏后生成，raw secret 不进入 provider |
| `input_metadata` | ✅ 复用 `InputMetadata` |
| `existing_findings` | ✅ 复用并脱敏后的 `Finding` |
| `sandbox_summary` | ✅ 复用 `SandboxSummary` |
| `governance_summary` | ✅ 复用 `GovernanceSummary` |

当前 fake provider 是 deterministic provider；真实 OpenAI / Claude / Gemini provider、API Key 和 SDK 绑定留到后续阶段。

## InputMetadata

输入元数据进入 `review_report.json` 和 `review_diagnostics.json` 的 `input_metadata` 字段，用于评测和回放时识别 Go 工程范围。

| 字段 | 当前状态 |
|------|----------|
| `changed_go_files` | ✅ 本次输入触达的 `.go` 文件 |
| `package_names` | ✅ 从 diff 中的 `package` 行提取 |
| `module_path` | ✅ `--repo-path` 有 `go.mod` 时提取 |
| `has_tests` | ✅ 本次输入是否触达测试文件 |
| `touched_test_files` | ✅ 本次输入触达的 `_test.go` 文件 |

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
| `source` | ✅ `skill_run` / `rule` / `sandbox` / `mode` / `permission` / `model` / `fake_model` |
| `rule_id` | ✅ |
| `dedupe_key` | ✅ `file + line + category + rule_id` |
| `status` | ✅ `finding` / `warning` / `needs_human_review` |

当前 SQLite 会把 `result.Findings`、`result.Warnings` 和 `result.HumanReviewItems` 都写入 `findings` 表，并通过 `status` 区分 `finding`、`warning`、`needs_human_review`。这样报告和数据库回放使用同一份结构化 review item 数据。

模型输出也复用同一结构。高置信模型项进入 `findings`；低置信模型项进入 `warnings` 并标记 `needs_human_review`；模型项和规则项按 `file + line + category + rule_id` 去重。模型 evidence 在进入报告和 SQLite 前必须走同一套脱敏逻辑。

## Artifact

当前 artifact 包含本地报告和诊断产物引用。Agent 默认使用官方 `artifact/inmemory` service 保存报告和诊断正文；业务侧也可以注入 COS/S3 等其他 `artifact.Service`。SQLite `artifacts` 表只保存引用、摘要和大小，作为审计索引。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `name` | ✅ `review_report.json` / `review_report.md` / `review_diagnostics.json` |
| `kind` | ✅ `report` / `diagnostic` |
| `path` | ✅ 产物路径或 artifact key |
| `digest` | ✅ 产物 SHA-256 摘要 |
| `size_bytes` | ✅ 产物字节数 |
| `created_at` | ✅ |

当前 Agent 默认限制单个本地产物最大 1MiB，可通过 Config 调整。SQLite 不保存 artifact 正文；后续可增加更细粒度 artifact filter。artifact 安全边界见 [sandbox-safety.md](sandbox-safety.md)。

## MetricsSummary

审查监控摘要，落库到 `metrics`。

| 字段 | 当前状态 |
|------|----------|
| `task_id` | ✅ |
| `total_duration_ms` | ✅ |
| `sandbox_duration_ms` | ✅ |
| `model_duration_ms` | ✅ |
| `tool_call_count` | ✅ |
| `model_call_count` | ✅ |
| `permission_block_count` | ✅ 统计所有非 allow / 非 dry-run 决策 |
| `finding_count` | ✅ |
| `model_finding_count` | ✅ |
| `model_exception_count` | ✅ |
| `severity_counts_json` | ✅ |
| `exception_counts_json` | ✅ |
| `redaction_count` | ✅ |
| `created_at` | ✅ |

当前 metrics 是本地聚合，官方 telemetry trace span 同步记录 mode、runtime、输入类型、task、finding、artifact、权限拦截、工具调用、model 调用/耗时/异常/finding 数、总耗时、沙箱耗时、severity 分布、exception 类型分布和结论。trpc-agent-go 的公开 metric 包主要初始化框架内置 LLM/tool/workflow 指标；本 CLI 原型不硬接官方 internal 指标，后续部署时可启动官方 metric exporter 和 OTLP dashboard。

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
| `input_metadata` | ✅ |
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
- [sandbox-safety.md](sandbox-safety.md)
