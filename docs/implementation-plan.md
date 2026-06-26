# 实现计划

本文档按 Issue #2004 验收优先级排列里程碑，而非严格的时间线顺序。每个 Phase 标注当前状态：`✅ 完成` / `🔶 部分` / `⬜ 待做`。

**纠偏原则：后续开发必须框架优先。** 本地 rule-only 原型只能作为迁移素材；第一版交付必须证明 `trpc-agent-go` 的 Skill、Permission、CodeExecutor、SQLite/Store、Filter、Telemetry 能串成一条可审计 CR 链路。

## 里程碑总览

```
M0  本地 rule-only 原型       🔶 可迁移，非最终主线
M1  trpc-agent-go 最小链路    ⬜ 当前最高优先级
M2  规则补全 + fixture 预期   🔶
M3  存储与报告对齐契约        ⬜
M4  验收与交付物              ⬜
```

---

## M0：本地 Rule-only 原型 🔶

**目标：** 作为迁移参考，无 API Key 可跑通 parse → rules → dedupe → redact → report。该阶段不能作为 Issue #2004 最终交付主线。

| 任务 | 状态 |
|------|------|
| Go module 与 CLI 入口（`cmd/review-agent/`） | ✅ |
| `--diff-file`、`--repo-path` 输入 | ✅ |
| unified diff 解析（文件、hunk、行号、package 提示） | ✅ |
| 确定性规则：secret-leak、panic-direct、todo-marker、missing-test-hint | ✅ |
| 去重（DedupeFindings）与脱敏（RedactSecrets） | ✅ |
| 生成 review_report.json / review_report.md | ✅ |
| fixture 端到端测试（报告文件存在） | ✅ |

---

## M1：trpc-agent-go 最小链路 ⬜

**目标：** 先用框架原语打通 `Skills + Permission + CodeExecutor + SQLite` 的最小闭环。完成后再继续扩展本地规则。

### Phase 1a：依赖与编排层

| 任务 | 状态 |
|------|------|
| 添加 `trpc-agent-go` 依赖，并记录具体 module path / version | ⬜ |
| 新增 `internal/agent` 或等价编排层，CLI 只调用 Agent，不直接调用本地 runner | ⬜ |
| 保留现有 parser / rules / report 作为 adapter 后端 | ⬜ |
| 增加 `--mode=rule-only/dry-run/sandbox/fake-model` 的真实分支 | ⬜ |

### Phase 1b：Skill 链路

| 任务 | 状态 |
|------|------|
| 通过 `tool/skill` 加载 `skills/code-review/SKILL.md` | ⬜ |
| 通过 `skill run` 执行 `skills/code-review/scripts/check.sh` | ⬜ |
| 脚本输出 JSON findings，映射为内部 `Finding` | ⬜ |
| Skill run 失败记录为 warning / exception，不导致任务崩溃 | ⬜ |

### Phase 1c：Permission + CodeExecutor

| 任务 | 状态 |
|------|------|
| 接入 `tool.PermissionPolicy` 或兼容 wrapper | ⬜ |
| 所有命令先生成 permission decision 并落库 | ⬜ |
| `deny` / `ask` / `needs_human_review` 不进入 executor | ⬜ |
| 默认使用 `codeexecutor/container` 执行 `go test` / `go vet` / 脚本 | ⬜ |
| E2B / Cube 作为可选 runtime | ⬜ |
| local runtime 仅通过显式 dev/test fallback 启用 | ⬜ |
| timeout、output limit、env whitelist、exit code、stdout/stderr digest 全部记录 | ⬜ |

### Phase 1d：最小持久化与报告

| 任务 | 状态 |
|------|------|
| SQLite 记录 task、permission decision、sandbox run、finding、report、metrics | ⬜ |
| 增加 filter decision 与 artifact 记录 | ⬜ |
| report 包含 findings、warnings、人审项、governance、sandbox、metrics、修复建议 | ⬜ |
| 至少 8 个 fixture 走完整框架链路并生成 JSON/Markdown | ⬜ |

---

## M2：规则补全 + Fixture 预期 🔶

**目标：** Issue 7 类规则至少覆盖 4 类（当前 4 类），补全至 6–7 类；8+ 公开样本有 deterministic 预期。

### Phase 2a：补全规则引擎

| rule_id | 类别 | fixture | 状态 |
|---------|------|---------|------|
| `secret-leak` | 敏感信息 | `secret.diff` | ✅ |
| `panic-direct` | 错误处理 | `panic.diff` | ✅ |
| `todo-marker` | 可维护性 | `todo.diff` | ✅ |
| `missing-test-hint` | 测试缺失 | `test-missing.diff`、`missing-test.diff` | ✅ warning |
| `goroutine-leak` | goroutine 泄漏 | `goroutine.diff` | ⬜ |
| `context-leak` | context 泄漏 | `context.diff` | ⬜ |
| `resource-leak` | 资源关闭 | `resource.diff` | ⬜ |
| `db-lifecycle` | DB 生命周期 | `db-lifecycle.diff` | ⬜ |

实现方式（首版）：基于 diff hunk 新增行的文本启发式，不必上 AST；后续可叠加 staticcheck 沙箱结果。

### Phase 2b：补全 fixture 与预期测试

