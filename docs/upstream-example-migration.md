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

## 最小迁移包

迁移时保持轻量，不把本仓库所有历史材料搬进官方 examples。建议只搬：

| 当前路径 | 迁移用途 |
|----------|----------|
| `examples/cr-agent/README.md` | 官方 example 的入口说明 |
| `examples/cr-agent/cr-agent.example.yaml` | 安全默认配置，不含密钥 |
| `examples/cr-agent/sample.diff` | 最小可运行输入 |
| `cmd/review-agent/` | 示例 CLI 入口 |
| `internal/agent/` | Tool / Skill / Permission / CodeExecutor / Runner 编排 |
| `internal/review/` | diff 解析、规则结果、脱敏和去重 |
| `internal/report/` | JSON / Markdown 报告生成 |
| `internal/storage/`、`internal/storage/sqlite/` | 审计 store 和 SQLite 默认实现 |
| `skills/code-review/` | CR Skill、规则文档和脚本 |
| `testdata/fixtures/` | 公开 diff 样本 |
| `scripts/acceptance.sh`、`scripts/eval.sh`、`scripts/llm_smoke.sh` | repo-neutral 验收、公开样本评测和 opt-in LLM smoke |
| `docs/architecture.md`、`docs/data-contract.md`、`docs/issue-2004-traceability.md`、`docs/eval-matrix.md`、`docs/sandbox-safety.md` | 只迁移会被 reviewer 用到的 contract 文档 |

## 不应迁移的内容

- 仓库级 `.github/workflows`，官方仓库是否接 CI 由维护者决定。
- 个人机器路径、临时数据库、`/private/tmp` 输出。
- hidden fixture 本体；只保留 external TSV / env var 接入契约。
- 独立仓库私有历史、实验性 prompt、未被 README 链接的本地草稿。
- 本地 `cr-agent.yaml`、`examples/review.db` 和任何真实 API key。
- 过长的阶段性 implementation plan；官方 PR 描述里保留当前状态和缺口即可。

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
| Runner / Event | 已有官方 `model.Model` adapter 和 `runner.Run` / `event.Event` adapter；迁移 examples 时保留这个主入口 |
| Session / Memory | 需要跨 PR 历史、长期经验复用、会话恢复时接入 |
| E2B / Cube | 当前只有 unsupported 审计入口；需要远端隔离沙箱或云端 runner 时替换为真实 adapter |
| telemetry exporter | 进入服务化部署后，把官方 trace / metric 接到 OTLP dashboard |

## 迁移前检查清单

1. `GOCACHE=/private/tmp/cr-agent-gocache go test ./...`
2. `scripts/eval.sh`
3. `bash -n scripts/llm_smoke.sh`
4. `CR_AGENT_ACCEPTANCE_DOCKER=skip GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh`
5. Docker 可用时运行 container E2E，并对比 `docker ps -a` 前后状态。
6. `git diff --check`
7. 确认 README 不含个人路径或独立仓库专属说法。
8. 确认 docs 明确：SQLite 是审计 store，不是假装官方 Session Service。
