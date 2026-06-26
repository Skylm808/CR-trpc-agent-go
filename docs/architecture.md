# CR-trpc-agent-go 架构设计

## 目标

在 `trpc-agent-go` 之上构建面向 Go 工程的自动代码评审 Agent，能够：

- 接收 unified diff、PR patch、文件路径列表或本地工作区变更
- 加载 code-review Skill 及其规则脚本
- 在沙箱执行前经过治理策略审批
- 在隔离 runtime 中可选执行静态检查（go test / go vet / staticcheck）
- 输出结构化 findings 与人类可读报告
- 将任务、决策、发现项、产物与监控指标持久化

## 设计原则

本项目是**框架之上的应用**，不是框架 fork。本仓库拥有：

- 审查工作流编排
- 规则引擎与 diff 解析
- 持久化 schema 与存储实现
- 报告生成
- fixture 与验收测试

`trpc-agent-go` 提供可复用原语：Skill 加载与执行、workspace/host/code 执行、container/E2B 沙箱、Session/Memory/SQL 存储模式、PermissionPolicy、Filter 与 Telemetry。

**关键约束：第一个可交付版本必须以 `trpc-agent-go` 为主线。** 现有本地 diff parser、规则、报告、SQLite 代码只作为业务逻辑和测试夹具的迁移基础；不能继续把纯本地 runner / policy / storage 当成最终交付主体。Issue #2004 验收的是 Skills、沙箱执行、治理策略、数据库、监控审计和安全边界组成的框架化链路。

## 系统流程

```
CLI 输入（--diff-file / --repo-path / fixture）
  ↓
Diff 解析器 → 文件、hunk、行号、Go package 提示
  ↓
trpc-agent-go Skill 层（tool/skill）→ skill load / skill run code-review
  ↓
Filter 层 → 输入/输出/content 是否允许进入报告或落库
  ↓
Permission 层（tool.PermissionPolicy）→ 命令是否允许进入沙箱
  ↓
CodeExecutor（container / E2B，local 仅 dev fallback）
  ↓
规则引擎 → 合并 diff 启发式 + 沙箱静态检查结果
  ↓
去重与降噪 → 低置信度进入 warnings / needs_human_review
  ↓
脱敏 → 报告与存储写入前替换敏感字面量
  ↓
报告生成（review_report.json + review_report.md）
  ↓
持久化（task / decision / sandbox run / finding / artifact / metrics / report）
```

## 框架集成映射

| 本仓库组件 | trpc-agent-go 对应能力 | 当前状态 |
|-----------|----------------------|---------|
| Skill 加载与规则 | `tool/skill`（skill load / skill run） | ⬜ 框架未接入；仅 Skill 目录骨架已有 |
| 工作区命令 | `tool/workspaceexec` | ⬜ 待接入 |
| 主机命令 | `tool/hostexec` | ⬜ 待接入 |
| 沙箱执行 | `codeexecutor/container`、`codeexecutor/e2b` | ⬜ 框架未接入；本地 runner 仅作迁移参考 |
| 命令治理 | `tool.PermissionPolicy` | ⬜ 框架未接入；自研 Policy 仅作兼容参考 |
| 内容过滤 | Filter | ⬜ 待实现 |
| 持久化 | `session/sqlite` 或兼容 `storage.Store` interface | 🔶 SQLite 基础实现已有，需记录框架事件 |
| 遥测 | telemetry hooks | ⬜ 框架 hook 未接入；Metrics 字段仅作参考 |
| 产物 | artifact | ⬜ 表结构与写入待实现 |

## 第一个 Framework-first 小版本

详见 [framework-first-mvp.md](framework-first-mvp.md)。本小版本的完成条件不是“本地规则能跑”，而是：

- CLI 通过 `trpc-agent-go` Skill API 加载并运行 `skills/code-review`
- `go test` / `go vet` / `scripts/check.sh` 的执行先经过 `tool.PermissionPolicy` 或兼容 wrapper
- 默认沙箱 runtime 是 `codeexecutor/container`；E2B 可选；local 仅显式 dev/test fallback
- sandbox run、permission decision、filter decision、artifact、finding、metrics、report 全部写入 SQLite 或兼容 Store
- `rule-only`、`dry-run`、`sandbox`、`fake-model` 在无真实模型 Key 时仍可验证完整链路

## 核心组件

### CLI

负责参数解析、mode 选择与流水线编排。

**支持的 mode：**

| Mode | 行为 | 用途 |
|------|------|------|
| `rule-only` | diff 解析 + 确定性规则，不调用模型、不跑沙箱 | 默认；fixture 测试；无 API Key 验收 |
| `dry-run` | 完整编排（Skill 加载、Permission 决策），但不执行沙箱、可选不落库 | 验证链路而不产生副作用 |
| `sandbox` | 规则 + 经 Permission 批准的沙箱检查（go test / go vet / staticcheck） | 接近生产路径 |
| `fake-model` | 模拟 LLM 审查路径，使用 stub 响应，验证全链路 | 无真实模型 Key 时的集成测试 |

### 输入解析器

解析：

- unified diff 文本
- 文件路径列表（待实现）
- git 工作区变更（`--repo-path` + `git diff`）

输出：归一化的 `ParsedFile`、`ParsedHunk`、候选行号与 Go package 提示。

### Skill 层

`skills/code-review/` 包含：

