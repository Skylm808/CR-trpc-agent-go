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
| `tool/codeexec` 执行 Go 检查 | ✅ | `runGoSandboxChecks` |
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
| SQLite 记录 sandbox_runs | ✅ | 增加 artifact_count / finished_at |
| SQLite 记录 findings/warnings/human review items | ✅ | 统一写入 `findings` 表，用 `status` 区分 |
| SQLite 记录 artifacts | ✅ | 当前是报告产物记录，未接官方 artifact service |
| SQLite 记录 metrics | ✅ | 后续接官方 telemetry hook |
| SQLite 记录 reports | ✅ | — |
| 按 task_id 查询全部核心实体 | ✅ | — |
| 报告含 governance/sandbox/human review/artifacts | ✅ | 补 conclusion 字段 |

建议后续小改：

1. 将 `Store` interface 从 `internal/agent` 抽到 `internal/storage/store.go`。
2. 在 `sandbox_runs` 增加 `finished_at` 和 `artifact_count`。
3. 增加 report conclusion 字段，便于 CI 汇总。

## M4：真实沙箱、治理与遥测增强 🔶

这是当前最高优先级。

| 任务 | 状态 | 验证方式 |
|------|------|----------|
| Docker container runtime 真实 E2E | ⬜ | `CR_AGENT_RUN_CONTAINER_TESTS=1 go test ./internal/agent -run Container` |
| container bind mount repo 到 `/workspace/repo` | ✅ | `ContainerRepoHostPath` + `WithBindMount` |
| E2B runtime 入口 | ⬜ | CLI/runtime adapter 或明确 unsupported |
| ask/needs_human_review 不进入 executor | 🔶 | policy 单测有基础，需 Agent E2E |
| deny 不进入 executor | 🔶 | 需 Agent E2E |
| env whitelist 强校验 | 🔶 | 当前记录 `PATH,HOME,TMPDIR`，未强制过滤所有 env |
| artifact cap | ⬜ | 当前只记录 report artifacts |
| 官方 telemetry hook | ⬜ | 当前是本地 metrics 表 |

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
| hidden/eval 评测脚本 | ⬜ | 增加 precision/recall 统计 |
| Docker/E2B 使用说明 | 🔶 | Docker 路径需验证 |

## 当前验收对照

| # | 标准 | 当前状态 | 缺口 |
|---|------|----------|------|
| 1 | 8 条公开 diff 全部可运行并生成报告 | ✅ | — |
| 2 | 隐藏样本高危检出率 ≥ 80%，误报率 ≤ 15% | ⬜ | 缺 hidden/eval 脚本 |
| 3 | DB 完整记录 task/sandbox/finding/report，按 task_id 查询 | ✅ | — |
| 4 | 沙箱超时控制；失败不崩溃 | ✅ local fallback 已测 | container 真实超时需测 |
| 5 | 脱敏检出率 ≥ 95%；报告/DB 无明文密钥 | 🔶 | DB 全表扫描已有；仍需更多 secret 样本 |
| 6 | dry-run/fake-model 全流程 ≤ 2 分钟 | ✅ | — |
| 7 | 高风险命令须先过 Filter/Permission | 🔶 | ask/deny Agent E2E 待补 |
| 8 | 报告含摘要、统计、人审、治理、监控、沙箱、建议 | ✅ | 可补 conclusion |

## 下一阶段推荐顺序

1. 写 container integration test，默认跳过，显式环境变量才跑 Docker。
2. 补 Agent 层 ask/deny/needs_human_review 不执行 executor 的测试。
3. 抽 `internal/storage/store.go`，降低 Agent 对 SQLite 包的耦合。
4. 增加 `scripts/eval.sh` 或 Go eval command，输出公开/隐藏样本的 recall、precision、耗时。
5. 补 E2B runtime 的最小 adapter 或文档化暂不支持。

## Definition of Done

- [x] 所有公开 fixture 产出报告且 rule_id/severity/status 符合矩阵。
- [x] findings 结构化、去重、低置信度分流正确。
- [x] SQLite 记录 task / decisions / sandbox runs / artifacts / metrics / reports。
- [x] 沙箱失败、超时不崩溃 review，且写入 DB。
- [x] 报告和 finding evidence 中不出现明文 API Key / token / password。
- [ ] container runtime 真实 E2E 验证。
- [x] DB 全表 secret 扫描测试。
- [ ] ask/deny/needs_human_review Agent E2E 测试。
- [ ] hidden/eval 评测脚本。
- [ ] 官方 artifact/session/telemetry 能力的最小接入或清晰边界说明。

## 相关文档

- [architecture.md](architecture.md)
- [framework-first-mvp.md](framework-first-mvp.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [goal-prompt-framework-mvp.md](goal-prompt-framework-mvp.md)
