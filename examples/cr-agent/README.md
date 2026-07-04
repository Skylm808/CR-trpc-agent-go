# CR Agent example

这个目录用于迁移到官方 `trpc-agent-go/examples/cr-agent` 时的最小示例形态。配置文件只对本项目的 `review-agent` 生效；官方 `examples/runner` 不会读取这里的 YAML。

## 1. 克隆和设置

```bash
git clone https://github.com/trpc-group/trpc-agent-go.git
cd trpc-agent-go
```

把本项目放入 `examples/cr-agent` 后，在官方仓库根目录运行时建议显式指定配置：

```bash
go run ./examples/cr-agent/cmd/review-agent \
  --config ./examples/cr-agent/cr-agent.yaml \
  --diff-file ./examples/cr-agent/sample.diff \
  --skills-root ./examples/cr-agent/skills \
  --runtime local-fallback
```

如果当前工作目录就是 `examples/cr-agent`，也可以直接运行本项目入口，程序会自动读取当前目录的 `cr-agent.yaml`。

## 2. 配置您的 LLM

默认 `cr-agent.yaml` 不启用真实模型。需要 OpenAI-compatible 中转站时：

```bash
export OPENAI_API_KEY="your-api-key-here"
export OPENAI_BASE_URL="your-base-url-here"
```

然后把 `cr-agent.yaml` 里的 `mode` 改成 `fake-model`，并设置：

```yaml
model:
  provider: openai-compatible
  name: gpt-4o-mini
```

需要 DeepSeek 时：

```bash
export DEEPSEEK_API_KEY="your-deepseek-key-here"
```

```yaml
mode: fake-model
model:
  provider: deepseek
  name: deepseek-chat
```

`api_key_env` 表示环境变量名，不是 API key 明文。仓库根目录的本地 `cr-agent.yaml` 已被 `.gitignore` 忽略；如果只做本机 smoke，也可以在 ignored YAML 中使用 `api_key`，但不建议长期保存明文 key。

## 3. 运行您的第一个 CR Agent

兼容官方示例的 `-model` / `-streaming` 参数：

```bash
go run ./cmd/review-agent \
  --config ./cr-agent.yaml \
  --diff-file ./sample.diff \
  -model="gpt-4o-mini" \
  -streaming=true
```

注意：`-model` 只设置模型名，不会自动启用外部模型。只有 `mode: fake-model` 且 `model.provider` 显式为 `openai`、`openai-compatible` 或 `deepseek` 时，才会调用真实 LLM。

真实 LLM smoke 支持显式配置文件：

```bash
CR_AGENT_LLM_SMOKE=1 \
CR_AGENT_LLM_CONFIG=./cr-agent.yaml \
scripts/llm_smoke.sh
```
