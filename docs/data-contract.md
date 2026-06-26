# 数据契约

本文档定义审查流程中的核心实体。**Target Contract** 为 Issue #2004 验收目标；**Current v0** 标注当前代码/SQLite 实现状态。

**框架优先约束：** Target Contract 中的 `PermissionDecision`、`FilterDecision`、`SandboxRun`、`Artifact` 和 `MetricsSummary` 必须记录 `trpc-agent-go` 的实际 Skill / Permission / CodeExecutor / telemetry 链路事件。本地 runner 或自研 policy 产生的数据只能作为 dev/test fallback，不能替代最终框架事件审计。

---

## ReviewTask

表示一次审查执行。

| 字段 | 类型 | Target | Current v0 |
|------|------|--------|-----------|
| `task_id` | string | ✅ | ✅ UUID 待改（当前硬编码 `task-1`） |
| `input_type` | string | ✅ | ✅ `diff` / `repo` |
| `input_ref` | string | ✅ | ✅ diff 文件或 repo 路径 |
| `input_digest` | string | ✅ | ✅ SHA 待改（当前用 timestamp） |
| `repo_path` | string | ✅ | ✅ |
| `status` | string | ✅ | ✅ `done` / `running` / `failed` |
| `mode` | string | ✅ | ✅ `rule-only` 等 |
| `created_at` | time | ✅ | ✅ |
| `started_at` | time | ✅ | 🔶 未填充 |
| `finished_at` | time | ✅ | 🔶 未填充 |

---

## ReviewInput

归一化审查输入（内存结构，不一定单独落库）。

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `source_type` | ✅ | 🔶 隐含在 input_type |
| `diff_text` | ✅ | ✅ |
| `file_paths` | ✅ | ⬜ 未支持 |
| `workspace_path` | ✅ | ✅ repo_path |
| `base_ref` / `head_ref` | ✅ | ⬜ 未支持 |
| `parsed_files` | ✅ | ✅ ParsedDiff.Files |
| `parsed_hunks` | ✅ | ✅ 嵌套在 ParsedFile.Hunks |

---

## ParsedFile

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `path` | ✅ | ✅ |
| `language` | ✅ | ✅ 由扩展名推断 |
| `package_name` | ✅ | ✅ PackageFromPath |
| `is_test_file` | ✅ | ✅ `_test.go` 后缀 |
| `change_type` | ✅ | 🔶 未填充 |

---

## ParsedHunk

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `file` | ✅ | ✅ |
| `old_start` / `old_lines` | ✅ | ✅ |
| `new_start` / `new_lines` | ✅ | ✅ |
| `context` | ✅ | ✅ |
| `candidate_lines` | ✅ | ✅ |
| `lines`（Line 明细） | — | ✅ 扩展字段 |

---

## PermissionDecision

命令执行前的 Permission 层决策。

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `decision_id` | ✅ | 🔶 SQLite AUTOINCREMENT id |
| `task_id` | ✅ | ✅ |
| `command` | ✅ | ✅ |
| `policy_name` | ✅ | ⬜ 未记录 |
| `decision` | ✅ | ✅ 存为 `action` 列 |
| `reason` | ✅ | ✅ |
| `created_at` | ✅ | ✅ |

**decision 枚举：** `allow` | `deny` | `ask` | `needs_human_review`

---

## FilterDecision

内容/输入/输出 Filter 层决策（Target；Current v0 未实现）。

| 字段 | 说明 |
|------|------|
| `decision_id` | 唯一 ID |
| `task_id` | 关联任务 |
| `filter_name` | 如 `secret-content`、`output-size` |
| `target` | 被过滤对象（如 finding evidence、stdout） |
| `decision` | `allow` / `block` / `redact` |
| `reason` | 拦截原因 |
| `created_at` | 时间戳 |

---

## SandboxRun

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `run_id` | ✅ | 🔶 AUTOINCREMENT id |
| `task_id` | ✅ | ✅ |
| `runtime` | ✅ | ⬜ 未记录（local/container/e2b） |
| `command` | ✅ | ✅ |
| `args` | ✅ | ⬜ 未记录 |
| `timeout_ms` | ✅ | ⬜ 未记录 |
| `output_limit_bytes` | ✅ | ⬜ 未记录 |
| `env_whitelist` | ✅ | ⬜ 未记录 |
| `status` | ✅ | ✅ `ok` / `timeout` / `failed` / `denied` |
| `exit_code` | ✅ | ⬜ 未记录 |
| `stdout_digest` | ✅ | ⬜ 存明文 output |
| `stderr_digest` | ✅ | ⬜ 未记录 |
| `artifact_count` | ✅ | ⬜ 未记录 |
| `duration_ms` | ✅ | ⬜ 未记录 |
| `created_at` / `finished_at` | ✅ | 🔶 仅 created_at |