- `SKILL.md` — Skill 入口与使用说明
- `rules.md` — 规则文档（与 engine rule_id 一一对应）
- `scripts/` — 沙箱可执行检查脚本（如 `check.sh`、go vet 包装）

Skill 必须通过 `trpc-agent-go` 的 `tool/skill` 能力加载，`skill run` 触发脚本；规则引擎与 Skill 脚本结果合并后去重。CLI 不应直接绕过 Skill 目录调用脚本。

### 治理层：Filter 与 Permission

两层职责分离：

| 层 | 职责 | 决策对象 | 典型动作 |
|----|------|---------|---------|
| **Filter** | 内容/输入/输出过滤 | diff 片段、报告文本、artifact | 拦截含明文密钥的内容进入报告或 DB |
| **Permission** | 命令执行授权 | shell / go test / staticcheck 等 | allow / deny / ask / needs_human_review |

**关键约束：** `deny`、`ask`、`needs_human_review` 的命令**不得**进入沙箱 executor；所有决策必须写入 `permission_decisions` 表供审计。

### 沙箱执行器

**Runtime 选择策略：**

| 环境 | 默认 runtime | 说明 |
|------|-------------|------|
| 生产 / CI | `container` 或 `e2b` | Issue 验收要求；local 不能作为默认 |
| 本地开发 | `local`（仅 fallback） | 通过 `CR_SANDBOX_RUNTIME=local` 或 test tag 启用 |
| 单元测试 | `local` 或 mock | 验证 timeout / deny / failure 不崩溃 |

**控制项：**

- 超时（timeout_ms）
- 输出大小限制（output_limit_bytes）
- 环境变量白名单（env_whitelist）
- artifact 数量上限（artifact cap）
- 敏感信息脱敏（stdout/stderr digest 写入 DB，明文不落库）
- 失败记录（status=timeout / failed / denied，不导致整个 review 崩溃）

### 规则引擎

面向 Go 代码评审的确定性规则，至少覆盖 Issue 要求的 4 类（目标覆盖 7 类）：

| 类别 | rule_id | 当前状态 |
|------|---------|---------|
| 敏感信息泄漏 | `secret-leak` | ✅ |
| 错误处理（直接 panic） | `panic-direct` | ✅ |
| 可维护性（TODO/FIXME） | `todo-marker` | ✅ |
| 测试缺失提示 | `missing-test-hint` | ✅（warning 级别） |
| goroutine 泄漏 | `goroutine-leak` | ⬜ fixture 已有，规则待实现 |
| context 泄漏 | `context-leak` | ⬜ fixture 已有，规则待实现 |
| 资源关闭生命周期 | `resource-leak` | ⬜ fixture 已有，规则待实现 |
| 数据库连接/事务生命周期 | `db-lifecycle` | ⬜ fixture 已有，规则待实现 |

低置信度问题进入 `warnings` 或 `needs_human_review`，不混入高置信 `findings`。

### 去重器（Deduper）

按以下维度归一化并去重：

- file
- line
- category
- rule_id

同一文件同一行同一 rule_id 只保留第一条 finding。

### 存储

持久化实体见 [data-contract.md](data-contract.md)。

- 默认后端：SQLite（`internal/storage/sqlite/`）
- 接口：`storage.Store`（待抽象，保留切换 SQL 后端空间）
- 查询：按 `task_id` 加载 task、findings、report、metrics、decisions、sandbox runs、artifacts

### 报告

输出：

- `review_report.json` — 结构化 JSON
- `review_report.md` — 人类可读 Markdown

报告必须包含（Issue 验收标准 8）：

- findings 摘要与 severity 分布
- warnings 与人工复核项（human_review_items）
- 治理拦截摘要（governance_summary）
- 沙箱执行摘要（sandbox_summary）
- 监控指标（metrics）
- 可执行修复建议（recommendation）

## 安全边界

- 禁止无限制 shell 执行
- 沙箱命令必须先过 Permission 决策
- Filter 拦截的敏感内容不得进入报告或 DB 明文
- 输出捕获有大小上限，运行有超时上限
- artifact 数量有上限
- 沙箱失败、超时、deny 均记录但不崩溃整个 review 任务

## 里程碑

| 里程碑 | 内容 | 状态 |
|--------|------|------|
| M0 | 本地 rule-only 原型（迁移参考，不是最终主线） | 🔶 可运行但未接框架 |
| M1 | trpc-agent-go Skill + Permission + sandbox 最小链路 | ⬜ 当前最高优先级 |
| M2 | SQLite / Store 对齐 task、decision、run、artifact、finding、report | ⬜ |
| M3 | Go CR 规则补全 + fixture 预期矩阵 | 🔶 进行中 |
| M4 | 报告/监控/验收（severity、governance、sandbox、telemetry） | ⬜ |

M1 必须在无真实模型 API Key 的情况下可完整运行，并且必须经过 `trpc-agent-go` 的 Skill / Permission / CodeExecutor 适配层。

## 相关文档

- [implementation-plan.md](implementation-plan.md) — 分阶段实现计划
- [framework-first-mvp.md](framework-first-mvp.md) — 框架优先的小版本边界
- [data-contract.md](data-contract.md) — 实体字段与持久化规则
- [issue-2004-traceability.md](issue-2004-traceability.md) — Issue 需求追踪矩阵
- [fixtures-matrix.md](fixtures-matrix.md) — 测试夹具预期行为
- [design-summary.md](design-summary.md) — 300–500 字方案设计说明
