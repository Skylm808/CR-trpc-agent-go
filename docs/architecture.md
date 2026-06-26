# CR-trpc-agent-go 架构设计

## 目标

在官方 `trpc-agent-go` 之上构建面向 Go 工程的自动代码评审 Agent。第一版要证明的不是“LLM 能评论 diff”，而是一条可验证、可落库、可审计的 CR 链路：

- 读取 unified diff、fixture 或本地 git 工作区变更。
- 通过 `tool/skill` 加载并运行 `skills/code-review`。
- 高风险命令进入沙箱前经过 `tool.PermissionPolicy` 决策。
- 默认使用 `codeexecutor/container`，`local-fallback` 仅用于开发和测试。
- 结构化输出 findings、warnings、human review items、治理摘要、沙箱摘要、指标和报告产物。
- SQLite 记录 task、permission/filter decision、sandbox run、finding、artifact、metrics、report。

## 设计原则

本仓库是 **trpc-agent-go 之上的应用示例**，不是框架 fork。本仓库负责 Go CR 业务逻辑、规则、报告、schema、fixture 和验收测试；官方框架负责 Skill、Tool、PermissionPolicy、CodeExecutor 等可复用原语。

当前代码已经接入：

- `trpc.group/trpc-go/trpc-agent-go v1.10.0`
- `trpc.group/trpc-go/trpc-agent-go/codeexecutor/container v1.10.0`
- `tool/skill` 的 load/run tool
- `tool/codeexec` 的 Go 检查执行入口
- `tool.PermissionPolicy`
- `codeexecutor/container` 和显式 `codeexecutor/local` fallback

仍未完成的框架侧增强：

- 真实 Docker container 端到端验证。
- E2B/Cube runtime adapter。
- 官方 artifact service 接入；当前 artifact 为 SQLite 中的报告产物记录。
- `session/sqlite` 作为 Agent session/history 的直接使用；当前使用本项目 SQLite store。
- 更完整的 telemetry hook；当前先将耗时、异常、severity 分布等落入 metrics 表。

## 系统流程

```text
CLI 输入（--diff-file / --repo-path / --fixture）
  -> internal/agent.Agent
  -> trpc-agent-go/tool/skill skill_load(code-review)
  -> tool.PermissionPolicy 决策 scripts/check.sh
  -> trpc-agent-go/tool/skill skill_run(scripts/check.sh)
  -> 可选 sandbox 模式：tool/codeexec 执行 go test / go vet / staticcheck
  -> 合并 Skill 输出，去重，脱敏，分流 warnings / human_review_items
  -> 生成 review_report.json 和 review_report.md
  -> SQLite 保存 task / decisions / runs / findings / artifacts / metrics / report
```

## 框架集成映射

| 本仓库组件 | trpc-agent-go 对应能力 | 当前状态 |
|-----------|----------------------|---------|
| Skill 加载与规则脚本 | `tool/skill`、`skill.NewFSRepository` | ✅ 已接入 `skill_load` / `skill_run` |
| Go 检查执行 | `tool/codeexec` | ✅ `sandbox` 模式执行 `go test` / `go vet`，`--staticcheck` 可选 |
| 沙箱 runtime | `codeexecutor/container`、`codeexecutor/local` | 🔶 container 为默认；local 仅显式 fallback；Docker E2E 未验证 |
| 命令治理 | `tool.PermissionPolicy` | ✅ 固定 allowlist，非 allow 不进入 executor |
| 内容过滤 | 本项目 Filter/Redaction 记录 | 🔶 secret 脱敏和 filter_decision 已落库，策略仍需扩展 |
| 持久化 | 兼容 Store + SQLite | ✅ Agent 层有 Store interface，SQLite 已存 task/run/decision/finding/artifact/report/metrics |
| 遥测 | metrics 表；后续接 telemetry hook | 🔶 已记录核心指标，未接官方 telemetry hook |
| 产物 | artifact 目标；当前 SQLite artifact record | 🔶 报告产物已记录，未接官方 artifact service |

## CLI Mode

