# 方案设计说明

> Issue #2004 交付物：300–500 字方案设计说明。

## Skill 设计

code-review Skill 位于 `skills/code-review/`，包含 `SKILL.md`（入口与使用约束）、`rules.md`（规则文档，与 engine `rule_id` 一一对应）和 `scripts/`（沙箱可执行检查脚本）。第一版必须通过 `trpc-agent-go` 的 `tool/skill` 执行 `skill load` / `skill run`，读取规则策略后由确定性规则引擎对 diff 新增行做启发式扫描；必要时经 Permission 批准后，Skill 脚本触发 `go test`、`go vet` 或 staticcheck。Skill 层负责「审查策略与可执行脚本」，规则引擎负责「diff 确定性检出」，两者结果合并去重后输出。

## 沙箱隔离策略

生产与 CI 默认使用 `codeexecutor/container` 或 E2B runtime，本地 `exec.Command` 仅通过 `CR_SANDBOX_RUNTIME=local` 显式启用。沙箱执行前必须经过 `tool.PermissionPolicy` 或兼容 wrapper 决策：`deny`、`ask`、`needs_human_review` 的命令不进入 executor。执行控制包括超时（默认 30s）、stdout/stderr 输出大小上限、环境变量白名单、artifact 数量上限。stdout/stderr 以 digest 形式落库，明文密钥在写入前脱敏。沙箱超时或命令失败记录为 `status=timeout/failed`，不导致整个 review 任务崩溃。

## Permission / Filter 策略

Permission 层拦截高风险 shell 命令（如 `rm -rf`、`sudo`、`mkfs`），允许 `go test`、`go vet`、`staticcheck` 等审查相关命令。Filter 层负责内容过滤：含明文 API Key、token、password 的 evidence 或 stdout 在进入报告和数据库前替换为 `[REDACTED]`。所有 Permission 决策（含 deny）写入 `permission_decisions` 表供审计。

## 监控字段

每次 review 记录 `total_duration_ms`、`sandbox_duration_ms`、`tool_call_count`、`permission_block_count`、`finding_count`、`severity_counts`（JSON）、`exception_counts`（超时/失败/deny 分类计数）、`redaction_count`。Metrics 与 task 关联，支持按 `task_id` 查询和后续评测回放。

## 数据库 Schema

最小 schema 包含：`review_tasks`（任务锚点）、`findings`（结构化发现项，含 dedupe_key）、`permission_decisions`（治理审计）、`sandbox_runs`（执行记录，含 runtime/status/digest）、`artifacts`（沙箱产物）、`metrics`（监控摘要）、`reports`（JSON + Markdown blob）。存储通过 `storage.Store` interface 抽象，默认 SQLite 实现，保留切换其他 SQL 后端的空间。

## 去重降噪与安全边界

去重键为 `file + line + category + rule_id` 的 SHA1，同一位置同一规则只保留首条。低置信度问题（如 missing-test-hint）进入 warnings，不混入 findings；需人工判断的进入 `needs_human_review`。安全边界：禁止无限制 shell、沙箱必须先过 Permission、敏感字面量脱敏后才落库、输出与运行均有上限、沙箱失败不崩溃。

---

字数：约 480 字。

## 相关文档

- [architecture.md](architecture.md) — 完整架构与组件说明
- [data-contract.md](data-contract.md) — 实体字段定义
- [issue-2004-traceability.md](issue-2004-traceability.md) — Issue 需求追踪
