# 实现计划

本文档按 Issue #2004 验收优先级排列。当前仓库已经完成第一轮 framework-first 纠偏：官方 `trpc-agent-go` 的 Skill、PermissionPolicy、CodeExecutor 和 SQLite 审计链路已进入主线。下一阶段目标是把它从“可跑通原型”推进到“可验收 MVP”。

## 里程碑总览

```text
M0  本地 rule-only 原型                    ✅ 已作为迁移基础
M1  trpc-agent-go 最小链路                 ✅ 已打通
M2  规则补全 + fixture 预期                ✅ 公开 fixture 已覆盖
M3  存储与报告对齐契约                     🔶 核心完成，仍需收口
M4  真实沙箱/治理/遥测增强                 🔶 当前最高优先级
M5  验收交付与评测                         ⬜
```

## M1：trpc-agent-go 最小链路 ✅

| 任务 | 状态 | 证据 |
|------|------|------|
| 添加官方 `trpc-agent-go` 依赖 | ✅ | `go.mod` |
| 建立 `internal/agent` 编排层 | ✅ | CLI 调用 Agent |
| `tool/skill` 加载 `skills/code-review` | ✅ | `agent.New` + `NewLoadTool` |
| `skill_run` 执行 `scripts/check.sh` | ✅ | `runSkillChecks` |
| `tool.PermissionPolicy` 执行前决策 | ✅ | `defaultPermissionPolicy` |
| 默认 `codeexecutor/container` | ✅ | `RuntimeContainer` 默认值 |
| `local-fallback` 仅显式启用 | ✅ | CLI `--runtime local-fallback` |
| `tool/workspaceexec` 执行 Go 检查 | ✅ | `runGoSandboxChecks` 优先走 workspace，`tool/codeexec` 兜底 |
| `dry-run` / `sandbox` / `fake-model` / `rule-only` mode | ✅ | CLI flag + Agent branch |

## M2：规则补全 + Fixture 预期 ✅

| rule_id | 类别 | fixture | 状态 |
|---------|------|---------|------|
| `secret-leak` | 敏感信息 | `secret.diff` | ✅ |
| `panic-direct` | 错误处理 | `panic.diff` | ✅ |
| `todo-marker` | 可维护性 | `todo.diff` | ✅ |
| `missing-test-hint` | 测试缺失 | `test-missing.diff`、`missing-test.diff` | ✅ warning |
| `goroutine-leak` | goroutine 泄漏 | `goroutine.diff` | ✅ |
| `context-leak` | context 生命周期 | `context.diff` | ✅ |
| `resource-leak` | 资源关闭 | `resource.diff` | ✅ |
| `db-lifecycle` | DB 生命周期 | `db-lifecycle.diff` | ✅ |
| 去重 | duplicate panic | `dedupe.diff` | ✅ |
| 沙箱失败 | command exit != 0 | `sandbox-fail.diff` | ✅ |
| 沙箱超时 | timed out | `sandbox-timeout.diff` | ✅ |

公开 fixture 已从“报告存在”升级为 rule_id/severity/status 断言。

## M3：存储与报告对齐契约 🔶

| 任务 | 状态 | 下一步 |
|------|------|--------|
| SQLite 记录 task | ✅ | — |
| SQLite 记录 permission_decisions | ✅ | 增加 policy_name 字段可选 |
| SQLite 记录 filter_decisions | ✅ | 扩展非 secret filter |
| SQLite 记录 sandbox_runs | ✅ | 已记录 artifact_count / finished_at |
| SQLite 记录 findings/warnings/human review items | ✅ | 统一写入 `findings` 表，用 `status` 区分 |
| SQLite 记录 artifacts | ✅ | 报告和诊断产物引用已记录，官方 artifact service 已有最小接入 |
| SQLite 记录 metrics | ✅ | trace span 同步记录审查摘要属性 |
| SQLite 记录 reports | ✅ | — |
| 按 task_id 查询全部核心实体 | ✅ | — |
| 报告含 governance/sandbox/human review/artifacts | ✅ | 补 conclusion 字段 |

建议后续小改：

1. ~~在 `sandbox_runs` 增加 `finished_at` 和 `artifact_count`。~~ 已完成。
2. 增加 report conclusion 字段，便于 CI 汇总。

