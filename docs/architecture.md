# CR-trpc-agent-go 架构设计

## 目标

在官方 `trpc-agent-go` 之上构建面向 Go 工程的自动代码评审 Agent。第一版要证明的不是“LLM 能评论 diff”，而是一条可验证、可落库、可审计的 CR 链路：

- 读取 unified diff、fixture 或本地 git 工作区变更。
- 通过 `tool/skill` 加载并运行 `skills/code-review`。
- 在 `fake-model` 模式下经过 LLM Review Provider 边界，当前只使用 deterministic fake provider。
- 高风险命令进入沙箱前经过 `tool.PermissionPolicy` 决策。
- 默认使用 `codeexecutor/container`，`local-fallback` 仅用于开发和测试。
- 结构化输出 findings、warnings、human review items、治理摘要、沙箱摘要、指标和报告产物。
- SQLite 记录 task、permission/filter decision、sandbox run、finding、artifact 引用、metrics、report。

## 设计原则

本仓库是 **trpc-agent-go 之上的应用示例**，不是框架 fork。本仓库负责 Go CR 业务逻辑、规则、报告、schema、fixture 和验收测试；官方框架负责 Skill、Tool、PermissionPolicy、CodeExecutor 等可复用原语。

当前是基于 trpc-agent-go Tool/Skill/CodeExecutor/workspaceexec/artifact 的 CLI Agent 原型，已接入模型审查 provider 边界但未绑定真实 LLM 厂商 SDK，尚未接入 Runner/Event，后续可演进。

当前代码已经接入：

- `trpc.group/trpc-go/trpc-agent-go v1.10.0`
- `trpc.group/trpc-go/trpc-agent-go/codeexecutor/container v1.10.0`
- `tool/skill` 的 load/run tool
- `tool/codeexec` 的 Go 检查执行入口
- `tool/workspaceexec` 的工作区内 Go 检查入口
- `tool.PermissionPolicy`
- `codeexecutor/container` 和显式 `codeexecutor/local` fallback
- `internal/agent.ModelReviewProvider` 边界和无 API Key 的 fake provider
- 早期 `internal/governance` / `internal/sandbox` 本地包装已删除，避免和官方治理、执行边界混淆。

仍未完成的框架侧增强：

- 真实 Docker container 端到端测试已加入 env gate，并已在 Docker Desktop 上通过。
- E2B/Cube runtime adapter。
- 官方 artifact service 默认用 inmemory 保存报告和诊断产物；SQLite 继续保留引用记录和查询。
- 官方 `session/sqlite` 尚未直接接入；当前使用本项目 SQLite 审计 store，后续接 Runner/Event 或多轮评审时再映射 session/history。
- 已接入官方 telemetry trace 边界和审查摘要属性；当前 SQLite metrics 表继续记录耗时、异常、severity 分布等可查询聚合指标。官方 metric exporter/OTLP dashboard 留作部署集成项。

## 系统流程

```text
CLI 输入（--diff-file / --file-list / --repo-path / --fixture）
  -> internal/agent.Agent
  -> trpc-agent-go/tool/skill skill_load(code-review)
  -> tool.PermissionPolicy 决策 scripts/check.sh
  -> trpc-agent-go/tool/skill skill_run(scripts/check.sh)
  -> fake-model 模式：redacted input -> ModelReviewProvider -> finding/warning merge
  -> 可选 sandbox 模式：优先 tool/workspaceexec 执行 go test / go vet / staticcheck，失败时退回 tool/codeexec
  -> 合并 Skill 输出，去重，脱敏，分流 warnings / human_review_items
  -> 生成 review_report.json 和 review_report.md
  -> SQLite 保存 task / decisions / runs / findings / artifact references / metrics / report
```

## 框架集成映射

| 官方模块 | 本仓库对应实现 | 当前状态 |
|---------|----------------|---------|
| Agent | `internal/agent.Agent` 是 CLI 编排器，负责输入、工具调用、报告和审计闭环 | ✅ 应用层 Agent 原型，未实现官方 `agent.Agent` 事件流接口 |
| Runner / Event | CLI 直接调用 `Agent.Run(ctx, Request)` | 🔶 尚未接入官方 Runner/Event；后续可演进为事件流和会话生命周期管理 |
| Tool | `skill_load`、`skill_run`、`execute_code` 都以 `tool.CallableTool` 形式持有 | ✅ 已使用官方工具抽象 |
| Skill | `skill.NewFSRepository` 加载 `skills/code-review`，`skill_run` 执行固定脚本 | ✅ 已接入官方 Skill 仓库和 Tool |
| Model Review Provider | `internal/agent.ModelReviewProvider` 接收脱敏 diff 摘要、input metadata、existing findings、sandbox/governance summary，输出复用 `review.Finding` | ✅ 当前仅 fake provider，无真实厂商 SDK/API Key |
| CodeExecutor | `codeexecutor/container` 默认执行，`codeexecutor/local` 仅显式 fallback | ✅ 使用官方执行器，Docker E2E 已通过 |
| PermissionPolicy | `internal/agent.defaultPermissionPolicy` 返回 `tool.PermissionPolicy` | ✅ 固定 allowlist，非 allow 不进入 executor |
| Session | SQLite 审计 store 记录 task、decision、run、finding、artifact、metrics、report | 🔶 尚未直接接官方 `session/sqlite`；当前不是官方 Session Service |
| Memory | 无长期用户记忆 | ⏳ 当前 CR MVP 不需要，后续如接多轮评审再评估 |
| Observability | 官方 telemetry trace span 记录 mode、runtime、输入类型、耗时、工具调用、model 调用/耗时/异常/finding 数、权限拦截、severity/exception 分布和结论；SQLite metrics 表保存可查询聚合指标 | 🔶 未启动官方 metric exporter；OTLP dashboard 属于后续部署集成 |
| Artifact | `review_report.json` / `review_report.md` / `review_diagnostics.json` 保存到本地，且同步写入官方 artifact service | ✅ 默认使用官方 inmemory service，SQLite 继续保留引用记录 |

