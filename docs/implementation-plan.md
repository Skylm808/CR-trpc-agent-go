# 实现计划

本文档按 Issue #2004 验收优先级排列里程碑，而非严格的时间线顺序。每个 Phase 标注当前状态：`✅ 完成` / `🔶 部分` / `⬜ 待做`。

## 里程碑总览

```
M1  Rule-only 闭环          ✅ 主路径已通
M2  规则补全 + fixture 预期  🔶 进行中
M3  存储与报告对齐契约       🔶 部分完成
M4  trpc-agent-go 集成       ⬜ 待开始
M5  验收与交付物             ⬜ 待开始
```

---

## M1：Rule-only 闭环 ✅

**目标：** 无 API Key 可跑通 parse → rules → dedupe → redact → report。

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

## M4：trpc-agent-go 集成 ⬜

**目标：** Skill run、PermissionPolicy、container/E2B 沙箱接入；local runtime 仅作 dev fallback。

| 任务 | 状态 |
|------|------|
| 添加 trpc-agent-go 依赖 | ⬜ |
| Skill load → 读 rules/scripts | ⬜ |
| skill run 触发 check.sh / go vet 包装 | ⬜ |
| 替换自研 Policy 为 PermissionPolicy wrapper | ⬜ |
| container runtime 作为默认沙箱 | ⬜ |
| E2B runtime 作为可选沙箱 | ⬜ |
| 沙箱 output size limit | ⬜ |
| 沙箱 env whitelist | ⬜ |
| 沙箱失败/超时不崩溃 review | 🔶 基础逻辑有 |
| Filter 层实现 | ⬜ |
| Telemetry hook 接入 | ⬜ |

**Runtime 选择：**

- 生产/CI：`CR_SANDBOX_RUNTIME=container`（默认）
- 本地开发：`CR_SANDBOX_RUNTIME=local`（显式启用）
- 单元测试：mock 或 local + 短 timeout

---

## M5：验收与交付物 ⬜

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
