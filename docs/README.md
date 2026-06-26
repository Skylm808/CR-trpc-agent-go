# 文档索引

本目录包含 CR-trpc-agent-go 的架构、实现计划、数据契约与 Issue #2004 验收追踪文档。

## 核心文档

| 文档 | 说明 |
|------|------|
| [architecture.md](architecture.md) | 系统架构、组件划分、框架集成映射、mode 定义、安全边界 |
| [implementation-plan.md](implementation-plan.md) | 分阶段实现计划（M1–M5）、Definition of Done、验收对照 |
| [data-contract.md](data-contract.md) | 实体字段定义；Target Contract vs Current v0 实现状态 |

## Issue #2004 专项

| 文档 | 说明 |
|------|------|
| [issue-2004-traceability.md](issue-2004-traceability.md) | 9 项能力 + 8 条验收标准 → 组件/测试/状态追踪矩阵 |
| [fixtures-matrix.md](fixtures-matrix.md) | 每个 diff fixture 的预期 rule_id、severity、测试断言模式 |
| [design-summary.md](design-summary.md) | 300–500 字方案设计说明（Issue 交付物） |

## 阅读顺序建议

**首次了解项目：**

1. [design-summary.md](design-summary.md) — 快速把握整体设计
2. [architecture.md](architecture.md) — 组件与流程
3. [issue-2004-traceability.md](issue-2004-traceability.md) — 当前进度与缺口

**开始开发：**

1. [implementation-plan.md](implementation-plan.md) — 当前 Milestone 与任务清单
2. [fixtures-matrix.md](fixtures-matrix.md) — 规则实现的目标行为
3. [data-contract.md](data-contract.md) — 字段与持久化约束

## 状态图例

文档中统一使用以下状态标记：

| 标记 | 含义 |
|------|------|
| ✅ | 已完成 |
| 🔶 | 部分完成 / 进行中 |
| ⬜ | 待开始 |
