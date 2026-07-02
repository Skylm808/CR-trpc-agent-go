# Issue #2004 需求追踪矩阵

本文档将 Issue #2004 的 9 项能力要求、输入输出要求、交付物和验收标准映射到当前仓库实现。

## 能力要求追踪

| # | Issue 要求 | 组件路径 | 测试覆盖 | 状态 | 缺口 |
|---|-----------|---------|---------|------|------|
| 1 | CR Skill（SKILL.md + 规则 + 脚本，≥4 类规则） | `skills/code-review/`、`internal/agent` | `agent_test.go`、`skill_test.go`、fixture tests | ✅ | 脚本输出 schema 可再文档化 |
| 2 | 沙箱执行（container/E2B，local 仅 fallback） | `codeexecutor/container`、`tool/workspaceexec`、`tool/codeexec` | workspaceexec 主路径/fallback tests + env-gated Docker test | ✅ | Docker Desktop 下 container E2E 已通过；E2B 入口当前未做最小 adapter |
| 3 | skill_run / workspace_exec / PermissionPolicy | `tool/skill`、`tool/workspaceexec`、`tool/codeexec`、`tool.PermissionPolicy` | `agent_test.go`、`policy_test.go` | ✅ | — |
| 4 | 输入解析（diff / 文件列表 / git 变更） | `internal/agent.readInput`、`internal/agent.inputMetadata`、`internal/review/parser.go` | `parser_test.go`、`repo_test.go`、`agent_test.go` | 🔶 | diff / file-list / repo 和 Go metadata 已支持；base/head ref 未支持 |
| 5 | 结构化 findings | `internal/review/types.go` | `engine_test.go`、fixture tests | ✅ | — |
| 6 | 数据库存储 | `internal/storage/sqlite` | `sqlite_test.go`、`agent_test.go` | ✅ | — |
| 7 | 去重降噪 | `DedupeFindings`、`dedupe.diff` | `types_test.go`、fixture tests | ✅ | 更多低置信分类可扩展 |
| 8 | 安全边界 | Agent timeout/output limit/digest/redaction、artifact size/cap、env whitelist audit | sandbox failure/timeout tests + 多形态 secret 报告/DB 扫描 | 🔶 | runtime 级 env 强隔离待补 |
| 9 | 监控审计 | metrics 表 + trace span + report metrics | report/agent/sqlite tests | 🔶 | telemetry attributes 已含 severity/exception 分布；OTLP dashboard 待补 |

## 输入输出要求追踪

| 要求 | 实现 | 状态 |
|------|------|------|
| `--diff-file` | CLI flag + Agent input | ✅ |
| `--file-list` | CLI flag + Agent input | ✅ |
| `--repo-path` | git diff / 普通目录 diff | ✅ |
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
| dry-run / fake-model / rule-only | Agent mode | ✅ |
| 示例输出 | `examples/review_report.json/md` | ✅ |

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
| 300–500 字方案说明 | `docs/design-summary.md` | ✅ |
| Goal prompt | `docs/goal-prompt-framework-mvp.md` | ✅ |

## 验收标准追踪

| # | 验收标准 | 状态 | 验证方式 | 缺口 |
|---|---------|------|---------|------|
| 1 | 8 条公开 diff 全部可运行并生成报告 | ✅ | `TestAllFixturesMatchExpectedReviewResults` 覆盖 14 条 fixture | — |
| 2 | 隐藏样本高危检出率 ≥ 80%，误报率 ≤ 15% | 🔶 | `scripts/eval.sh` 公开样本 eval | 隐藏样本 expected matrix/CI 注入待补，契约见 `docs/eval-matrix.md` |
| 3 | DB 完整记录 task/sandbox/finding/report，可按 task_id 查询 | ✅ | `sqlite_test.go`、`agent_test.go`、`TestAcceptanceEvidenceReportsAndSQLiteReplay` | — |
| 4 | 沙箱超时控制；失败不崩溃 | ✅ | `TestAgentRunRecordsSandboxFailureWithoutCrashing`、timeout test、container E2E | Docker Desktop 下 env-gated container test 已通过 |
| 5 | 脱敏检出率 ≥ 95%；报告/DB 无明文密钥 | ✅ | API key、LLM key、OpenAI key、Bearer、password、GitHub token、JWT-like token、private key、DB URL 报告/DB 全表扫描 | 仍需用隐藏样本持续校准 |
| 6 | dry-run/fake-model 全流程 ≤ 2 分钟 | ✅ | unit/integration tests | — |
| 7 | 高风险命令须先过 Filter/Permission；非 allow 不进沙箱 | ✅ | `policy_test.go` + Agent ask/deny E2E | — |
| 8 | 报告含 findings 摘要、severity 统计、人工复核项、治理拦截、监控、沙箱摘要、修复建议和 conclusion | ✅ | `report_test.go`、`agent_test.go` | — |

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

## 下一步

1. 在 CI 中开启 Docker daemon 后运行 container runtime E2E，保持本机 Docker Desktop 验证结果可复现。
2. Runner/Event、Session/Memory 和 E2B 暂不接入的边界见 `issue-acceptance.md`；telemetry 已有 trace span 和审查摘要属性，artifact service 已有报告和诊断产物最小接入，SQLite artifacts 表仅作为引用索引。
3. 为隐藏样本扩展外部 expected matrix 输入。
4. 如需正式交付，可继续用隐藏样本 expected matrix 校准检出率和误报率。

## 相关文档

- [architecture.md](architecture.md)
- [issue-acceptance.md](issue-acceptance.md)
- [framework-first-mvp.md](framework-first-mvp.md)
- [implementation-plan.md](implementation-plan.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [data-contract.md](data-contract.md)
- [goal-prompt-framework-mvp.md](goal-prompt-framework-mvp.md)
