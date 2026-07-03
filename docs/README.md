# 文档索引

本目录包含 CR-trpc-agent-go 的架构、当前 roadmap、数据契约、评测和 Issue #2004 验收追踪文档。阶段性 prompt 和已合并的 MVP 边界文档已删除，避免索引继续指向过期状态。

## 核心文档

| 文档 | 说明 |
|------|------|
| [architecture.md](architecture.md) | 系统架构、组件划分、框架集成映射、mode 定义、官方 model/Runner/Event 适配路径 |
| [implementation-plan.md](implementation-plan.md) | 当前真实 roadmap、优先级、验收标准和验证命令 |
| [data-contract.md](data-contract.md) | 实体字段、SQLite schema、按 task_id 查询契约 |
| [sandbox-safety.md](sandbox-safety.md) | 沙箱执行、Permission、输出限制、脱敏和 artifact cap 安全边界矩阵 |
| [ci.md](ci.md) | 本地/CI acceptance 脚本、Docker E2E 开关和 hidden sample 接入 |
| [eval-matrix.md](eval-matrix.md) | 公开 fixture 评测和外部 hidden matrix 输入契约 |

## Issue #2004 专项

| 文档 | 说明 |
|------|------|
| [issue-2004-traceability.md](issue-2004-traceability.md) | 9 项能力、验收命令、SQLite 回放、剩余缺口和状态追踪矩阵 |
| [fixtures-matrix.md](fixtures-matrix.md) | 每个 diff fixture 的预期 rule_id、severity、测试断言模式 |
| [upstream-example-migration.md](upstream-example-migration.md) | 后续迁移到官方 trpc-agent-go/examples 的目录和边界准备 |

## 阅读顺序建议

**首次了解项目：**

1. [architecture.md](architecture.md) — 组件与流程
2. [issue-2004-traceability.md](issue-2004-traceability.md) — 当前进度与缺口
3. [sandbox-safety.md](sandbox-safety.md) — 沙箱和内容安全边界
4. [ci.md](ci.md) — CI 与隐藏样本评测闭环

**开始开发：**

1. [implementation-plan.md](implementation-plan.md) — 当前优先级、影响文件、验收标准和验证命令
2. [architecture.md](architecture.md) — 确认不要绕开 trpc-agent-go 主线
3. [fixtures-matrix.md](fixtures-matrix.md) — 规则实现的目标行为
4. [eval-matrix.md](eval-matrix.md) — hidden matrix 注入和评测口径
5. [data-contract.md](data-contract.md) — 字段与持久化约束
6. [upstream-example-migration.md](upstream-example-migration.md) — 准备迁移到官方 examples

## 状态图例

文档中统一使用以下状态标记：

| 标记 | 含义 |
|------|------|
| ✅ | 已完成 |
| 🔶 | 部分完成 / 进行中 |
| ⬜ | 待开始 |
