# Issue #2004 需求追踪矩阵

本文档将 Issue #2004 的能力要求、输入输出要求、交付物、验收命令和剩余缺口映射到当前仓库实现。当前实现是基于 `trpc-agent-go` Tool / Skill / CodeExecutor / PermissionPolicy / artifact / telemetry / Runner / SQLite 的 CLI Agent 原型；已接入 LLM Review Provider 边界、无 API Key fake provider、显式 opt-in generic HTTP provider、官方 `model.Model` 薄 adapter、官方 `runner.Run` / `event.Event` 主入口、E2B unsupported runtime 入口和 base/head ref 输入。尚未接入官方 Session/Memory、真实 E2B runtime adapter 和真实 hidden fixture matrix 验收记录。

## 验收命令

本地基础验收入口：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

该脚本默认执行：

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
git diff --check
```

Docker 可用时 `scripts/acceptance.sh` 会自动追加 container E2E；也可以强制运行：

```bash
CR_AGENT_ACCEPTANCE_DOCKER=always \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/acceptance.sh
```

隐藏样本通过外部 root 和 expected TSV 注入，不随仓库提交：

```bash
CR_AGENT_EVAL_FIXTURES_ROOT=/path/to/hidden-fixtures \
CR_AGENT_EVAL_FIXTURES="hidden-001.diff hidden-002.diff" \
CR_AGENT_EVAL_MATRIX=/path/to/expected.tsv \
CR_AGENT_EVAL_REPORT_ROOT=/tmp/cr-agent-hidden-reports \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/eval.sh
```

## 能力要求追踪

| # | Issue 要求 | 组件路径 | 测试覆盖 | 状态 | 缺口 |
|---|-----------|---------|---------|------|------|
| 1 | CR Skill（SKILL.md + 规则 + 脚本，>=4 类规则） | `skills/code-review/`、`internal/agent` | `agent_test.go`、`skill_test.go`、fixture tests | ✅ | 脚本输出 schema 可再文档化 |
| 2 | 沙箱执行（container/E2B，local 仅 fallback） | `codeexecutor/container`、`tool/workspaceexec`、`tool/codeexec`、E2B unsupported audit | workspaceexec 主路径/fallback tests + env-gated Docker test + E2B unsupported test | 🔶 | container E2E 已通过；E2B/Cube 真实 runtime adapter 未接入 |
| 3 | skill_run / workspace_exec / PermissionPolicy | `tool/skill`、`tool/workspaceexec`、`tool/codeexec`、`tool.PermissionPolicy` | `agent_test.go`、`policy_test.go` | ✅ | — |
| 4 | 输入解析（diff / 文件列表 / git 变更 / base-head） | `internal/agent.readInput`、`internal/agent.inputMetadata`、`internal/review/parser.go` | `parser_test.go`、`repo_test.go`、`agent_test.go`、CLI base/head test | ✅ | 不自动 fetch 远端 ref |
| 5 | 结构化 findings | `internal/review/types.go`、`internal/agent/model.go`、`internal/agent/model_http.go`、`internal/agent/model_official.go` | `engine_test.go`、fixture tests、model provider tests | ✅ | provider 已走官方 `model.Model` adapter；CLI 兼容入口已走官方 Runner/Event adapter |
| 6 | 数据库存储 | `internal/storage/sqlite` | `sqlite_test.go`、`agent_test.go` | ✅ | 当前 SQLite 是审计 store，不是官方 Session Service；session/sqlite / Memory 未接入 |
| 7 | 去重降噪 | `DedupeFindings`、`dedupe.diff` | `types_test.go`、fixture tests | ✅ | 更多低置信分类可扩展 |
| 8 | 安全边界 | Agent timeout/output limit/digest/redaction、artifact size/cap、env whitelist audit | `sandbox-safety.md` + sandbox failure/timeout tests + 多形态 secret 报告/DB 扫描 | 🔶 | runtime 级 env 强隔离依赖部署侧 executor 配置 |
| 9 | 监控审计 | SQLite metrics 表 + 官方 trace span + report metrics | report/agent/sqlite tests | 🔶 | telemetry attributes 已覆盖核心摘要；官方 metric exporter/OTLP dashboard 待部署集成 |

## 框架模块证据

| Issue 能力 | 当前实现 | 证据 |
|------------|----------|------|
| Skill 加载与执行 | `tool/skill` 的 `skill_load` / `skill_run` 执行 `skills/code-review/scripts/check.sh` | `internal/agent/execution.go`、`skills/code-review/SKILL.md` |
| workspace 级脚本 | `tool/workspaceexec` 执行 `go test ./...`、`go vet ./...`、可选 `staticcheck ./...` | `internal/agent/execution.go`、`TestAgentRunContainerRuntimeExecutesGoChecks` |
| CodeExecutor 沙箱 | 默认 `codeexecutor/container`，`local-fallback` 仅开发测试；`tool/codeexec` 是 Go checks fallback | `internal/agent/execution.go`、README runtime 说明 |
| Permission 治理 | 所有 `skill_run` / Go check 命令先过 `tool.PermissionPolicy`，非 allow 不执行 | `TestAgentRunDoesNotExecuteNonAllowPermission` |
| artifact | `review_report.json`、`review_report.md`、`review_diagnostics.json` 写入官方 artifact service，本地文件和 SQLite 引用继续保留 | `TestArtifactServiceReportsCanBeSavedAsArtifacts`、`TestAgentDefaultArtifactService` |
| LLM provider boundary | `fake-model` 模式在 Skill 后经官方 `model.Model` adapter 调用 `ModelReviewProvider`，默认 fake provider；显式 `--model-provider http` 调用 HTTP provider。输入/输出脱敏，失败降级人工复核 | `TestAgentRunFakeModelUsesProviderBoundary`、`TestModelReviewProviderModelImplementsOfficialModel`、HTTP/model provider tests |
| Event facade | CLI `Agent.Run` 通过官方 `event.Event` 输出 input/skill/sandbox/model/report/task 阶段事件 | `TestAgentRunEmitsOfficialEvents` |
| telemetry | 官方 trace span 记录 task、mode、runtime、输入类型、耗时、tool/model 调用数、model finding/exception、permission block、finding/artifact 数、severity/exception 分布和结论；SQLite metrics 表保存聚合指标 | `TestAgentRunRecordsTelemetryAttributes`、`TestAcceptanceEvidenceReportsAndSQLiteReplay` |
| SQLite 审计 | task、finding、permission/filter decision、sandbox run、artifact、metrics、report 按 task id 查询 | `TestAcceptanceEvidenceReportsAndSQLiteReplay` |

## 输入输出要求追踪

| 要求 | 实现 | 状态 |
|------|------|------|
| `--diff-file` | CLI flag + Agent input | ✅ |
| `--file-list` | CLI flag + Agent input | ✅ |
| `--repo-path` | git working tree diff / 普通目录 diff | ✅ |
| base/head ref | `--base-ref` / `--head-ref`，git repo 可生成 `base...head` diff，并进入 metadata/report/diagnostics/SQLite report/telemetry | ✅ |
| 测试 fixture | `--fixture` + `testdata/fixtures/` | ✅ |
| `review_report.json` | `internal/report.BuildJSON` | ✅ |
| `review_report.md` | `internal/report.BuildMarkdown` | ✅ |
| `review_diagnostics.json` | `internal/agent.buildDiagnostics`，包含 metrics / input metadata / governance / sandbox / artifacts / conclusion | ✅ |
| SQLite 查询 task 状态 | `TaskByID` | ✅ |
| SQLite 查询 sandbox run | `SandboxRunsByTaskID` | ✅ |
| SQLite 查询 permission decision | `DecisionsByTaskID` | ✅ |
| SQLite 查询 filter decision | `FilterDecisionsByTaskID` | ✅ |
| SQLite 查询 metrics | `MetricsByTaskID` | ✅ |
| SQLite 查询 findings | `FindingsByTaskID` | ✅ |
| SQLite 查询 artifact 引用 | `ArtifactsByTaskID` | ✅ |
| dry-run / fake-model / rule-only | Agent mode；fake-model 经过 `ModelReviewProvider` 边界，默认 fake provider，可显式 opt-in HTTP provider | ✅ |
| 示例输出 | `examples/review_report.json/md`、`examples/review_diagnostics.json` | ✅ |

## 交付物追踪

| 交付物 | 路径 | 状态 |
|--------|------|------|
| Go 入口与 CLI | `cmd/review-agent/main.go` | ✅ |
| CR Skill | `skills/code-review/SKILL.md` | ✅ |
| 规则文档 | `skills/code-review/rules.md` | ✅ |
| 沙箱脚本 | `skills/code-review/scripts/check.sh` | ✅ |
| Agent 编排 | `internal/agent/agent.go` | ✅ |
| DB schema | `internal/storage/sqlite/sqlite.go` | ✅ artifacts 表只保存引用、摘要和大小 |
| 8+ 测试样例 | `testdata/fixtures/*.diff` | ✅ 14 个 |
| 示例 report 输出 | `examples/review_report.json/md`、`examples/review_diagnostics.json` | ✅ |
| README | `README.md` | ✅ |
| 当前 roadmap | `docs/implementation-plan.md` | ✅ |

## 验收标准追踪

| # | 验收标准 | 状态 | 验证方式 | 缺口 |
|---|---------|------|---------|------|
| 1 | 8 条公开 diff 全部可运行并生成报告 | ✅ | `TestAllFixturesMatchExpectedReviewResults` 覆盖 14 条 fixture | — |
| 2 | 隐藏样本高危检出率 >= 80%，误报率 <= 15% | 🔶 | `scripts/eval.sh` 支持 external expected TSV、阈值门禁和报告保留 | 真实 hidden 样本本体不提交，仍需外部样本持续校准 |
| 3 | DB 完整记录 task/sandbox/finding/report，可按 task_id 查询 | ✅ | `sqlite_test.go`、`agent_test.go`、`TestAcceptanceEvidenceReportsAndSQLiteReplay` | — |
| 4 | 沙箱超时控制；失败不崩溃 | ✅ | `TestAgentRunRecordsSandboxFailureWithoutCrashing`、timeout test、container E2E、`sandbox-safety.md` | — |
| 5 | 脱敏检出率 >= 95%；报告/DB 无明文密钥 | ✅ | API key、LLM key、OpenAI key、Bearer、password、GitHub token、JWT-like token、private key、DB URL 报告/DB 全表扫描 | 仍需用隐藏样本持续校准 |
| 6 | dry-run/fake-model 全流程 <= 2 分钟 | ✅ | unit/integration tests | — |
| 7 | 高风险命令须先过 Filter/Permission；非 allow 不进沙箱 | ✅ | `policy_test.go` + Agent ask/deny E2E | — |
| 8 | 报告含 findings 摘要、severity 统计、人工复核项、治理拦截、监控、沙箱摘要、修复建议和 conclusion | ✅ | `report_test.go`、`agent_test.go` | — |

## SQLite 回放证据

`TestAcceptanceEvidenceReportsAndSQLiteReplay` 运行 `secret-shapes.diff` 后读取报告中的 `task_id`，并查询：

- `review_tasks`
- `findings`
- `permission_decisions`
- `filter_decisions`
- `sandbox_runs`
- `artifacts`
- `metrics`
- `reports`

README 中保留等价 SQL 查询示例。`examples/review.db` 可本地生成，但因 `*.db` 被忽略，不作为文本交付物提交。

## LLM Provider 边界追踪

| 要求 | 当前实现 | 状态 |
|------|----------|------|
| Provider 输入脱敏 | `ModelReviewInput.DiffSummary` 使用 `review.RedactSecrets`，existing findings 复用 `sanitizeFinding` | ✅ |
| 不绑定真实厂商 | 默认 `fakeModelProvider`；可选 `httpModelProvider` 使用标准库 `net/http`，无 OpenAI/Claude/Gemini SDK，无 API Key 默认路径保持可测 | ✅ |
| 复用 Finding 字段 | provider 输出是 `[]review.Finding` | ✅ |
| 高低置信分流 | high -> `findings`，其他 -> `warnings` + `needs_human_review` | ✅ |
| 与规则去重 | `file + line + category + rule_id` dedupe | ✅ |
| 失败不崩溃 | provider error -> `model-provider-failed` human review item + metrics exception | ✅ |
| 审计指标 | report/diagnostics/SQLite/telemetry 记录 model call、duration、exception、finding count | ✅ |
| 真实模型语义能力 | 已有 opt-in generic HTTP provider；真实效果取决于外部 endpoint，默认验收仍使用 fake provider | 🔶 |
| 官方模型路线 | 已实现 `trpc-agent-go/model.Model` adapter；`Agent.Run` 通过官方 Runner/Event adapter 暴露模型和任务阶段事件；内部执行体仍保留本项目 runDirect 以保持报告和 SQLite contract | ✅ |

## 规则覆盖追踪

| 规则类别 | rule_id | fixture | 检出 | severity/status |
|---------|---------|---------|------|-----------------|
| 敏感信息泄漏 | `secret-leak` | `secret.diff` | ✅ | critical/finding |
| 敏感信息多形态脱敏 | `secret-leak` | `secret-shapes.diff` | ✅ | critical/finding |
| 错误处理 | `panic-direct` | `panic.diff` | ✅ | high/finding |
| 可维护性 | `todo-marker` | `todo.diff` | ✅ | medium/finding |
| 测试缺失 | `missing-test-hint` | `test-missing.diff` | ✅ | low/warning |
| goroutine 泄漏 | `goroutine-leak` | `goroutine.diff` | ✅ | high/finding |
| context 泄漏 | `context-leak` | `context.diff` | ✅ | high/finding |
| 资源关闭 | `resource-leak` | `resource.diff` | ✅ | high/finding |
| DB 生命周期 | `db-lifecycle` | `db-lifecycle.diff` | ✅ | high/finding |
| 无问题 | — | `safe.diff` | ✅ | zero findings |

## 当前剩余缺口

1. 官方 `session/sqlite` / Memory 未接入；当前 SQLite 是审计 store，不是会话服务。
2. E2B/Cube 真实 runtime adapter 未接入；当前只有 `--runtime e2b` unsupported 审计入口。
3. hidden matrix 支持外部注入，但真实 hidden 数据未随仓库提交，也未在公开仓库内证明达标。
4. 官方 metric exporter / OTLP dashboard 未接；当前只有 trace span + SQLite metrics。
5. runtime 级 env 强隔离仍依赖部署侧 executor 配置；当前仓库记录 env whitelist 审计字段。

## 下一步

1. 用真实 hidden fixture matrix 跑一次验收。
2. 将 E2B unsupported 入口替换为真实 E2B/Cube runtime adapter。
3. 评估是否需要官方 `session/sqlite` / Memory 映射多轮评审。
4. 持续清理文档和状态矩阵，避免过期信息。

## 相关文档

- [architecture.md](architecture.md)
- [implementation-plan.md](implementation-plan.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [eval-matrix.md](eval-matrix.md)
- [data-contract.md](data-contract.md)
- [sandbox-safety.md](sandbox-safety.md)
- [ci.md](ci.md)
- [upstream-example-migration.md](upstream-example-migration.md)