---

## Finding

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `finding_id` | ✅ | 🔶 使用 dedupe_key 代替 |
| `task_id` | ✅ | ✅ 落库时有，内存结构无 |
| `severity` | ✅ | ✅ |
| `category` | ✅ | ✅ |
| `file` | ✅ | ✅ |
| `line` | ✅ | ✅ |
| `title` | ✅ | ✅ |
| `evidence` | ✅ | ✅ 写入前脱敏 |
| `recommendation` | ✅ | ✅ |
| `confidence` | ✅ | ✅ |
| `source` | ✅ | ✅ `rule` / `sandbox` / `skill` |
| `rule_id` | ✅ | ✅ |
| `dedupe_key` | ✅ | ✅ DedupeKey() |
| `status` | ✅ | ✅ `finding` / `warning` / `needs_human_review` |

**severity 枚举：** `info` | `low` | `medium` | `high` | `critical`

**status 分流规则：**

- `confidence=high` + 明确 pattern → `finding`
- `confidence=medium/low` → `warning`
- 需人工判断 → `needs_human_review`（不混入 findings）

---

## Artifact

沙箱或 Skill 执行产生的产物（Target；Current v0 未实现）。

| 字段 | 说明 |
|------|------|
| `artifact_id` | 唯一 ID |
| `task_id` | 关联任务 |
| `kind` | 如 `stdout`、`report`、`script-output` |
| `path` | 文件路径或虚拟路径 |
| `digest` | SHA256 |
| `size_bytes` | 大小 |
| `created_at` | 时间戳 |

---

## MetricsSummary

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `task_id` | ✅ | ✅ |
| `total_duration_ms` | ✅ | ✅ |
| `sandbox_duration_ms` | ✅ | 🔶 沙箱未真正计时 |
| `tool_call_count` | ✅ | 🔶 部分填充 |
| `permission_block_count` | ✅ | 🔶 部分填充 |
| `finding_count` | ✅ | ✅ |
| `severity_counts` | ✅ | ✅ JSON |
| `exception_counts` | ✅ | ⬜ 未填充 |
| `redaction_count` | ✅ | ⬜ 未计数 |

---

## ReviewReport

| 字段 | Target | Current v0 |
|------|--------|-----------|
| `task_id` | ✅ | ⬜ JSON 未含 |
| `summary` | ✅ | ✅ |
| `findings` | ✅ | ✅ |
| `warnings` | ✅ | 🔶 JSON 有，Markdown 段待补 |
| `human_review_items` | ✅ | ⬜ |
| `governance_summary` | ✅ | ⬜ |
| `sandbox_summary` | ✅ | ⬜ |
| `metrics` | ✅ | 🔶 基础 |
| `artifacts` | ✅ | ⬜ |
| `conclusion` | ✅ | ⬜ |

---

## 持久化规则

1. 每个 task 对应唯一 `review_tasks` 行。
2. decisions、runs、findings、artifacts、metrics 均通过 `task_id` 外键关联。
3. findings 必须可按 `task_id` 查询。
4. report 必须可从存储行重建（JSON + Markdown blob 或从 findings 聚合）。
5. 敏感字面量在写入 findings evidence、report 文本、stdout/stderr 存储前必须脱敏。
6. `deny` / `ask` / `needs_human_review` 的 Permission 决策必须落库，即使命令未执行。

---

## Storage Interface（Target）

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

    SaveSandboxRun(ctx context.Context, rec SandboxRun) error
    SandboxRunsByTaskID(ctx context.Context, taskID string) ([]SandboxRun, error)

    SaveArtifact(ctx context.Context, rec Artifact) error
    ArtifactsByTaskID(ctx context.Context, taskID string) ([]Artifact, error)

    SaveMetrics(ctx context.Context, rec MetricsSummary) error
    MetricsByTaskID(ctx context.Context, taskID string) (MetricsSummary, error)
}
```

Current v0：`internal/storage/sqlite/` 直接实现，尚未抽象 interface。

---

## 相关文档

- [architecture.md](architecture.md)
- [implementation-plan.md](implementation-plan.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
