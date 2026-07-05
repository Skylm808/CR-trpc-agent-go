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
| 官方 examples 风格 CLI | 已完成 | `-model` 兼容 `--model-name`，`-streaming` 安全接受且不自动联网 |
| 真实 git repo fixture 测试 | 已完成 | `TestRunUsesCopiedGitHubAgentRepoAsGitFixture` clone 外部 git repo 到临时目录并验证报告/SQLite |
| LLM smoke 入口 | 已完成 | `scripts/llm_smoke.sh` 使用临时 git repo，支持 env 和 `CR_AGENT_LLM_CONFIG` |
| 公开 fixture eval | 已完成 | `scripts/eval.sh` 覆盖公开 matrix、recall、precision、false positive rate |
| hidden matrix 注入契约 | 已完成 | `CR_AGENT_EVAL_FIXTURES_ROOT` / `CR_AGENT_EVAL_MATRIX` / `CR_AGENT_EVAL_REPORT_ROOT` |

## 当前剩余缺口

1. 真实 hidden fixture matrix 验收记录还没有提交；仓库只保留外部注入契约。
2. E2B/Cube 真实 runtime adapter 还未实现；当前只有 unsupported 审计入口。
3. 官方 `session/sqlite` / Memory 还未映射；当前 SQLite 是审计 store，不是会话服务。
4. 官方 metric exporter / OTLP dashboard 还未接；当前是 trace span + SQLite metrics。
5. runtime 级 env 强隔离仍依赖部署侧 executor 配置。

## 下一步优先级

1. 用真实 hidden fixture matrix 跑一次验收，记录 recall、precision、false positive rate、missing/unexpected 明细。
2. 评估并实现最小 E2B/Cube runtime adapter，替换当前 unsupported 占位。
3. 决定是否接官方 Session/Memory；只有需要跨 PR 经验复用时再做。
4. 进入服务化部署时接 metric exporter / OTLP dashboard。

## 验收命令

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
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
- [x] findings、warnings、human review items 结构化、去重、脱敏。
- [x] SQLite 记录 task / decisions / sandbox runs / artifacts / metrics / reports。
- [x] 沙箱失败、超时不崩溃 review，且写入 DB。
- [x] 报告和 finding evidence 中不出现明文 API key / token / password。
- [x] container runtime 有 env-gated E2E 测试。
- [x] LLM provider 适配官方 `model.Model`，OpenAI-compatible / DeepSeek 走官方 `model/openai`。
- [x] 官方 Runner/Event 主入口，保留 `Agent.Run` 兼容 shim。
- [x] E2B runtime unsupported/adapter 入口。
- [x] base/head ref 输入。
- [x] CLI 少参数当前目录入口。
- [x] YAML 配置入口和 examples 安全配置样例。
- [ ] 真实 hidden fixture matrix 验收记录。

## 相关文档

- [architecture.md](architecture.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [eval-matrix.md](eval-matrix.md)
- [sandbox-safety.md](sandbox-safety.md)
- [ci.md](ci.md)
- [upstream-example-migration.md](upstream-example-migration.md)