## CLI Mode

| Mode | 行为 | 用途 |
|------|------|------|
| `rule-only` | 加载 Skill 并运行 deterministic 脚本，不依赖模型 | 默认测试路径 |
| `dry-run` | 加载 Skill，记录 skipped decision/run，不执行沙箱 | 验证治理与报告链路 |
| `sandbox` | 执行 Skill，并在有 `--repo-path` 时执行 Go 检查 | 接近生产路径 |
| `fake-model` | 执行 deterministic Skill 后进入 ModelReviewProvider 边界，默认 fake provider，不使用真实模型 key | 无 API Key 的模型路径替身 |

## LLM Review Provider

当前新增的是模型审查边界，不是真实模型接入。`ModelReviewProvider` 的输入只包含脱敏后的 diff summary、Go input metadata、已有 findings、sandbox summary 和 governance summary；raw diff secret 不允许进入 prompt。Provider 输出继续使用 `review.Finding` 字段：`severity`、`category`、`file`、`line`、`title`、`evidence`、`recommendation`、`confidence`、`source`、`rule_id`、`status`。

合并规则保持确定性：`source` 归一为 `model` 或 `fake_model`；高置信模型项进入 `findings`；低置信模型项降级为 `warnings` / `needs_human_review`；模型项和规则项按 `file + line + category + rule_id` 去重。模型失败不让 review 崩溃，会增加 `model_provider` exception，生成人工复核项，并继续输出报告和 SQLite 审计。

真实 OpenAI / Claude / Gemini provider、API Key 管理、Runner/Event、Session/Memory 和 E2B 都不属于当前阶段。

## 输入解析

当前支持：

- `--diff-file`：读取 unified diff。
- `--file-list`：读取文件路径列表，转换为新增文件 diff；相对路径优先按 `--repo-path` 解析。
- `--fixture` + `--fixtures-root`：读取公开测试样本。
- `--repo-path`：git 仓库使用 `git diff --unified=3`；普通目录转换为新增文件 diff。

待补强：

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

默认 runtime 是 `container`，默认容器镜像为 `golang:1.25-bookworm`。如果本地没有 Docker，开发和测试必须显式传 `--runtime local-fallback`。

container 模式下 Go 检查先经过 `tool.PermissionPolicy`，再通过官方 `tool/workspaceexec` 进入容器 workspace。Agent 记录的审计命令保持为 `go test ./...` / `go vet ./...`，容器内实际执行使用 `/usr/local/go/bin/go`，避免治理策略清理 PATH 后找不到 Go 工具链。

沙箱记录字段包括：

- runtime、command、status、exit_code
- timeout_ms、output_limit_bytes、env_whitelist
- stdout_digest、stderr_digest
- duration_ms

失败、超时、deny/ask 都不会让整个 review 崩溃；它们进入 sandbox summary、metrics exception_counts 和 SQLite。更细的安全边界、审计字段和测试证据见 [sandbox-safety.md](sandbox-safety.md)。

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

按 `task_id` 可以查询 task、findings、report、decisions、filter decisions、sandbox runs、artifacts、metrics；报告或 artifact 阶段失败时 task 会标记为 `failed` 便于回放。

## 报告

输出：

- `review_report.json`
- `review_report.md`
- `review_diagnostics.json`

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

1. 将真实模型 provider 挂到现有 `ModelReviewProvider` 边界，同时保持无 API Key 默认路径不变。
2. 在有 Docker daemon 的 CI/机器上执行 env-gated container E2E。
3. E2B runtime 入口或清晰的 unsupported 记录。
4. 官方 metric exporter、Session/Memory 和 Runner/Event 的接入条件评估。
5. hidden/eval 评测脚本，验证高危检出率和误报率，契约见 `docs/eval-matrix.md`。

## 相关文档

- [framework-first-mvp.md](framework-first-mvp.md)
- [implementation-plan.md](implementation-plan.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [sandbox-safety.md](sandbox-safety.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [upstream-example-migration.md](upstream-example-migration.md)
- [goal-prompt-framework-mvp.md](goal-prompt-framework-mvp.md)
