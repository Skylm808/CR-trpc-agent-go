# 当前实现计划

本文档记录当前真实 roadmap。旧的 M0-M5 里程碑状态已收敛到本文，避免把已完成的 Docker/Eval 工作继续显示为缺口。

## 当前已完成

| 能力 | 状态 | 证据 |
|------|------|------|
| trpc-agent-go Skill / Tool | ✅ | `tool/skill` load/run `skills/code-review/scripts/check.sh` |
| PermissionPolicy | ✅ | 所有 Skill 和 Go check 命令先过 allow/ask/deny 决策，非 allow 不进入 executor |
| CodeExecutor / workspaceexec | ✅ | 默认 `codeexecutor/container`，Go check 优先 `tool/workspaceexec`，`tool/codeexec` 兜底 |
| artifact | ✅ | 报告和 diagnostics 写本地并同步写入官方 artifact service，SQLite 保存引用 |
| telemetry | ✅ | 官方 trace span 记录 task、mode、runtime、tool/model 调用、异常、finding 和 conclusion 摘要 |
| SQLite 审计 | ✅ | task、decision、sandbox run、finding、artifact、metrics、report 均可按 task_id 查询 |
| deterministic fake provider | ✅ | 默认无 API Key、无网络模型，经官方 `model.Model` adapter 调用 `ModelReviewProvider` 边界 |
| opt-in HTTP provider | ✅ | `--model-provider http` 显式开启，经官方 `model.Model` adapter 调用，输入/输出脱敏，失败降级人工复核 |
| Runner/Event 主入口 | ✅ | `Agent.Run` 通过 `RunWithEvents` 调用官方 `runner.NewRunner(...).Run(...)`，并消费官方 `event.Event` 流 |
| event.Event sink | ✅ | `EventSink` 仍可观察 input_loaded、skill_run、sandbox_run、model_review、report_written、task_finished/task_failed |
| E2B unsupported 入口 | ✅ | `--runtime e2b` 生成 `runtime=e2b status=unsupported` 审计记录，不静默 fallback |
| base/head ref 输入 | ✅ | `--base-ref` / `--head-ref` 进入 metadata/report/diagnostics/SQLite report/telemetry；git repo 可生成 `base...head` diff |
| Docker container E2E | ✅ | env-gated container runtime test 已可在 Docker Desktop/daemon 下运行 |
| 公开 fixture eval | ✅ | `scripts/eval.sh` 覆盖公开 matrix、recall、precision、false positive rate |
| hidden matrix 注入契约 | ✅ | `CR_AGENT_EVAL_FIXTURES_ROOT` / `CR_AGENT_EVAL_MATRIX` / `CR_AGENT_EVAL_REPORT_ROOT`；`CR_AGENT_EVAL_EXPECTED` 保留为兼容别名 |
| 默认无 API Key 路径 | ✅ | `rule-only`、`dry-run`、默认 `fake-model` 不要求真实模型凭证 |

## 下一阶段优先级

### 1. 官方 model.Model / Runner / Event 适配

**当前状态：** 已把 `ModelReviewProvider` 适配到官方 `trpc-agent-go/model.Model`。`Agent.Run` 现在是兼容 shim，内部通过 `RunWithEvents` 调用 `runner.NewRunner(...).Run(...)`，由 `reviewRunnerAgent` 这个官方 `agent.Agent` adapter 发出 `event.Event` 流。adapter 内部仍调用本项目 `runDirect` 执行体，目的是保持 CLI、报告、SQLite 和 fixture contract 不被 Runner 接入打断。

**影响文件：**

- `internal/agent/model.go`
- `internal/agent/model_http.go`
- `internal/agent/agent.go`
- `internal/agent/events.go`
- `internal/agent/runner_official.go`
- `cmd/review-agent/*.go`
- `docs/architecture.md`
- `docs/data-contract.md`

**验收标准：**

