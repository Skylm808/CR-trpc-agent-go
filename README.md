# CR-trpc-agent-go

基于 [trpc-agent-go](https://github.com/trpc-group/trpc-agent-go) 构建的 **Go 代码自动审查 Agent** 原型。项目目标是：接收 unified diff、PR patch 或本地工作区变更，通过确定性规则与可选沙箱检查，输出结构化审查结论与可读报告，并将任务、决策与指标持久化。

> 本项目是框架之上的**应用层实现**，而非框架 fork。审查工作流、规则引擎、diff 解析、持久化 schema、报告生成与测试夹具均在本仓库维护；`trpc-agent-go` 提供 Skill 加载、工作区执行、会话/存储模式等可复用原语（后续逐步接入）。

---

## 当前进度

对照 [docs/implementation-plan.md](docs/implementation-plan.md) 的七个阶段，当前状态如下：

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | Go 模块、CLI 入口、确定性 rule-only 流水线、fixture 加载 | ✅ 已完成 |
| Phase 2 | unified diff 解析、Go 包名推断、规则检查、去重与脱敏 | 🔶 部分完成 |
| Phase 3 | SQLite schema、任务/发现项/报告/指标持久化 | ✅ 已完成 |
| Phase 4 | 治理策略、沙箱执行、超时与输出限制 | 🔶 基础版完成 |
| Phase 5 | `skills/code-review` Skill 打包 | 🔶 骨架完成 |
| Phase 6 | JSON/Markdown 报告、示例 diff | ✅ 已完成 |
| Phase 7 | 单元测试、端到端 fixture 审查 | 🔶 进行中 |

### 已实现能力

- **输入**：`--diff-file` 指定 unified diff，或 `--repo-path` 从 Git 仓库 / 普通目录生成 diff
- **Diff 解析**：提取文件、hunk、行号映射与 Go 包名提示
- **规则引擎**（确定性、无需模型 API Key）：
  - 密钥/凭证字面量泄露（`secret-leak`）
  - 直接 `panic` 路径（`panic-direct`）
  - TODO/FIXME 标记（`todo-marker`）
  - 新函数缺少测试提示（`missing-test-hint`，warning 级别）
- **去重与脱敏**：按 file + line + category + rule_id 去重；敏感内容写入报告前脱敏
- **报告**：生成 `review_report.json` 与 `review_report.md`
- **持久化**（可选）：`--sqlite` 写入任务、发现项、指标、治理决策与沙箱运行记录
- **治理层**：拦截高风险命令（如 `rm -rf`、`sudo`），允许 `go test` / `go vet` / `staticcheck`
- **沙箱**（可选）：本地命令执行 + 超时保护；失败不阻断报告生成
- **测试夹具**：`testdata/fixtures/` 下 10 个 diff 样例 + 端到端 fixture 测试

### 待完善 / 后续接入

- **规则扩展**：goroutine/context 泄漏、资源生命周期、DB 连接/事务、更细粒度 error handling（fixture 已预留，规则待实现）
- **沙箱运行时**：容器 / E2B 等隔离环境（当前为本地 fallback）
- **trpc-agent-go 深度集成**：Skill 加载、工作区执行、telemetry hooks
- **运行模式**：`dry-run`、`sandbox`、`fake-model` 等 CLI mode 完整接线
- **Artifact 持久化**与报告中的 governance/sandbox 摘要完善

第一个里程碑（[docs/architecture.md](docs/architecture.md)）——**无需模型 API Key 的确定性 rule-only 流水线**——已打通主路径。

---

## 系统架构

```
CLI 输入
  ↓
Diff 解析器 ──→ 文件 / hunk / 行号 / 包名
  ↓
Skill 层（skills/code-review/）
  ↓
治理层（allow / deny / ask / needs_human_review）
  ↓
沙箱执行器（可选，经策略批准）
  ↓
规则引擎 ──→ 去重 ──→ 脱敏
  ↓
报告生成（JSON + Markdown）
  ↓
SQLite 持久化（可选）
```

安全边界（详见 [docs/architecture.md](docs/architecture.md)）：

- 禁止无限制 shell 执行
- 沙箱命令须先过策略审批
- 日志、产物与报告中不得出现明文密钥
- 输出大小、运行时间与 artifact 数量均有上限

---

## 项目结构

```
cmd/review-agent/     CLI 入口与编排
internal/
  review/             diff 解析、规则引擎、去重、脱敏
  report/             JSON / Markdown 报告
  governance/         命令权限策略
  sandbox/            受控命令执行
  storage/sqlite/     SQLite 持久化
skills/code-review/   审查 Skill（SKILL.md、规则文档、脚本）
testdata/fixtures/    示例 diff 与端到端测试输入
docs/                 架构、实现计划、数据契约
```

---

## 快速开始

### 环境要求

- Go 1.25+

### 运行测试

```bash
go test ./...
```

针对单个 fixture 生成报告：

```bash
go run ./cmd/review-agent \
  --diff-file testdata/fixtures/secret.diff \
  --output-dir /tmp/review-out
```

从 Git 仓库读取工作区 diff：

```bash
go run ./cmd/review-agent \
  --repo-path /path/to/your/repo \
  --output-dir /tmp/review-out
```

启用 SQLite 持久化：

```bash
go run ./cmd/review-agent \
  --diff-file testdata/fixtures/panic.diff \
  --sqlite /tmp/review.db \
  --output-dir /tmp/review-out
```

### CLI 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--diff-file` | — | unified diff 文件路径 |
| `--repo-path` | — | Git 仓库或普通目录（与 diff-file 二选一） |
| `--output-dir` | `.` | 报告输出目录 |
| `--mode` | `rule-only` | 审查模式 |
| `--sqlite` | — | SQLite 数据库路径（可选） |

---

## 测试夹具

`testdata/fixtures/` 覆盖多种审查场景：

| 文件 | 场景 |
|------|------|
| `safe.diff` | 干净的 Go 变更 |
| `secret.diff` | 潜在密钥泄露 |
| `panic.diff` | 直接 panic 路径 |
| `todo.diff` | TODO/FIXME 标记 |
| `test-missing.diff` | 缺少测试提示 |
| `goroutine.diff` | goroutine 相关（规则待扩展） |
| `context.diff` | context 相关（规则待扩展） |
| `resource.diff` | 资源生命周期（规则待扩展） |
| `db-lifecycle.diff` | 数据库生命周期（规则待扩展） |
| `missing-test.diff` | 测试覆盖提示 |

---

## 数据契约

审查流程中的核心实体定义见 [docs/data-contract.md](docs/data-contract.md)，包括：

- `ReviewTask` / `ReviewInput` — 任务与归一化输入
- `ParsedFile` / `ParsedHunk` — 解析后的文件与 hunk
- `Finding` — 发现项（severity、category、confidence、dedupe_key 等）
- `PermissionDecision` / `SandboxRun` — 治理与沙箱审计
- `MetricsSummary` / `ReviewReport` — 指标与最终报告

持久化规则：每个 task 对应唯一任务行；发现项、决策、运行记录均关联 task_id；报告可从存储行重建；敏感字面量在写入前脱敏。

---

## 后续规划

按 [docs/implementation-plan.md](docs/implementation-plan.md) 的里程碑顺序：

1. **确定性 rule-only 流水线** — 主路径已可用，持续补充规则类别
2. **持久化与报告** — 基础完成，完善 artifact 与报告摘要字段
3. **治理与沙箱** — 接入容器/E2B 运行时，强化输出限制与失败处理
4. **Skill 打包** — 丰富 `skills/code-review/` 规则文档与可执行脚本
5. **夹具与验证** — 扩展规则后更新 fixture 预期，补齐集成测试

**Definition of Done**（完成标准）：

- 所有示例 diff 均能产出报告
- 发现项结构化且去重
- 存储可按 task id 查询
- 沙箱失败不导致审查崩溃
- 报告与存储中无明文密钥泄露

---

## 文档索引

| 文档 | 说明 |
|------|------|
| [docs/architecture.md](docs/architecture.md) | 系统目标、组件划分、安全边界、首版里程碑 |
| [docs/implementation-plan.md](docs/implementation-plan.md) | 分阶段实现计划与完成标准 |
| [docs/data-contract.md](docs/data-contract.md) | 实体字段、枚举值与持久化规则 |
| [skills/code-review/SKILL.md](skills/code-review/SKILL.md) | 审查 Skill 使用说明 |
| [testdata/fixtures/README.md](testdata/fixtures/README.md) | 夹具场景说明 |

---

## License

见仓库根目录 LICENSE 文件（如有）。
