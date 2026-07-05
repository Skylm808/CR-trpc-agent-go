# Code Review Agent Example

Minimal upstream-friendly example shape for a Go code review agent built with
`trpc-agent-go`.

## Files

```text
examples/cr-agent/
├── README.md
├── cr-agent.example.yaml
└── sample.diff
```

The full implementation currently lives in the repository root:

- `cmd/review-agent`
- `internal/agent`
- `internal/review`
- `internal/report`
- `internal/storage`
- `skills/code-review`

See [../../docs/upstream-example-migration.md](../../docs/upstream-example-migration.md)
for the exact migration set.

## Configure

Copy the safe example config:

```bash
cp examples/cr-agent/cr-agent.example.yaml cr-agent.yaml
```

`cr-agent.yaml` is local-only. Do not commit API keys.

DeepSeek:

```yaml
mode: fake-model
model:
  provider: deepseek
  name: deepseek-chat
  api_key_env: DEEPSEEK_API_KEY
```

`fake-model` means the model review stage runs. With `provider: deepseek`, this
calls DeepSeek through official `trpc-agent-go/model/openai`.

## Run

From the repository root:

```bash
go run ./cmd/review-agent \
  --config ./cr-agent.yaml \
  --diff-file ./examples/cr-agent/sample.diff \
  --skills-root ./skills \
  --runtime local-fallback \
  --output-dir /tmp/cr-agent-example
```

The CLI accepts official-example-style flags:

```bash
go run ./cmd/review-agent \
  --config ./cr-agent.yaml \
  --diff-file ./examples/cr-agent/sample.diff \
  -model="deepseek-chat" \
  -streaming=true
```

`-model` only sets the model name. It does not enable a real LLM unless the YAML
or CLI also selects a real provider.

## Live Smoke

This is opt-in and uses your local ignored config:

```bash
CR_AGENT_LLM_SMOKE=1 \
CR_AGENT_LLM_CONFIG=./cr-agent.yaml \
scripts/llm_smoke.sh
```

Smoke validates connectivity, report generation, and secret non-leakage. It is
not an accuracy benchmark.
