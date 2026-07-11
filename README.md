# CR-trpc-agent-go

English version: [README.en.md](README.en.md)

这是一个基于官方 [trpc-agent-go](https://github.com/trpc-group/trpc-agent-go)
组件构建的 Go 代码评审 Agent 示例。它读取 diff、fixture、文件列表或 git
工作区变更，执行 code-review Skill，可选运行沙箱 Go 检查，然后生成
JSON/Markdown 报告和 SQLite 审计记录。

## 这是什么

这个仓库是应用层示例，不是框架 fork。

Issue #2004 主链路保持显式：

- `trpc-agent-go/tool/skill` 加载并执行 `skills/code-review`。
- `tool.PermissionPolicy` 在命令执行前做治理。
- `tool/workspaceexec` 和 `tool/codeexec` 执行 Go 检查。
- `codeexecutor/container` 是默认沙箱 runtime。
- `artifact` 保存报告产物。
- telemetry 记录评审摘要属性。
- SQLite 保存 task、decision、sandbox run、finding、artifact、metrics 和 report。
- 可选 LLM review 走官方 `model.Model`；DeepSeek/OpenAI-compatible 使用 `trpc-agent-go/model/openai`。

更完整的架构和验收矩阵见 [docs/issue-2004-traceability.md](docs/issue-2004-traceability.md)。
Reviewer 可以先看 [docs/reviewer-guide.md](docs/reviewer-guide.md)，里面按 review surface、安全边界、Testing Matrix、live LLM evidence 和 Not-tested 限制列出审查入口。

## 方案设计说明

本 Agent 面向 Go 项目代码评审场景，把 Issue #2004 要求拆成可验证的执行链路：CLI 先把 `--diff-file`、`--repo-path`、`--file-list` 或 fixture 归一化为 unified diff，并提取 Go 文件、package、测试触达和 base/head metadata；随后通过 `tool/skill` 加载 `skills/code-review`，由固定入口 `scripts/check.sh` 执行 deterministic 规则，覆盖 secret、goroutine/context、资源关闭、错误处理、测试缺失和数据库生命周期等风险。沙箱检查只在 `sandbox` 模式执行，默认使用 `codeexecutor/container`，本地 `local-fallback` 必须显式选择；`go test`、`go vet` 和可选 `staticcheck` 在进入 workspace/codeexec 前都要经过 `PermissionPolicy` 精确 allowlist。Agent 会对 findings 做去重、置信度分流和脱敏，高置信项进入 findings，低置信或治理异常进入 warnings / needs_human_review。报告同时写出 JSON、Markdown 和 diagnostics，并通过 SQLite 保存 task、permission/filter decision、sandbox run、finding、artifact、metrics 和 final report。安全边界包含 timeout、output limit、env whitelist、artifact size cap、失败不崩溃和明文密钥脱敏；监控字段记录总耗时、沙箱耗时、工具调用、权限拦截、severity 分布和异常分布。fake-model / rule-only / dry-run 保证无真实 API key 时也能完整测试链路。

## 快速开始

运行完整测试：

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
```

运行本地验收：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

在当前仓库跑一次 review：

```bash
go run ./cmd/review-agent --runtime local-fallback --output-dir /tmp/review-out
```

没有传输入参数时，CLI 会把当前目录推断为 `--repo-path .`。默认 mode 是
`rule-only`，不需要 API key。

## YAML 配置

本地配置是可选的。先复制安全样例：

```bash
cp cr-agent.example.yaml cr-agent.yaml
```

`cr-agent.yaml` 已被 git 忽略。最小配置：

```yaml
mode: rule-only
runtime: local-fallback
output_dir: .cr-agent/reports
skills_root: skills
fixtures_root: testdata/fixtures
```

配置优先级：

```text
CLI flags > YAML > env/default
```

## DeepSeek / OpenAI-Compatible

`fake-model` 表示进入模型评审阶段，不一定表示 provider 是假的。如果
`model.provider=deepseek`，评审会调用 DeepSeek。

推荐 DeepSeek 配置：

```yaml
mode: fake-model
model:
  provider: deepseek
  name: deepseek-chat
  api_key_env: DEEPSEEK_API_KEY
```

运行：

```bash
export DEEPSEEK_API_KEY="your-key"
go run ./cmd/review-agent --config ./cr-agent.yaml
```

本机 smoke 也支持在 ignored YAML 里直接写 `model.api_key`，但推荐优先使用
`api_key_env`。不要提交明文 key。smoke 脚本会检查 report 和 diagnostics 是否泄漏 key。

OpenAI-compatible 网关可以使用：

```bash
export OPENAI_API_KEY="your-key"
export OPENAI_BASE_URL="https://your-gateway.example.com/v1"
```

## Modes

| Mode | 行为 |
|------|------|
| `rule-only` | 执行 deterministic Skill/rule 检查，不调用模型。 |
| `dry-run` | 加载 Skill 并记录跳过执行。 |
| `sandbox` | 执行 Skill 检查，并通过 workspace execution 运行 Go 检查。 |
| `fake-model` | 执行 Skill 检查后进入模型评审阶段；未配置真实 provider 时使用 fake provider。 |

## 输出

每次运行会写出：

- `review_report.json`
- `review_report.md`
- `review_report.zh.md`
- `review_diagnostics.json`

真实模型运行后，`metrics` 会记录非敏感审计字段：

- `model_provider`
- `model_name`
- `model_backend`

使用 `--sqlite /path/to/review.db` 时，审计库可回放：

- task 状态
- permission/filter decisions
- sandbox runs
- findings 和 warnings
- artifacts
- metrics
- final reports

已提交的示例输出：

- [examples/review_report.json](examples/review_report.json)
- [examples/review_report.md](examples/review_report.md)
- [examples/review_report.zh.md](examples/review_report.zh.md)
- [examples/review_diagnostics.json](examples/review_diagnostics.json)

常用 CLI flags：

```text
--fixture        从 --fixtures-root 运行 fixture
--runtime        container、local-fallback 或 e2b
--staticcheck    sandbox mode 中追加 staticcheck ./...
```

`container` 是默认生产沙箱路径；`local-fallback` 仅用于本地开发；`e2b` 当前是显式 unsupported audit 入口，不会静默回退到本地执行。Issue 主线由默认 `codeexecutor/container` 满足。

## Testing Matrix

Reviewer 最短复现路径：

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
GOCACHE=/private/tmp/cr-agent-gocache scripts/holdout_eval.sh
GOCACHE=/private/tmp/cr-agent-gocache bash scripts/hidden_matrix_smoke.sh
CR_AGENT_ACCEPTANCE_DOCKER=skip GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

这组命令覆盖 unit/integration、公开 fixtures、holdout matrix、hidden-like
smoke、examples 迁移和本地验收。更完整的 reviewer 视角见
[docs/reviewer-guide.md](docs/reviewer-guide.md)。

如果只想快速看报告产物和 SQLite 审计，可运行：

```bash
out="$(mktemp -d)"
GOCACHE=/private/tmp/cr-agent-gocache go run ./cmd/review-agent \
  --config /dev/null \
  --fixture realistic-service-risk.diff \
  --fixtures-root testdata/fixtures \
  --skills-root skills \
  --runtime local-fallback \
  --sqlite "$out/review.db" \
  --output-dir "$out"
ls "$out"
```

upstream examples 迁移 smoke 仍是独立检查：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/upstream_example_smoke.sh
```

Docker container 沙箱测试：

```bash
docker ps -a
CR_AGENT_RUN_CONTAINER_TESTS=1 \
GOCACHE=/private/tmp/cr-agent-gocache \
go test ./internal/agent -run TestAgentRunContainerRuntimeExecutesGoChecks -count=1
docker ps -a
```

真实 LLM smoke 是 opt-in，使用临时 git repo，验证 provider 通路和泄漏约束，不评估模型准确率：

```bash
scripts/llm_smoke.sh

CR_AGENT_LLM_SMOKE=1 \
CR_AGENT_LLM_CONFIG=./cr-agent.yaml \
scripts/llm_smoke.sh
```

对任意本地 git repo 跑真实 LLM smoke：

```bash
CR_AGENT_LLM_SMOKE=1 \
scripts/repo_llm_smoke.sh \
  --repo /path/to/repo \
  --config ./cr-agent.yaml \
  --go-only \
  --output-dir /tmp/cr-agent-repo-smoke
```

脚本会从本仓库根目录运行 `go run ./cmd/review-agent`，并检查
`model_call_count=1`、`model_provider` 存在和 API key 不泄漏。

live LLM evidence 分四层：

1. 无网络单测：prompt、decode、redaction、失败降级；
2. deterministic fake-provider 集成测试：report/SQLite 行为；
3. opt-in live smoke：DeepSeek/OpenAI-compatible 连通性；
4. opt-in semantic eval：固定语义样本上记录真实模型实际发现项。

真实 LLM 语义评测会保留每个 fixture 的 `review_report.json`、
`review_report.md`、`review_report.zh.md` 和 `review_diagnostics.json`，并额外生成
`llm_semantic_eval.md` 索引、`llm_semantic_eval.zh.md` 中文汇总和
`llm_semantic_eval.en.md` 英文汇总。当 provider 返回高置信
`source=model` 项时，模型 finding 会写入 `review_report.md` 的 Findings 段。
如果提供 `testdata/holdout/expected.tsv`，summary 还会计算 fixture-level
recall 和 safe-fixture false-positive 数；它是人工可复核证据，不是 CI 硬门禁。
真实模型输出可能随 provider、模型版本、网络和 prompt 行为波动，因此 semantic
eval 是 reviewable evidence, not a hard CI gate。

```bash
CR_AGENT_LLM_SMOKE=1 \
CR_AGENT_LLM_CONFIG=./cr-agent.yaml \
scripts/llm_semantic_eval.sh
```

## Examples 迁移

轻量迁移形态见 [examples/cr-agent](examples/cr-agent)。
迁移说明见 [docs/upstream-example-migration.md](docs/upstream-example-migration.md)。
本地迁移演练可运行：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/upstream_example_smoke.sh
```

## Not-tested / Issue #2004 仍缺什么

- 真实 E2B workspace runtime 仍是显式 unsupported audit path；当前生产形态路径是 `codeexecutor/container`。
- 真实模型输出会波动；live semantic eval 是人工证据，不作为 deterministic CI hard gate。
- 中文报告已经对 deterministic rule findings 增加中文补充；model findings 保留模型原始措辞。
- SQLite `reports` 表保存 JSON 和英文 Markdown 正文；`review_report.zh.md` 通过 artifact 引用保存 digest/size，不额外扩 schema。
- 继续用 self-contained holdout/adversarial 样本校准误报边界，尤其是真实模型能发现的语义风险。
- hidden 类验收采用仓库自造的 holdout matrix 和 hidden-like external smoke；如需扩展更多本地样本，可通过 `CR_AGENT_EVAL_FIXTURES_ROOT` / `CR_AGENT_EVAL_MATRIX` 追加评测。

非阻塞扩展项：E2B/Cube 真实 adapter、跨 PR Session/Memory、metric exporter / OTLP dashboard、生产部署层额外 runtime 加固。Issue 主线允许 `codeexecutor/container` 或 E2B workspace runtime；当前默认生产路径已经是 container，且具备 timeout、output limit、permission gate、failure record 和 SQLite 审计，所以 E2B 不是当前 blocker。

权威进度矩阵见 [docs/issue-2004-traceability.md](docs/issue-2004-traceability.md)。