| Mode | 行为 | 用途 |
|------|------|------|
| `rule-only` | 加载 Skill 并运行 deterministic 脚本，不依赖模型 | 默认测试路径 |
| `dry-run` | 加载 Skill，记录 skipped decision/run，不执行沙箱 | 验证治理与报告链路 |
| `sandbox` | 执行 Skill，并在有 `--repo-path` 时执行 Go 检查 | 接近生产路径 |
| `fake-model` | 复用 deterministic Skill 链路，不使用真实模型 key | 无 API Key 的模型路径替身 |

## 输入解析

当前支持：

- `--diff-file`：读取 unified diff。
- `--fixture` + `--fixtures-root`：读取公开测试样本。
- `--repo-path`：git 仓库使用 `git diff --unified=3`；普通目录转换为新增文件 diff。

待补强：

- 文件路径列表输入。
- base/head ref。
- 更强 Go package/module 元数据提取。

## Skill 设计

`skills/code-review/` 包含：

- `SKILL.md`：Skill 入口、运行方式、安全约束。
- `rules.md`：规则说明，与 `rule_id` 对齐。
- `scripts/check.sh`：确定性 diff 检查脚本，输出 JSON findings/warnings。

CLI 不直接调用脚本，而是通过 Agent 的 `tool/skill` adapter 运行。

## Permission 与 Filter

Permission 负责“命令能不能执行”：

- 允许 `scripts/check.sh`。
- 允许 `go test ./...`、`go vet ./...`、显式 `--staticcheck` 下的 `staticcheck ./...`。
- 未识别命令返回 `ask`，不会进入 executor。
- 所有决策写入 `permission_decisions`。

Filter 负责“内容能不能进入报告/数据库”：

- evidence、错误信息和报告写入前做 secret 脱敏。
- 出现脱敏时写入 `filter_decisions`。
- 当前主要覆盖 API key/token/password 形态；后续要扩展为统一过滤器。

## 沙箱执行

默认 runtime 是 `container`。如果本地没有 Docker，开发和测试必须显式传 `--runtime local-fallback`。

沙箱记录字段包括：

- runtime、command、status、exit_code
- timeout_ms、output_limit_bytes、env_whitelist
- stdout_digest、stderr_digest
- duration_ms

失败、超时、deny/ask 都不会让整个 review 崩溃；它们进入 sandbox summary、metrics exception_counts 和 SQLite。

## Go 规则覆盖

当前 deterministic Skill 脚本和 Go 规则覆盖：

| rule_id | 类别 | 状态 |
|---------|------|------|
| `secret-leak` | 敏感信息泄漏 | ✅ |
| `panic-direct` | 错误处理 | ✅ |
| `todo-marker` | 可维护性 | ✅ |
| `missing-test-hint` | 测试缺失提示 | ✅ warning |
| `goroutine-leak` | goroutine 生命周期 | ✅ |
| `context-leak` | context 生命周期 | ✅ |
| `resource-leak` | 资源关闭 | ✅ |
| `db-lifecycle` | 数据库连接/事务生命周期 | ✅ |

## 存储

默认后端是 SQLite。当前 schema 包含：

- `review_tasks`
- `findings`
- `reports`
- `permission_decisions`
- `filter_decisions`
- `sandbox_runs`
- `artifacts`
- `metrics`

按 `task_id` 可以查询 task、findings、report、decisions、filter decisions、sandbox runs、artifacts、metrics。

## 报告

输出：

- `review_report.json`
- `review_report.md`

报告包含：

- findings / warnings
- severity counts
- human_review_items
- governance_summary
- sandbox_summary
- metrics
- artifacts
- recommendation

## 当前优先级

下一阶段不是重新写本地原型，而是在现有 framework-first 链路上补齐验收缺口：

1. 真实 container runtime E2E 或受环境控制的 integration test。
2. E2B runtime 入口或清晰的 unsupported 记录。
3. 官方 artifact/session/telemetry 能力的最小接入或明确 adapter 边界。
4. 更强 Permission/Filter 策略与 ask/needs_human_review 测试。
5. hidden/eval 评测脚本，验证高危检出率和误报率。

## 相关文档

- [framework-first-mvp.md](framework-first-mvp.md)
- [implementation-plan.md](implementation-plan.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [goal-prompt-framework-mvp.md](goal-prompt-framework-mvp.md)