- 默认 fake provider 和 opt-in HTTP provider 仍可用，且仍不要求 API Key。
- 当前 `ModelReviewProvider` 的脱敏、分流、去重、失败降级语义不丢失。
- adapter 明确实现并包装官方 `model.Model`，单元测试覆盖 request/response 和 redaction；HTTP timeout/error 继续由既有 provider 测试覆盖。
- 官方 Runner route 输出 input_loaded、skill_run、model_review、sandbox_run、report_written、task_finished/task_failed 等事件。
- 老 CLI 输出和 SQLite 审计 contract 保持兼容。

**测试/验证命令：**

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
CR_AGENT_ACCEPTANCE_DOCKER=skip GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

**剩余边界：**

- 不在这一步绑定 OpenAI / Claude / Gemini 厂商 SDK。
- 不删除无 API Key 默认路径。
- 不为了接完整 Runner 重写规则引擎、SQLite schema 或报告 contract。
- 尚未把规则执行体改造成官方 llmagent/graphagent；当前是官方 Runner + 应用 adapter。

### 2. E2B runtime 最小 unsupported/adapter 入口

**当前状态：** `--runtime e2b` 已提供最小入口。当前不接 E2B SDK，不执行本地 fallback；审查会生成结构化 `unsupported` run、人工复核项、diagnostics 和 SQLite sandbox run，避免用户误以为 E2B 已真实执行。

**影响文件：**

- `internal/agent/execution.go`
- `internal/agent/config.go`
- `cmd/review-agent/run.go`
- `cmd/review-agent/main.go`
- `docs/sandbox-safety.md`
- `docs/architecture.md`
- `docs/ci.md`

**验收标准：**

- CLI 能识别 E2B runtime 标识。
- 未配置 E2B 时不崩溃，产出 `unsupported` 和 `needs_human_review` 记录。
- 报告、diagnostics、SQLite sandbox run 明确写出 E2B 未支持/未配置。
- container 默认 runtime 不变，`local-fallback` 仍只能显式选择。

**测试/验证命令：**

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./internal/agent ./cmd/review-agent
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
```

**风险和不做事项：**

- 不新增 E2B SDK 依赖，除非明确进入真实 adapter 实现。
- 不把 unsupported 伪装成成功执行。
- 不降低 PermissionPolicy 对命令执行的前置要求。

### 3. base/head ref 输入

**当前状态：** CLI 和 `Request` 已支持 `--base-ref` / `--head-ref`。它们进入 `InputMetadata`、report、diagnostics、SQLite report blob 和 telemetry。`--repo-path` 指向 git repo 且二者都提供时，输入读取使用 `git diff --unified=3 base...head`；不会自动 fetch。

**影响文件：**

- `cmd/review-agent/main.go`
- `cmd/review-agent/run.go`
- `internal/agent/input.go`
- `internal/review/parser.go`
- `docs/data-contract.md`
- `README.md`

**验收标准：**

- CLI 提供 `--base-ref` 和 `--head-ref` 或等价输入。
- `--repo-path` + base/head 能生成统一 diff。
- 未传 base/head 时保持现有 working tree diff 行为。
- invalid ref 返回可诊断错误，不生成误导性空报告。
- `review_report.json` 和 diagnostics 记录 base/head ref。

**测试/验证命令：**

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./internal/agent ./cmd/review-agent
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
```

**风险和不做事项：**

- 不自动 fetch 远端 ref；网络和凭证由调用方准备。
- 不改变 `--diff-file`、`--file-list`、`--fixture` 的语义。

### 4. 真实 hidden fixture matrix 验收

**当前状态：** `scripts/eval.sh` 已支持 `CR_AGENT_EVAL_MATRIX=/path/to/expected.tsv`，缺失文件会清晰失败；公开 builtin matrix 保持默认。真实 hidden fixture 本体不提交，因此仓库内只能证明入口和公开 matrix，不能证明真实 hidden 数据达标。

**影响文件：**

