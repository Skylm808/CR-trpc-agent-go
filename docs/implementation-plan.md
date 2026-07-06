# 当前实现计划

本文档只保留当前事实、剩余缺口和验收命令。已经完成的旧阶段计划和 prompt 状态不再作为 roadmap 展开，避免把已完成工作误读成下一步。

## 当前已完成

| 能力 | 状态 | 证据 |
|------|------|------|
| trpc-agent-go Skill / Tool | 已完成 | `tool/skill` load/run `skills/code-review/scripts/check.sh` |
| PermissionPolicy | 已完成 | Skill 和 Go check 命令先过 allow/ask/deny 决策 |
| CodeExecutor / workspaceexec | 已完成 | 默认 `codeexecutor/container`；Go check 优先 `tool/workspaceexec`，`tool/codeexec` 兜底 |
| artifact | 已完成 | 本地报告 + 官方 artifact service；SQLite 保存引用 |
| telemetry | 已完成 | 官方 trace span 记录 task、mode、runtime、tool/model 调用、异常、finding 和 conclusion 摘要 |
| SQLite 审计 | 已完成 | task、decision、sandbox run、finding、artifact、metrics、report 可按 task_id 查询 |
| model.Model adapter | 已完成 | `ModelReviewProvider` 通过官方 `trpc-agent-go/model.Model` 包装调用 |
| OpenAI-compatible / DeepSeek | 已完成 | `--model-provider openai|openai-compatible|deepseek` 走官方 `trpc-agent-go/model/openai`，缺 key 降级人工复核 |
| Runner/Event 主入口 | 已完成 | `Agent.Run` 经 `RunWithEvents` 调官方 `runner.NewRunner(...).Run(...)`，保留兼容 shim |
| E2B unsupported 入口 | 已完成 | `--runtime e2b` 明确记录 unsupported，不静默 fallback |
| base/head ref 输入 | 已完成 | `--base-ref` / `--head-ref` 进入 metadata、报告、diagnostics、SQLite report 和 telemetry |
| CLI 少参数入口 | 已完成 | 无输入 flags 时推断 `--repo-path .` |
| YAML 配置入口 | 已完成 | 默认读取本地 ignored `cr-agent.yaml`；提交 `cr-agent.example.yaml` 和 `examples/cr-agent/cr-agent.example.yaml` |
| 双语 README | 已完成 | `README.md` 为中文默认入口，`README.en.md` 为英文入口 |
| 官方 examples 风格 CLI | 已完成 | `-model` 兼容 `--model-name`，`-streaming` 安全接受且不自动联网 |
| 真实 git repo fixture 测试 | 已完成 | `TestRunUsesGeneratedRepoFixtureWithBaseAndHeadRefs` 动态创建临时 git repo，验证 base/head diff、报告/diagnostics/SQLite |
| LLM smoke 入口 | 已完成 | `scripts/llm_smoke.sh` 使用临时 git repo，支持 env 和 `CR_AGENT_LLM_CONFIG` |
| 公开 fixture eval | 已完成 | `scripts/eval.sh` 覆盖公开 matrix、recall、precision、false positive rate |
| holdout/adversarial eval | 已完成 | `scripts/holdout_eval.sh` 覆盖自包含 holdout matrix、false-positive guardrail 和 fake-model 语义增量路径 |
| hidden matrix 注入契约 | 已完成 | `CR_AGENT_EVAL_FIXTURES_ROOT` / `CR_AGENT_EVAL_MATRIX` / `CR_AGENT_EVAL_REPORT_ROOT` |
| hidden-like 本地验收入口 | 已完成 | `scripts/hidden_matrix_smoke.sh` 用临时外部 root/matrix 模拟 hidden 样本，并保留 report root |

## 当前必需缺口

1. 持续扩充 holdout/adversarial 样本，尤其是能稳定证明真实模型语义增量价值的 diff。
2. 继续收紧 Skill contract 和脚本拆分边界，保持 `scripts/check.sh` 作为稳定入口。

## 非阻塞扩展项