## M4：真实沙箱、治理与遥测增强 🔶

这是当前最高优先级。

| 任务 | 状态 | 验证方式 |
|------|------|----------|
| Docker container runtime 真实 E2E | 🔶 | env-gated test 已加；需 Docker daemon 执行 |
| container bind mount repo 到 `/workspace/repo` | ✅ | `ContainerRepoHostPath` + `WithBindMount` |
| E2B runtime 入口 | ⬜ | CLI/runtime adapter 或明确 unsupported |
| ask/needs_human_review 不进入 executor | ✅ | Agent E2E 覆盖 ask |
| deny 不进入 executor | ✅ | Agent E2E 覆盖 deny |
| env whitelist 强校验 | 🔶 | 当前记录 `PATH,HOME,TMPDIR`，未强制过滤所有 env |
| artifact cap | 🔶 | 当前记录报告和诊断产物，未增加 size cap 字段 |
| 官方 telemetry hook | 🔶 | 当前有 trace span + 审查摘要属性 + 本地 metrics 表 |

## M5：验收交付与评测 ⬜

| 交付物 | 状态 | 下一步 |
|--------|------|--------|
| Go 示例入口 | ✅ | — |
| Skill + rules + script | ✅ | 增加脚本 schema 文档 |
| 数据库 schema + SQLite 实现 | ✅ | 补 migration 说明 |
| 8+ diff 样本 | ✅ | — |
| 示例输出 | ✅ | `examples/review_report.json/md` |
| README | ✅ | — |
| 300–500 字方案说明 | ✅ | `design-summary.md` |
| hidden/eval 评测脚本 | 🔶 | 公开 fixture eval 已有；隐藏样本可通过外部 root 注入，契约见 `docs/eval-matrix.md` |
| Docker/E2B 使用说明 | 🔶 | Docker test 命令已写；E2B 入口待补或文档化暂不支持 |

## 当前验收对照

| # | 标准 | 当前状态 | 缺口 |
|---|------|----------|------|
| 1 | 8 条公开 diff 全部可运行并生成报告 | ✅ | — |
| 2 | 隐藏样本高危检出率 ≥ 80%，误报率 ≤ 15% | ⬜ | 缺 hidden/eval 脚本，契约见 `docs/eval-matrix.md` |
| 3 | DB 完整记录 task/sandbox/finding/report，按 task_id 查询 | ✅ | — |
| 4 | 沙箱超时控制；失败不崩溃 | ✅ local fallback 已测 | container 真实超时需测 |
| 5 | 脱敏检出率 ≥ 95%；报告/DB 无明文密钥 | 🔶 | DB 全表扫描已有；仍需更多 secret 样本 |
| 6 | dry-run/fake-model 全流程 ≤ 2 分钟 | ✅ | — |
| 7 | 高风险命令须先过 Filter/Permission | ✅ | — |
| 8 | 报告含摘要、统计、人审、治理、监控、沙箱、建议 | ✅ | 可补 conclusion |

## 下一阶段推荐顺序

1. 在有 Docker daemon 的 CI/机器上运行 env-gated container integration test。
2. 补 E2B runtime 的最小 adapter 或文档化暂不支持。
3. 将隐藏样本 expected matrix 接入 `scripts/eval.sh` 或 CI 注入。
4. 给 report 增加 `conclusion` 字段。

## Definition of Done

- [x] 所有公开 fixture 产出报告且 rule_id/severity/status 符合矩阵。
- [x] findings 结构化、去重、低置信度分流正确。
- [x] SQLite 记录 task / decisions / sandbox runs / artifacts / metrics / reports。
- [x] 沙箱失败、超时不崩溃 review，且写入 DB。
- [x] 报告和 finding evidence 中不出现明文 API Key / token / password。
- [ ] container runtime 真实 E2E 在 Docker 环境中验证。
- [x] DB 全表 secret 扫描测试。
- [x] ask/deny/needs_human_review Agent E2E 测试。
- [x] 公开 fixture eval 脚本。
- [x] 官方 artifact/telemetry 能力的最小接入或清晰边界说明；session/sqlite 仍是后续演进。

## 相关文档

- [architecture.md](architecture.md)
- [framework-first-mvp.md](framework-first-mvp.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [goal-prompt-framework-mvp.md](goal-prompt-framework-mvp.md)
