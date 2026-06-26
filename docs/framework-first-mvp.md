# Framework-first MVP

本文档定义下一轮实现目标：第一个小版本必须基于 `trpc-agent-go` 现有能力串起 **Skills + 沙箱 + 数据库存储**，而不是继续扩展纯本地 CR 原型。

## 纠偏结论

Issue #2004 的核心难点不是“写一个 diff 规则扫描器”，而是验证 `trpc-agent-go` 的工程编排能力：

- `tool/skill` 负责加载和运行 `skills/code-review`
- `tool/workspaceexec` 或 `tool/codeexec` 负责在 workspace 中执行检查脚本
- `codeexecutor/container` 或 `codeexecutor/e2b` 负责隔离执行 `go test`、`go vet`、自定义规则脚本
- `tool.PermissionPolicy` 或兼容 wrapper 负责命令执行前的 allow / deny / ask / needs_human_review 决策
- `session/sqlite`、SQL schema 或兼容 Store 负责记录 task、decision、sandbox run、finding、artifact、report
- Filter 和 telemetry 负责脱敏、拦截记录、耗时、异常与 severity 分布

当前仓库中已有的 diff parser、deterministic rules、report、SQLite schema 只能作为业务逻辑和测试夹具的迁移基础。它们不能替代框架集成，也不能作为最终验收主线。

## 第一个小版本范围

第一个 framework-first MVP 只做一条最小但完整的链路：

```text
CLI 输入
  -> trpc-agent-go Skill 加载 code-review
  -> 解析 diff / repo 变更
  -> PermissionPolicy 决策 go test / go vet / scripts/check.sh
  -> codeexecutor/container 执行，local 仅 test/dev fallback
  -> 合并 Skill 脚本输出 + deterministic rules
  -> Filter 脱敏与降噪
  -> SQLite 持久化
  -> review_report.json / review_report.md
```

## 必须完成的能力

| 能力 | 第一版要求 |
|------|-----------|
| Skill | `skills/code-review/SKILL.md`、`rules.md`、`scripts/check.sh` 由 `tool/skill` 加载和运行 |
| 沙箱 | 默认 runtime 为 container；E2B 可选；local 只能通过显式 dev/test fallback 启用 |
| Permission | 所有高风险命令先经过 `tool.PermissionPolicy` 或兼容 wrapper；非 allow 不执行 |
| 输入 | 支持 `--diff-file`、`--repo-path`、fixture；提取文件、hunk、候选行号、Go package |
| 规则 | 至少覆盖 secret、panic/error handling、goroutine/context/resource/db/test missing 中 4 类 |
| 存储 | SQLite 保存 task、permission/filter decision、sandbox run、finding、artifact、report、metrics |
| 报告 | JSON/Markdown 包含 findings、warnings、人审项、severity 统计、governance、sandbox、metrics、建议 |
| 模式 | `rule-only`、`dry-run`、`sandbox`、`fake-model` 都能无真实模型 Key 验证 |

## 实现顺序

1. **依赖与适配层**：添加 `trpc-agent-go` 依赖；建立 `internal/agent` 编排层，禁止 CLI 直接调用本地 sandbox。
2. **Skill 链路**：用框架 Skill API 加载 `skills/code-review`，运行 `scripts/check.sh`，把脚本 JSON 输出映射为 findings。
3. **Permission 链路**：用 `tool.PermissionPolicy` 或 wrapper 统一执行决策，并把 decision 写入 DB。
4. **沙箱链路**：用 `codeexecutor/container` 作为默认 executor，记录 timeout、output limit、env whitelist、exit code、stdout/stderr digest。
5. **存储链路**：抽 `storage.Store` interface，补 `filter_decisions`、`artifacts`、扩展 `sandbox_runs`。
6. **报告链路**：补齐 Issue 验收要求的所有摘要段。
7. **fixture 验证**：8+ diff 样本必须跑完整链路；dry-run/fake-model 不需要真实 API Key。

## 非目标

- 不继续把本地 `internal/sandbox` 扩展成生产沙箱。
- 不用纯文本 LLM 评论替代 Skill / sandbox / DB 链路。
- 不把 local runtime 当默认生产方案。
- 不先追求复杂 AST 规则；第一版优先打通框架链路和审计闭环。