| 任务 | 状态 |
|------|------|
| 现有 10 个 diff fixture | ✅ |
| 新增 `dedupe.diff`（同一行同一 rule 重复触发） | ⬜ |
| 新增 `sandbox-fail.diff`（触发沙箱超时/失败） | ⬜ |
| `fixtures-matrix.md` 预期表 | ✅ 文档已写 |
| 测试从「报告存在」升级为「断言 rule_id + severity」 | ⬜ |
| `rules.md` 与 engine rule_id 一一对应 | 🔶 部分 |

详见 [fixtures-matrix.md](fixtures-matrix.md)。

---

## M3：存储与报告对齐契约 🔶

**目标：** 数据库完整记录 task / sandbox run / permission decision / finding / artifact / report；报告包含 Issue 要求的全部摘要字段。

### Phase 3a：Storage interface

| 任务 | 状态 |
|------|------|
| 定义 `internal/storage/store.go` interface | ⬜ |
| SQLite 实现 task / finding / report / metrics | ✅ |
| SQLite 实现 permission_decisions | ✅ |
| SQLite 实现 sandbox_runs（扩展 runtime/args/timeout/digest 字段） | 🔶 字段不完整 |
| 新增 artifacts 表与 SaveArtifact / ArtifactsByTaskID | ⬜ |
| 新增 filter_decisions 表 | ⬜ |
| 按 task_id 查询全部关联记录 | 🔶 部分方法已有 |
| task_id 使用 UUID 而非硬编码 | ⬜ |

### Phase 3b：报告字段补全

| ReviewReport 字段 | 状态 |
|-------------------|------|
| findings / warnings | ✅ |
| severity 分布 | 🔶 JSON 有，Markdown 段待补 |
| governance_summary | ⬜ |
| sandbox_summary | ⬜ |
| human_review_items | ⬜ |
| metrics | 🔶 基础字段有 |
| artifacts | ⬜ |
| conclusion | ⬜ |

### Phase 3c：CLI mode 分支

| Mode | 状态 |
|------|------|
| `rule-only` | ✅ 默认 |
| `dry-run` | ⬜ 未分支 |
| `sandbox` | 🔶 RunChecks 标志存在但未暴露 CLI flag |
| `fake-model` | ⬜ 未实现 |

---

## M4：验收与交付物 ⬜

**目标：** 满足 Issue #2004 全部 Deliverables 与 8 条验收标准。

| 交付物 | 状态 |
|--------|------|
| Go 示例入口（`cmd/review-agent/`） | ✅ |
| `skills/code-review/SKILL.md` + rules + scripts | 🔶 骨架 |
| 数据库 schema + 存储实现 | 🔶 SQLite 有，interface 待抽象 |
| 8+ 测试样例 diff | ✅ 10 个，缺 dedupe / sandbox-fail |
| `review_report.json/md` 示例输出（`testdata/expected/`） | ⬜ |
| README 运行说明 | ✅ |
| 300–500 字方案设计说明 | ✅ [design-summary.md](design-summary.md) |
| Issue 追踪矩阵 | ✅ [issue-2004-traceability.md](issue-2004-traceability.md) |

### 验收标准对照

| # | 标准 | 当前 | 阻塞项 |
|---|------|------|--------|
| 1 | 8 条公开 diff 全部可运行并生成报告 | ✅ | — |
| 2 | 隐藏样本高危检出率 ≥ 80%，误报率 ≤ 15% | ⬜ | 缺评测脚本与规则补全 |
| 3 | DB 完整记录 task/sandbox/finding/report，可按 task_id 查询 | 🔶 | artifact、filter 表缺失 |
| 4 | 沙箱超时控制；失败不崩溃 | 🔶 | output limit 未实现 |
| 5 | 脱敏检出率 ≥ 95%，报告/DB 无明文密钥 | 🔶 | 需补脱敏计数与断言测试 |
| 6 | dry-run/fake-model 全流程 ≤ 2 分钟 | ✅ rule-only 已满足 | mode 分支待实现 |
| 7 | 高风险命令须先过 Filter/Permission | 🔶 | deny 有，ask/needs_human_review 链路未完整 |
| 8 | 报告含 findings 摘要、severity 统计、人工复核项、治理拦截、监控、沙箱摘要、修复建议 | 🔶 | 报告字段待补全 |

### 评测方法（待实现）

```
testdata/fixtures/          ← 公开样本（deterministic 预期）
testdata/hidden/            ← 隐藏样本（评测用，不提交或 CI 注入）
scripts/eval.sh             ← 输出 recall / precision / 耗时
```

---

## Definition of Done

- [ ] 所有公开 fixture 产出报告且 rule_id/severity 符合预期矩阵
- [ ] findings 结构化、去重、低置信度分流正确
- [ ] 存储 interface 抽象完成，SQLite 实现全部 entity
- [ ] 按 task_id 可查询 task / findings / report / metrics / decisions / sandbox runs / artifacts
- [ ] 沙箱失败、超时、deny 不崩溃 review，且写入 DB
- [ ] 报告与 DB 中无明文 API Key / token / password
- [ ] container/E2B 为默认沙箱 runtime（local 仅 dev fallback）
- [ ] dry-run / fake-model / rule-only 三种 mode 可无 API Key 验收
- [ ] 隐藏样本评测 recall ≥ 80%、precision ≥ 85%（误报 ≤ 15%）

## 相关文档

- [architecture.md](architecture.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [design-summary.md](design-summary.md)
