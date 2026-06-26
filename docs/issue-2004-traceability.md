# Issue #2004 需求追踪矩阵

本文档将 Issue #2004 的 9 项能力要求、8 条验收标准与仓库组件、测试、当前状态一一对应，便于 PR 评审与 Issue 进度更新。

**重要纠偏：当前最高优先级是接入 `trpc-agent-go` 框架能力。** 纯本地 diff parser、rules、sandbox、SQLite 只能作为迁移基础；如果没有 `tool/skill`、`PermissionPolicy`、`codeexecutor/container|e2b`、Filter、Telemetry 的实际调用链，不能视为满足本 Issue。

## 能力要求追踪

| # | Issue 要求 | 组件路径 | 测试覆盖 | 状态 | 阻塞项 |
|---|-----------|---------|---------|------|--------|
| 1 | CR Skill（SKILL.md + 规则 + 脚本，≥4 类规则） | `skills/code-review/`、`tool/skill` adapter、`internal/review/engine.go` | `skill_test.go` + framework integration test | 🔶 | Skill 目录已有，但未通过框架 skill load / skill run 编排 |
| 2 | 沙箱执行（container/E2B，local 仅 fallback） | `codeexecutor/container` / `codeexecutor/e2b` adapter | sandbox integration test | ⬜ | 当前仅本地 runner，必须迁移到框架 CodeExecutor |
| 3 | skill_run / workspace_exec / PermissionPolicy | `tool.PermissionPolicy` wrapper、`tool/workspaceexec` 或 `tool/codeexec` | policy + integration test | ⬜ | 自研 Policy 只能临时兼容，未接框架 PermissionPolicy |
| 4 | 输入解析（diff / 文件列表 / git 变更） | `internal/review/parser.go`、`cmd/review-agent/run.go` | `parser_test.go`、`repo_test.go` | 🔶 | 文件路径列表、base/head ref 未支持 |
| 5 | 结构化 findings（severity/category/file/line/...） | `internal/review/types.go` | `engine_test.go`、`types_test.go` | ✅ | — |
| 6 | 数据库存储（task/run/decision/finding/artifact/report） | `session/sqlite` 或兼容 `storage.Store` + `internal/storage/sqlite` | `sqlite_test.go` | 🔶 | 无 Store interface；artifact/filter 表缺失；需记录框架执行事件 |
| 7 | 去重降噪（同文件同行不重复；低置信度分流） | `internal/review/types.go` DedupeFindings | `types_test.go` | 🔶 | needs_human_review 分流未实现；dedupe fixture 缺失 |
| 8 | 安全边界（timeout/output limit/env whitelist/脱敏/artifact cap） | `internal/sandbox/runner.go`、`RedactSecrets` | `runner_test.go` | 🔶 | output limit、env whitelist、artifact cap 未实现 |
| 9 | 监控审计（耗时/工具调用/拦截/finding 分布/异常分布） | `internal/review/types.go` Metrics | `report_test.go` | 🔶 | exception_counts、redaction_count 未完整填充 |

## 输入输出要求追踪

| 要求 | 实现 | 状态 |
|------|------|------|
| `--diff-file` | `cmd/review-agent/main.go` | ✅ |
| `--repo-path` | `run.go` diffFromRepo | ✅ |
| 测试 fixture | `testdata/fixtures/` + `fixtures_test.go` | ✅ |
| `review_report.json` | `internal/report/report.go` | ✅ |
| `review_report.md` | `internal/report/report.go` | 🔶 缺 governance/sandbox/human_review 段 |
| SQLite 可查询 task 状态 | `sqlite.TaskByID` | ✅ |
| SQLite 可查询 sandbox run | `SaveSandboxRun`（字段不完整） | 🔶 |
| SQLite 可查询 permission decision | `SaveDecision` | ✅ |
| SQLite 可查询 metrics | `MetricsByTaskID` | ✅ |
| SQLite 可查询 findings | `FindingsByTaskID` | ✅ |
| SQLite 可查询 artifact | — | ⬜ |
| dry-run / fake-model / rule-only | `--mode` flag | 🔶 仅 rule-only 生效 |
| 示例目录 | `cmd/review-agent/` | ✅ |