- E2B/Cube 真实 runtime adapter：Issue 主线可由默认 `codeexecutor/container` 满足；当前 `--runtime e2b` 是显式 unsupported 审计入口，不静默 fallback。
- 官方 `session/sqlite` / Memory：当前 CR Agent 是一次性 diff review workflow，SQLite 审计 store 已满足 task/finding/sandbox/report 回放；跨 PR 经验复用时再接。
- metric exporter / OTLP dashboard：当前 report / diagnostics / SQLite metrics / trace span 已记录验收所需监控审计字段，服务化部署时再接 exporter。
- 部署层 runtime 加固：当前原型记录 env whitelist、timeout、output limit、artifact cap 和脱敏；生产环境可按平台继续加固。

## 验收命令

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
GOCACHE=/private/tmp/cr-agent-gocache scripts/holdout_eval.sh
GOCACHE=/private/tmp/cr-agent-gocache bash scripts/hidden_matrix_smoke.sh
CR_AGENT_ACCEPTANCE_DOCKER=skip GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
git diff --check
```

Docker 可用时追加：

```bash
docker ps -a
CR_AGENT_RUN_CONTAINER_TESTS=1 \
GOCACHE=/private/tmp/cr-agent-gocache \
go test ./internal/agent -run TestAgentRunContainerRuntimeExecutesGoChecks -count=1
docker ps -a
```

真实 LLM smoke 是 opt-in：

```bash
CR_AGENT_LLM_SMOKE=1 \
CR_AGENT_LLM_CONFIG=./cr-agent.yaml \
scripts/llm_smoke.sh
```

## Definition of Done

- [x] 公开 fixture 全部可运行并按 rule_id/severity/status 断言。
- [x] holdout/adversarial matrix 自包含运行，并覆盖 fake-model 增量 finding。
- [x] findings、warnings、human review items 结构化、去重、脱敏。
- [x] SQLite 记录 task / decisions / sandbox runs / artifacts / metrics / reports。
- [x] 沙箱失败、超时不崩溃 review，且写入 DB。
- [x] 报告和 finding evidence 中不出现明文 API key / token / password。
- [x] container runtime 有 env-gated E2E 测试。
- [x] LLM provider 适配官方 `model.Model`，OpenAI-compatible / DeepSeek 走官方 `model/openai`。
- [x] 官方 Runner/Event 主入口，保留 `Agent.Run` 兼容 shim。
- [x] E2B runtime unsupported 审计入口。
- [x] base/head ref 输入。
- [x] CLI 少参数当前目录入口。
- [x] YAML 配置入口和 examples 安全配置样例。
- [x] 默认中文 README 和英文 README。
- [x] hidden-like 本地验收入口证明外部 root/matrix/report root contract。
- [x] 外部 hidden matrix 注入契约；如 reviewer/CI 提供私有样本，可额外运行但不是本仓库 blocker。

## 本轮审查记录

- 文档集合保持精简：`docs/` 下 10 个文档均被 `docs/README.md` 索引并服务于架构、验收、数据契约、安全、评测或迁移边界；本轮未发现可直接删除的孤立 md。
- `examples/cr-agent` 迁移面仍保持轻量，只包含 README、安全 YAML 和 sample diff；`skills_root: skills`、`fixtures_root: testdata/fixtures` 适合迁入独立 example module 后继续使用。
- E2B/Cube 不在本轮实现真实 adapter；当前代码和文档应继续明确它只是 unsupported 审计入口，避免半成品联网执行绕过 workspace staging、artifact 拉取和 cleanup contract。
- 真实 LLM smoke 已证明 DeepSeek/OpenAI-compatible provider 通路和泄漏约束；本轮目标 diff 的模型阶段返回 0 条增量 finding，不能把它写成真实模型语义检出能力证明。
- `testdata/holdout/model-semantic.diff` 使用 deterministic fake model 证明模型合并链路可产生增量 finding；真实模型语义价值仍需后续样本持续校准。

## 相关文档

- [architecture.md](architecture.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [eval-matrix.md](eval-matrix.md)
- [sandbox-safety.md](sandbox-safety.md)
- [ci.md](ci.md)
- [upstream-example-migration.md](upstream-example-migration.md)
