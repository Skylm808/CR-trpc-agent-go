# 迁移到官方 examples 的准备说明

本文档用于后续把当前仓库迁移到 `trpc-group/trpc-agent-go` 官方仓库的 `examples` 目录。当前阶段不做大搬家，也不在官方仓库直接改代码；先保持独立仓库提交，等验收链路稳定后再发 PR。当前分层也不会照抄 upstream PR #2121 的 `inputsource` / `sandboxrun` / `safetywrap` 命名，而是保留本项目自己的 `execution` / `approval` / `llm` 语义。

## 建议目标路径

优先建议使用：

```text
examples/cr-agent/
```

官方 examples 目录中已有 `skill`、`skillrun`、`codeexecution`、`sandboxcodeexecution`、`runner`、`memory` 等示例。当前项目更像一个完整应用示例，不只是 Skill API 演示。按当前 issue / 本地样例命名，`examples/cr-agent` 最贴近用户入口，也避免再引入一层新命名。

## 最小迁移包

迁移时保持轻量，不把本仓库所有历史材料搬进官方 examples。建议只搬：

| 当前路径 | 迁移用途 |
|----------|----------|
| `examples/cr-agent/README.md` -> `README.md` | 官方 example 的入口说明 |
| `examples/cr-agent/cr-agent.example.yaml` -> `cr-agent.example.yaml` | 安全默认配置，不含密钥 |
| `examples/cr-agent/sample.diff` -> `sample.diff` | 最小可运行输入 |
| `cmd/review-agent/` | 示例 CLI 入口 |
| `internal/agent/` | Tool / Skill / Runner/Event 编排、report/SQLite/artifact 串联 |
| `internal/execution/` | container/local/e2b-unsupported runtime、测试专用 fake-execution seam、workspace Go checks、sandbox env allowlist |
| `internal/approval/` | PermissionPolicy 和允许执行命令清单 |
| `internal/llm/` | fake/http/OpenAI-compatible/DeepSeek provider、official model adapter、model finding merge |
| `internal/review/` | diff 解析、规则结果、脱敏和去重 |
| `internal/report/` | JSON / Markdown 报告生成 |
| `internal/storage/`、`internal/storage/sqlite/` | 审计 store 和 SQLite 默认实现 |
| `skills/code-review/` | CR Skill、规则文档和脚本 |
| `testdata/fixtures/` | 公开 diff 样本 |
| `scripts/acceptance.sh`、`scripts/eval.sh`、`scripts/holdout_eval.sh`、`scripts/hidden_matrix_smoke.sh`、`scripts/llm_smoke.sh`、`scripts/repo_llm_smoke.sh`、`scripts/upstream_example_smoke.sh` | repo-neutral 验收、公开样本评测、holdout/adversarial 评测、hidden-like matrix smoke、opt-in LLM smoke、任意 repo LLM smoke 和 examples 迁移演练 |
| `docs/architecture.md`、`docs/data-contract.md`、`docs/issue-2004-traceability.md`、`docs/ci.md`、`docs/sandbox-safety.md` | 只迁移会被 reviewer 用到的 contract 文档 |

## 不应迁移的内容

- 仓库级 `.github/workflows`，官方仓库是否接 CI 由维护者决定。
- 个人机器路径、临时数据库、`/private/tmp` 输出。
- 大量本地扩展 fixture 本体；只保留 external TSV / env var 接入契约和最小 smoke 样本。
- 独立仓库私有历史、实验性 prompt、未被 README 链接的本地草稿。
- 本地 `cr-agent.yaml`、`examples/review.db` 和任何真实 API key。
- 过长的阶段性 implementation plan；官方 PR 描述里保留当前状态和缺口即可。

## go module 和 import 调整

迁入官方仓库后需要把当前 module import：

```text
github.com/Skylm808/CR-trpc-agent-go/...
```

调整为官方 examples 内部可用路径。可选方案：

1. 作为 `examples/cr-agent` 独立 Go module，保留本示例自己的 `go.mod`，依赖 `trpc.group/trpc-go/trpc-agent-go`。
2. 合并到官方 `examples/go.mod`，把包路径改成 examples module 下的相对 import。

建议先采用独立 example module，便于隔离 SQLite、Docker E2E 和 fixture 评测脚本。

## 后续框架路线

当前仍保持 CLI Agent 原型，因为 issue 的核心是可验证 CR 链路：diff 输入、Skill 执行、沙箱检查、Permission 审计、结构化报告、SQLite 回放和验收脚本。迁移 examples 前优先保持独立仓库内分层稳定，再考虑最小迁移包。以下模块适合后续按需接入：

| 官方模块 | 接入时机 |
|----------|----------|
| Runner / Event | 已有官方 `model.Model` adapter 和 `runner.Run` / `event.Event` adapter；迁移 examples 时保留这个主入口 |
| Session / Memory | 需要跨 PR 历史、长期经验复用、会话恢复时接入 |
| E2B / Cube | 当前只有 unsupported 审计入口；官方依赖里已有 `codeexecutor/e2b`，但示例迁移前仍需要补 workspace staging、API/env 配置、artifact 拉取和 sandbox cleanup contract |
| telemetry exporter | 进入服务化部署后，把官方 trace / metric 接到 OTLP dashboard |

## 本地迁移演练

不污染官方仓库的 dry run：

```bash
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/upstream_example_smoke.sh \
  --work-dir /tmp/cr-agent-upstream-example-smoke \
  --keep
```

脚本会把最小迁移包复制到：

```text
/tmp/cr-agent-upstream-example-smoke/trpc-agent-go/examples/cr-agent
```

并在该目录执行 `go run ./cmd/review-agent`，使用根目录下的
`cr-agent.example.yaml` 和 `sample.diff`
生成 `review_report.json`、`review_report.md`、`review_diagnostics.json`。

当前演练结论：采用独立 example module 的路径最自然；样例 config 中的
`skills_root: skills`、`fixtures_root: testdata/fixtures` 在迁移目录下无需改动。

## E2B / Cube 最小 adapter 边界

当前 `--runtime e2b` 的 explicit unsupported 入口足以避免静默 fallback，但不等于
Issue 最终态里的真实远端沙箱。最小实现前置条件：

1. 配置：`E2B_API_KEY` / endpoint / template 或 image 名称，全部只从 env/YAML 引用，不入报告。
2. workspace staging：把待审 repo 或最小 diff 工作区上传到远端 workspace。
3. 命令映射：复用现有 `approval` PermissionPolicy、`execution` timeout、output limit 和实际 env allowlist。
4. artifact 拉取：stdout/stderr digest、报告和必要日志回传本地 artifact/SQLite。
5. cleanup contract：无论成功、失败还是超时，都关闭远端 sandbox，并有测试证明不会遗留实例。

在这些条件明确前，继续保持 unsupported 比半成品联网执行更可审计。

## 迁移前检查清单

1. `GOCACHE=/private/tmp/cr-agent-gocache go test ./...`
2. `scripts/eval.sh`
3. `scripts/holdout_eval.sh`
4. `bash scripts/hidden_matrix_smoke.sh`
5. `GOCACHE=/private/tmp/cr-agent-gocache scripts/upstream_example_smoke.sh`
6. `bash -n scripts/llm_smoke.sh`
7. `CR_AGENT_ACCEPTANCE_DOCKER=skip GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh`
8. Docker 可用时运行 container E2E，并对比 `docker ps -a` 前后状态。
9. `git diff --check`
10. 确认 README 不含个人路径或独立仓库专属说法。
11. 确认 docs 明确：SQLite 是审计 store，不是假装官方 Session Service。
