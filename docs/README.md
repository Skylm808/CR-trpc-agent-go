# 文档索引

本目录只保留 CR-trpc-agent-go 的长期 contract 文档：架构、Issue #2004 验收追踪、数据契约、安全边界、CI/评测、fixture 矩阵和 upstream examples 迁移。阶段性 roadmap、prompt 和重复评测说明已删除，避免索引继续指向过期状态。

## 核心文档

| 文档 | 说明 |
|------|------|
| [architecture.md](architecture.md) | 系统架构、项目自有分层、框架集成映射、mode 定义、官方 model/Runner/Event 适配路径 |
| [issue-2004-traceability.md](issue-2004-traceability.md) | 9 项能力、验收命令、SQLite 回放、剩余缺口和状态追踪矩阵 |
| [data-contract.md](data-contract.md) | 实体字段、SQLite schema、按 task_id 查询契约 |
| [sandbox-safety.md](sandbox-safety.md) | 沙箱执行、Permission、输出限制、脱敏和 artifact cap 安全边界矩阵 |
| [ci.md](ci.md) | 本地/CI acceptance 脚本、Docker E2E 开关、holdout 和 hidden-like matrix 接入 |

## Issue #2004 专项

| 文档 | 说明 |
|------|------|
| [fixtures-matrix.md](fixtures-matrix.md) | public / holdout diff fixture 的预期 rule_id、severity、测试断言模式 |
| [upstream-example-migration.md](upstream-example-migration.md) | 后续迁移到官方 trpc-agent-go/examples 的目录和边界准备 |

## 阅读顺序建议

**首次了解项目：**

1. [architecture.md](architecture.md) — 组件与流程
2. [issue-2004-traceability.md](issue-2004-traceability.md) — 当前进度与缺口
3. [sandbox-safety.md](sandbox-safety.md) — 沙箱和内容安全边界
4. [ci.md](ci.md) — CI 与 hidden-like 样本评测闭环

**开始开发：**

1. [architecture.md](architecture.md) — 确认不要绕开 trpc-agent-go 主线
2. [fixtures-matrix.md](fixtures-matrix.md) — 规则实现的目标行为
3. [ci.md](ci.md) — public / holdout / external hidden matrix 注入和评测口径
4. [data-contract.md](data-contract.md) — 字段与持久化约束
5. [upstream-example-migration.md](upstream-example-migration.md) — 准备迁移到官方 examples
