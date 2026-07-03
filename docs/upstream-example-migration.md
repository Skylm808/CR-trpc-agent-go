# 迁移到官方 examples 的准备说明

本文档用于后续把当前仓库迁移到 `trpc-group/trpc-agent-go` 官方仓库的 `examples` 目录。当前阶段不做大搬家，也不在官方仓库直接改代码；先保持独立仓库提交，等验收链路稳定后再发 PR。

## 建议目标路径

优先建议使用：

```text
examples/code_review_agent/
```

备选路径：

```text
examples/skills_code_review_agent/
```

官方 examples 目录中已有 `skill`、`skillrun`、`codeexecution`、`sandboxcodeexecution`、`runner`、`memory` 等示例。当前项目更像一个完整应用示例，不只是 Skill API 演示，因此 `examples/code_review_agent` 更清晰。

## 需要迁移的目录

| 当前路径 | 迁移用途 |
|----------|----------|
| `cmd/review-agent/` | 示例 CLI 入口 |
| `internal/agent/` | Tool / Skill / Permission / CodeExecutor 编排 |
| `internal/review/` | diff 解析、规则结果、脱敏和去重 |
| `internal/report/` | JSON / Markdown 报告生成 |
| `internal/storage/` | 审计 store 接口 |
| `internal/storage/sqlite/` | SQLite 默认实现 |
| `skills/code-review/` | CR Skill、规则文档和脚本 |
| `testdata/fixtures/` | 公开 diff 样本 |
| `scripts/acceptance.sh`、`scripts/eval.sh` | repo-neutral 验收和公开样本评测 |
| `docs/` 子集 | 架构、数据契约、验收追踪、迁移说明 |
| `examples/review_report.*` | 示例报告产物 |

## 不应迁移的内容

- 仓库级 `.github/workflows`，官方仓库是否接 CI 由维护者决定。
- 个人机器路径、临时数据库、`/private/tmp` 输出。
- hidden fixture 本体；只保留 external TSV / env var 接入契约。
- 独立仓库私有历史、实验性 prompt、未被 README 链接的本地草稿。

## go module 和 import 调整

迁入官方仓库后需要把当前 module import：

```text
github.com/Skylm808/CR-trpc-agent-go/...
```

调整为官方 examples 内部可用路径。可选方案：

1. 作为 `examples/code_review_agent` 独立 Go module，保留本示例自己的 `go.mod`，依赖 `trpc.group/trpc-go/trpc-agent-go`。
2. 合并到官方 `examples/go.mod`，把包路径改成 examples module 下的相对 import。

建议先采用独立 example module，便于隔离 SQLite、Docker E2E 和 fixture 评测脚本。

## 后续框架路线

当前仍保持 CLI Agent 原型，因为 issue 的核心是可验证 CR 链路：diff 输入、Skill 执行、沙箱检查、Permission 审计、结构化报告、SQLite 回放和验收脚本。以下模块适合后续按需接入：

| 官方模块 | 接入时机 |
|----------|----------|
| `model.Model` / Runner / Event | 下一阶段优先接入；用于把当前 LLM provider 边界纳入官方模型调用和事件流路线 |
| Session / Memory | 需要跨 PR 历史、长期经验复用、会话恢复时接入 |
| E2B / Cube | 需要远端隔离沙箱或云端 runner 时接入 |
| telemetry exporter | 进入服务化部署后，把官方 trace / metric 接到 OTLP dashboard |

## 迁移前检查清单

1. `GOCACHE=/private/tmp/cr-agent-gocache go test ./...`
2. `scripts/eval.sh`
3. `CR_AGENT_ACCEPTANCE_DOCKER=skip GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh`
4. Docker 可用时运行 container E2E。
5. `git diff --check`
6. 确认 README 不含个人路径或独立仓库专属说法。
7. 确认 docs 明确：SQLite 是审计 store，不是假装官方 Session Service。
