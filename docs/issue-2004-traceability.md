# Issue #2004 需求追踪矩阵

本文档将 Issue #2004 的 9 项能力要求、输入输出要求、交付物和验收标准映射到当前仓库实现。

## 能力要求追踪

| # | Issue 要求 | 组件路径 | 测试覆盖 | 状态 | 缺口 |
|---|-----------|---------|---------|------|------|
| 1 | CR Skill（SKILL.md + 规则 + 脚本，≥4 类规则） | `skills/code-review/`、`internal/agent` | `agent_test.go`、`skill_test.go`、fixture tests | ✅ | 脚本输出 schema 可再文档化 |
| 2 | 沙箱执行（container/E2B，local 仅 fallback） | `codeexecutor/container`、`tool/codeexec` | local fallback tests | 🔶 | 真实 Docker/E2B E2E 待补 |
| 3 | skill_run / workspace_exec / PermissionPolicy | `tool/skill`、`tool/codeexec`、`tool.PermissionPolicy` | `agent_test.go`、`policy_test.go` | ✅ | ask/deny Agent E2E 待补 |
| 4 | 输入解析（diff / 文件列表 / git 变更） | `internal/agent.readInput`、`internal/review/parser.go` | `parser_test.go`、`repo_test.go` | 🔶 | 文件路径列表、base/head ref 未支持 |
| 5 | 结构化 findings | `internal/review/types.go` | `engine_test.go`、fixture tests | ✅ | — |
| 6 | 数据库存储 | `internal/storage/sqlite` | `sqlite_test.go`、`agent_test.go` | ✅ | warnings 持久化语义待定 |
| 7 | 去重降噪 | `DedupeFindings`、`dedupe.diff` | `types_test.go`、fixture tests | ✅ | 更多低置信分类可扩展 |
| 8 | 安全边界 | Agent timeout/output limit/digest/redaction | sandbox failure/timeout tests | 🔶 | artifact cap、env 强白名单、DB 全表 secret 扫描待补 |
| 9 | 监控审计 | metrics 表 + report metrics | report/agent/sqlite tests | 🔶 | 官方 telemetry hook 待接 |

## 输入输出要求追踪

| 要求 | 实现 | 状态 |
|------|------|------|
| `--diff-file` | CLI flag + Agent input | ✅ |
| `--repo-path` | git diff / 普通目录 diff | ✅ |
| 测试 fixture | `--fixture` + `testdata/fixtures/` | ✅ |
| `review_report.json` | `internal/report.BuildJSON` | ✅ |
| `review_report.md` | `internal/report.BuildMarkdown` | ✅ |
| SQLite 查询 task 状态 | `TaskByID` | ✅ |
| SQLite 查询 sandbox run | `SandboxRunsByTaskID` | ✅ |
| SQLite 查询 permission decision | `DecisionsByTaskID` | ✅ |
| SQLite 查询 filter decision | `FilterDecisionsByTaskID` | ✅ |
| SQLite 查询 metrics | `MetricsByTaskID` | ✅ |
| SQLite 查询 findings | `FindingsByTaskID` | ✅ |
| SQLite 查询 artifacts | `ArtifactsByTaskID` | ✅ |
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
| DB schema | `internal/storage/sqlite/sqlite.go` | ✅ |
| 8+ 测试样例 | `testdata/fixtures/*.diff` | ✅ 13 个 |
| 示例 report 输出 | `examples/review_report.json/md` | ✅ |
| README | `README.md` | ✅ |
| 300–500 字方案说明 | `docs/design-summary.md` | ✅ |
| Goal prompt | `docs/goal-prompt-framework-mvp.md` | ✅ |

## 验收标准追踪

| # | 验收标准 | 状态 | 验证方式 | 缺口 |
|---|---------|------|---------|------|
| 1 | 8 条公开 diff 全部可运行并生成报告 | ✅ | `TestAllFixturesMatchExpectedReviewResults` | — |
| 2 | 隐藏样本高危检出率 ≥ 80%，误报率 ≤ 15% | ⬜ | 待建 eval | 缺 hidden/eval |
| 3 | DB 完整记录 task/sandbox/finding/report，可按 task_id 查询 | ✅ | `sqlite_test.go`、`agent_test.go` | warnings 是否入库需明确 |
| 4 | 沙箱超时控制；失败不崩溃 | ✅ | `TestAgentRunRecordsSandboxFailureWithoutCrashing`、timeout test | container 真实 E2E 待补 |
| 5 | 脱敏检出率 ≥ 95%；报告/DB 无明文密钥 | 🔶 | secret fixture + report assertion | 需 DB 全表扫描与更多 secret 样本 |
| 6 | dry-run/fake-model 全流程 ≤ 2 分钟 | ✅ | unit/integration tests | — |
| 7 | 高风险命令须先过 Filter/Permission；非 allow 不进沙箱 | 🔶 | `policy_test.go` | Agent ask/deny E2E 待补 |
| 8 | 报告含 findings 摘要、severity 统计、人工复核项、治理拦截、监控、沙箱摘要、修复建议 | ✅ | `report_test.go`、`agent_test.go` | 可补 conclusion |

## 规则覆盖追踪

| 规则类别 | rule_id | fixture | 检出 | severity/status |
|---------|---------|---------|------|-----------------|
| 敏感信息泄漏 | `secret-leak` | `secret.diff` | ✅ | critical/finding |
| 错误处理 | `panic-direct` | `panic.diff` | ✅ | high/finding |
| 可维护性 | `todo-marker` | `todo.diff` | ✅ | medium/finding |
| 测试缺失 | `missing-test-hint` | `test-missing.diff` | ✅ | low/warning |
| goroutine 泄漏 | `goroutine-leak` | `goroutine.diff` | ✅ | high/finding |
| context 泄漏 | `context-leak` | `context.diff` | ✅ | high/finding |
| 资源关闭 | `resource-leak` | `resource.diff` | ✅ | high/finding |
| DB 生命周期 | `db-lifecycle` | `db-lifecycle.diff` | ✅ | high/finding |
| 无问题 | — | `safe.diff` | ✅ | zero findings |

## 下一步

1. 增加 Docker container runtime E2E，默认 skip，显式环境变量运行。
2. 增加 Agent ask/deny/needs_human_review 不进入 executor 的测试。
3. 增加 DB 全表 secret 扫描测试。
4. 抽出 `internal/storage/store.go`。
5. 增加 hidden/eval 评测脚本。
6. 明确 E2B、artifact service、session/sqlite、telemetry 的最小接入边界。

## 相关文档

- [architecture.md](architecture.md)
- [framework-first-mvp.md](framework-first-mvp.md)
- [implementation-plan.md](implementation-plan.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [data-contract.md](data-contract.md)
- [goal-prompt-framework-mvp.md](goal-prompt-framework-mvp.md)
