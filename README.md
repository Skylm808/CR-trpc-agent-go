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
- [examples/review_diagnostics.json](examples/review_diagnostics.json)

常用 CLI flags：

```text
--fixture        从 --fixtures-root 运行 fixture
--runtime        container、local-fallback 或 e2b
--staticcheck    sandbox mode 中追加 staticcheck ./...
```

## 测试

公开 fixture 评测和 hidden-like 外部 matrix smoke：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
GOCACHE=/private/tmp/cr-agent-gocache bash scripts/hidden_matrix_smoke.sh
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

LLM 验证分三层：

1. 无网络单测：prompt、decode、redaction、失败降级；
2. deterministic fake-provider 集成测试：report/SQLite 行为；
3. opt-in live smoke：DeepSeek/OpenAI-compatible 连通性。

## Examples 迁移

轻量迁移形态见 [examples/cr-agent](examples/cr-agent)。
迁移说明见 [docs/upstream-example-migration.md](docs/upstream-example-migration.md)。
本地迁移演练可运行：

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/upstream_example_smoke.sh
```

## Issue #2004 仍缺什么

- 真实 E2B/Cube runtime adapter；
- 真实 hidden fixture matrix 验收记录；
- 官方 Session/Memory 映射，用于跨评审历史；
- metric exporter / OTLP dashboard；
- 部署层 runtime 环境隔离。

权威进度矩阵见 [docs/issue-2004-traceability.md](docs/issue-2004-traceability.md)。
