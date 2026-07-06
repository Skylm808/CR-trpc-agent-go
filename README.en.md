# CR-trpc-agent-go

Chinese version: [README.md](README.md)

Go code review agent example built on top of official
[trpc-agent-go](https://github.com/trpc-group/trpc-agent-go) components.

It reads a diff, fixture, file list, or git working tree, runs the code-review
Skill through the agent pipeline, optionally runs sandboxed Go checks, then
writes JSON/Markdown reports plus a SQLite audit trail.

## What It Is

This repository is an application-style example, not a framework fork.

It keeps the Issue #2004 path explicit:

- `trpc-agent-go/tool/skill` loads and runs `skills/code-review`.
- `tool.PermissionPolicy` gates commands before execution.
- `tool/workspaceexec` and `tool/codeexec` run Go checks.
- `codeexecutor/container` is the default sandbox runtime.
- `artifact` stores report artifacts.
- telemetry records review summary attributes.
- SQLite stores task, decisions, sandbox runs, findings, artifacts, metrics, and reports.
- optional LLM review uses the official `model.Model` path; DeepSeek/OpenAI-compatible providers use `trpc-agent-go/model/openai`.

Longer architecture and acceptance details live in [docs/issue-2004-traceability.md](docs/issue-2004-traceability.md).

## Quick Start

Run the full unit/integration suite:

```bash
GOCACHE=/private/tmp/cr-agent-gocache go test ./...
```

Run the local acceptance workflow:

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh
```

Run a review in this repository:

```bash
go run ./cmd/review-agent --runtime local-fallback --output-dir /tmp/review-out
```

When no input flag is provided, the CLI treats the current directory as
`--repo-path .`. The default built-in mode is `rule-only`; it does not require an
API key.

## Config YAML

Local config is optional. Copy the safe example:

```bash
cp cr-agent.example.yaml cr-agent.yaml
```

`cr-agent.yaml` is ignored by git. A minimal local config:

```yaml
mode: rule-only
runtime: local-fallback
output_dir: .cr-agent/reports
skills_root: skills
fixtures_root: testdata/fixtures
```

Config priority is:

```text
CLI flags > YAML > env/default
```

## DeepSeek / OpenAI-Compatible

`fake-model` means "enter the model review stage." It does not always mean the
provider is fake. If `model.provider=deepseek`, the review calls DeepSeek.

Recommended DeepSeek config:

```yaml
mode: fake-model
model:
  provider: deepseek
  name: deepseek-chat
  api_key_env: DEEPSEEK_API_KEY
```

Then run:

```bash
export DEEPSEEK_API_KEY="your-key"
go run ./cmd/review-agent --config ./cr-agent.yaml
```

For workstation-only smoke testing, ignored YAML also supports `model.api_key`,
but `api_key_env` is preferred. Do not commit direct keys. The smoke script
checks reports and diagnostics for key leakage.

OpenAI-compatible gateways can use:

```bash
export OPENAI_API_KEY="your-key"
export OPENAI_BASE_URL="https://your-gateway.example.com/v1"
```

## Modes

| Mode | Behavior |
|------|----------|
| `rule-only` | Run deterministic Skill/rule checks. No model call. |
| `dry-run` | Load the Skill and record skipped execution. |
| `sandbox` | Run Skill checks plus Go checks through workspace execution. |
| `fake-model` | Run Skill checks and then the model review stage. Uses fake provider unless a real provider is configured. |

## Outputs

Each run writes:

- `review_report.json`
- `review_report.md`
- `review_diagnostics.json`

After a real model run, `metrics` includes non-sensitive audit fields:

- `model_provider`
- `model_name`
- `model_backend`

With `--sqlite /path/to/review.db`, the audit store can replay:

- task status
- permission/filter decisions
- sandbox runs
- findings and warnings
- artifacts
- metrics
- final reports

Sample committed outputs:

- [examples/review_report.json](examples/review_report.json)
- [examples/review_report.md](examples/review_report.md)
- [examples/review_diagnostics.json](examples/review_diagnostics.json)

Common CLI flags:

```text
--fixture        run a fixture from --fixtures-root
--runtime        container, local-fallback, or e2b
--staticcheck    include staticcheck ./... in sandbox mode
```

## Tests

Public fixture evaluation:

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh
GOCACHE=/private/tmp/cr-agent-gocache scripts/holdout_eval.sh
GOCACHE=/private/tmp/cr-agent-gocache bash scripts/hidden_matrix_smoke.sh
GOCACHE=/private/tmp/cr-agent-gocache scripts/upstream_example_smoke.sh
```

Docker container sandbox test:

```bash
docker ps -a
CR_AGENT_RUN_CONTAINER_TESTS=1 \
GOCACHE=/private/tmp/cr-agent-gocache \
go test ./internal/agent -run TestAgentRunContainerRuntimeExecutesGoChecks -count=1
docker ps -a
```

Live LLM smoke is opt-in and uses a temporary git repo. It verifies the real
provider path and leakage invariants, not model accuracy:

```bash
scripts/llm_smoke.sh

CR_AGENT_LLM_SMOKE=1 \
CR_AGENT_LLM_CONFIG=./cr-agent.yaml \
scripts/llm_smoke.sh
```

Run live LLM smoke against any local git repo:

```bash
CR_AGENT_LLM_SMOKE=1 \
scripts/repo_llm_smoke.sh \
  --repo /path/to/repo \
  --config ./cr-agent.yaml \
  --go-only \
  --output-dir /tmp/cr-agent-repo-smoke
```

The script runs `go run ./cmd/review-agent` from this repository root and checks
`model_call_count=1`, a present `model_provider`, and no API key leakage.

Complete LLM verification is layered:

1. no-network unit tests for prompt, decoding, redaction, and failure handling;
2. deterministic fake-provider integration tests for report/SQLite behavior;
3. opt-in live smoke for DeepSeek/OpenAI-compatible connectivity.

## Examples Migration

The upstream-friendly example shape is in [examples/cr-agent](examples/cr-agent).
Migration notes are in [docs/upstream-example-migration.md](docs/upstream-example-migration.md).
Local migration rehearsal:

```bash
GOCACHE=/private/tmp/cr-agent-gocache scripts/upstream_example_smoke.sh
```

## What Is Still Missing For Issue #2004

- more holdout/adversarial fixtures, especially semantic risks a real model can add;
- optional external hidden evaluation when reviewer/CI provides `CR_AGENT_EVAL_FIXTURES_ROOT` and `CR_AGENT_EVAL_MATRIX`.

Non-blocking extensions: real E2B/Cube adapter, cross-PR Session/Memory, metric exporter / OTLP dashboard integration, and extra production runtime hardening.

The authoritative progress matrix is [docs/issue-2004-traceability.md](docs/issue-2004-traceability.md).