## 交付物追踪

| 交付物 | 路径 | 状态 |
|--------|------|------|
| Go 入口与 CLI | `cmd/review-agent/main.go` | ✅ |
| CR Skill | `skills/code-review/SKILL.md` | 🔶 |
| 规则文档 | `skills/code-review/rules.md` | 🔶 仅 4 条，与 engine 部分对应 |
| 沙箱脚本 | `skills/code-review/scripts/check.sh` | 🔶 占位脚本 |
| Agent 编排 | `cmd/review-agent/run.go` | 🔶 |
| DB schema | `internal/storage/sqlite/sqlite.go` Init() | 🔶 |
| 8+ 测试样例 | `testdata/fixtures/*.diff` | ✅ 10 个（缺 dedupe/sandbox-fail） |
| 示例 report 输出 | `testdata/expected/` | ⬜ |
| README | `README.md` | ✅ |
| 300–500 字方案说明 | `docs/design-summary.md` | ✅ |

## 验收标准追踪

| # | 验收标准 | 状态 | 验证方式 | 缺口 |
|---|---------|------|---------|------|
| 1 | 8 条公开 diff 全部可运行并生成报告 | ✅ | `TestAllFixturesGenerateReports` | — |
| 2 | 隐藏样本高危检出率 ≥ 80%，误报率 ≤ 15% | ⬜ | `scripts/eval.sh`（待建） | 规则未补全；无 hidden 样本集 |
| 3 | DB 完整记录 task/sandbox/finding/report，可按 task_id 查询 | 🔶 | `sqlite_test.go` | artifact、filter 缺失；sandbox 字段不完整 |
| 4 | 沙箱超时控制；失败不崩溃 | 🔶 | `runner_test.go`、`sandbox_test.go` | output size limit 未实现 |
| 5 | 脱敏检出率 ≥ 95%；报告/DB 无明文密钥 | 🔶 | `types_test.go` RedactSecrets | redaction_count 未计数；缺专项 fixture 断言 |
| 6 | dry-run/fake-model 全流程 ≤ 2 分钟 | ✅ | rule-only 实测 | mode 分支待实现后需再测 |
| 7 | 高风险命令须先过 Filter/Permission；deny 不进沙箱 | 🔶 | `policy_test.go` | ask/needs_human_review 链路未完整 |
| 8 | 报告含 findings 摘要、severity 统计、人工复核项、治理拦截、监控、沙箱摘要、修复建议 | 🔶 | `report_test.go` | 报告字段待补全 |

## 规则覆盖追踪

Issue 要求 7 类规则中至少 4 类。当前与目标：

| 规则类别 | rule_id | fixture | 检出 | 目标 severity |
|---------|---------|---------|------|--------------|
| 敏感信息泄漏 | `secret-leak` | `secret.diff` | ✅ | critical |
| 错误处理 | `panic-direct` | `panic.diff` | ✅ | high |
| 可维护性 | `todo-marker` | `todo.diff` | ✅ | medium |
| 测试缺失 | `missing-test-hint` | `test-missing.diff` | ✅ | low (warning) |
| goroutine 泄漏 | `goroutine-leak` | `goroutine.diff` | ⬜ | high |
| context 泄漏 | `context-leak` | `context.diff` | ⬜ | high |
| 资源关闭 | `resource-leak` | `resource.diff` | ⬜ | high |
| DB 生命周期 | `db-lifecycle` | `db-lifecycle.diff` | ⬜ | high |
| 无问题 | — | `safe.diff` | ✅ 零 finding | — |

## 下一步（按优先级）

1. **M1** — 优先接入 trpc-agent-go：Skill run + PermissionPolicy + CodeExecutor(container/E2B)
2. **M2** — 补全 4 类缺失 Go 规则 + fixture 预期断言测试
3. **M3** — Storage interface + artifact/filter 表 + 框架执行事件落库
4. **M4** — 示例 report 输出 + 隐藏样本评测脚本

## 相关文档

- [architecture.md](architecture.md)
- [framework-first-mvp.md](framework-first-mvp.md)
- [implementation-plan.md](implementation-plan.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [data-contract.md](data-contract.md)