- `scripts/eval.sh`
- `cmd/review-agent/eval_script_test.go`
- `docs/eval-matrix.md`
- `docs/ci.md`
- `docs/issue-2004-traceability.md`

**验收标准：**

- 使用真实 hidden root、expected TSV 和 report root 完整运行 `scripts/eval.sh`。
- 记录 recall、precision、false_positive_rate、missing/unexpected 明细。
- 不提交 hidden fixture 本体。
- 若未达阈值，记录失败样本和后续规则/模型校准方向。

**测试/验证命令：**

```bash
CR_AGENT_EVAL_FIXTURES_ROOT=/path/to/hidden-fixtures \
CR_AGENT_EVAL_FIXTURES="hidden-001.diff hidden-002.diff" \
CR_AGENT_EVAL_MATRIX=/path/to/expected.tsv \
CR_AGENT_EVAL_REPORT_ROOT=/tmp/cr-agent-hidden-reports \
GOCACHE=/private/tmp/cr-agent-gocache \
scripts/eval.sh
```

**风险和不做事项：**

- 不把 private hidden 样本提交到公开仓库。
- 不把公开 fixture 的 100% 结果等同于 hidden 达标。

### 5. 持续清理文档和状态矩阵

**目标：** 保持 README、docs 索引、traceability、roadmap 与代码事实一致，避免阶段性 prompt 和历史 milestone 重新变成误导源。

**影响文件：**

- `README.md`
- `docs/README.md`
- `docs/implementation-plan.md`
- `docs/issue-2004-traceability.md`
- `docs/architecture.md`
- `docs/data-contract.md`
- `docs/sandbox-safety.md`
- `docs/ci.md`

**验收标准：**

- docs 索引不引用已删除文件。
- 当前事实保持一致：HTTP provider 已有但 opt-in；默认 fake provider 不需要 API Key；LLM 已有官方 model.Model adapter；CLI 兼容入口已走官方 Runner/Event adapter；hidden matrix 支持外部注入但真实 hidden 数据未随仓库提交；E2B 目前是 unsupported 入口；base/head ref 已作为输入和审计上下文接入。
- 任何新增阶段性 prompt 不作为长期文档入口。

**测试/验证命令：**

```bash
rg -n "缺 hidden/eval|container E2E 未|真实 provider 尚未" README.md docs
rg -n "\]\(([^)]*)\.md\)" README.md docs
git diff --check
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
```

**风险和不做事项：**

- 不为了减少文档数量删除 Issue 硬性交付物。
- 不把迁移参考文档当作当前开发优先级。

## 当前 Definition of Done

- [x] 公开 fixture 全部可运行并按 rule_id/severity/status 断言。
- [x] findings、warnings、human review items 结构化、去重、脱敏。
- [x] SQLite 记录 task / decisions / sandbox runs / artifacts / metrics / reports。
- [x] 沙箱失败、超时不崩溃 review，且写入 DB。
- [x] 报告和 finding evidence 中不出现明文 API Key / token / password。
- [x] container runtime 真实 E2E 有 env-gated 测试，并已在 Docker daemon 环境验证过。
- [x] 公开 fixture eval 脚本和外部 hidden matrix 注入契约。
- [x] 官方 artifact/telemetry 最小接入或清晰边界说明。
- [x] LLM provider 适配官方 model.Model，并通过官方 Runner/Event adapter 暴露阶段事件。
- [x] 官方 Runner/Event 主入口，保留 `Agent.Run` 兼容 shim。
- [x] E2B runtime unsupported/adapter 入口。
- [x] base/head ref 输入。
- [ ] 真实 hidden fixture matrix 验收记录。

## 相关文档

- [architecture.md](architecture.md)
- [data-contract.md](data-contract.md)
- [issue-2004-traceability.md](issue-2004-traceability.md)
- [fixtures-matrix.md](fixtures-matrix.md)
- [eval-matrix.md](eval-matrix.md)
- [sandbox-safety.md](sandbox-safety.md)
- [ci.md](ci.md)
