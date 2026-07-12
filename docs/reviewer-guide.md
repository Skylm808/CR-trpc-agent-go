# Reviewer Guide

This guide is the shortest path for reviewing the CR agent prototype. The
implementation is an application example around official trpc-agent-go
components, not a framework fork.

## Review Surface

Start with these files:

- `cmd/review-agent`: CLI flags, config resolution, and repo/diff entrypoint.
- `internal/agent/agent.go`: review workflow orchestration and event/telemetry boundary.
- `internal/agent/reports.go`: report bundle generation, artifact list, diagnostics, and metrics.
- `internal/agent/persist.go`: SQLite persistence for tasks, decisions, findings, artifacts, metrics, and reports.
- `internal/report/report.go`: JSON, English Markdown, and Chinese Markdown rendering.
- `internal/rules/rules.go`: deterministic rule findings and warnings.
- `internal/llm`: fake, HTTP, OpenAI-compatible, and DeepSeek model provider boundary.
- `internal/execution`: container/local-fallback execution configuration and sandbox arguments.
- `skills/code-review`: Skill contract, rules document, and check scripts.
- `scripts/llm_semantic_eval.sh`: opt-in live LLM semantic evidence.
- `scripts/repo_llm_smoke.sh`: opt-in live LLM smoke against a local git repository.

## Safety Boundaries

The prototype is intentionally fail-closed around command execution and report
artifacts:

- `tool.PermissionPolicy` gates review scripts before sandbox execution.
- Unknown or non-allowlisted high-risk commands do not enter the sandbox.
- Sandbox runs record timeout, output limit, runtime, env whitelist, exit code,
  output digests, and failure status.
- `local-fallback` is an explicit development runtime; `codeexecutor/container`
  remains the default production-shaped runtime.
- Secret redaction is applied before report, diagnostics, SQLite, telemetry, and model
  provider prompts can persist secret-shaped evidence.
- An artifact size cap is enforced before local writes and artifact service saves.
- Provider failures become low-severity `needs_human_review` items instead of
  aborting the whole review.

## Testing Matrix

Recommended reviewer commands:

| Layer | Command | What it proves |
| --- | --- | --- |
| Fast unit | `GOCACHE=/private/tmp/cr-agent-gocache go test ./...` | Parser, rules, reports, persistence and lightweight CLI contracts; expected around 15-20 seconds locally. |
| Integration | `GOCACHE=/private/tmp/cr-agent-gocache go test -tags=integration -p 1 ./internal/agent ./cmd/review-agent ./scripts` | Representative real workspace, CLI, SQLite and script contracts; fixture matrices are excluded to avoid duplicate evaluation. |
| Public fixtures | `GOCACHE=/private/tmp/cr-agent-gocache scripts/eval.sh` | Deterministic fixture recall and false-positive accounting. |
| Holdout fixtures | `GOCACHE=/private/tmp/cr-agent-gocache scripts/holdout_eval.sh` | Broader local recall/precision matrix. |
| Hidden-like smoke | `GOCACHE=/private/tmp/cr-agent-gocache bash scripts/hidden_matrix_smoke.sh` | External matrix contract without committing hidden samples. |
| Acceptance | `CR_AGENT_ACCEPTANCE_DOCKER=skip GOCACHE=/private/tmp/cr-agent-gocache scripts/acceptance.sh` | CLI reports, SQLite audit, and generated artifacts. |
| Container runtime | `CR_AGENT_RUN_CONTAINER_TESTS=1 GOCACHE=/private/tmp/cr-agent-gocache go test -tags=integration ./internal/agent -run TestAgentRunContainerRuntimeExecutesGoChecks -count=1` | Real container executor path when Docker is available. |
| Live semantic evidence | `CR_AGENT_LLM_SMOKE=1 CR_AGENT_LLM_CONFIG=./cr-agent.yaml scripts/llm_semantic_eval.sh` | Real provider call on fixed semantic fixtures; evidence only, not a hard CI gate. |
| Repo LLM smoke | `CR_AGENT_LLM_SMOKE=1 scripts/repo_llm_smoke.sh --repo /path/to/repo --config ./cr-agent.yaml --go-only` | Real provider call against a local git repository diff. |

## Live LLM Evidence

Live LLM evidence is opt-in because provider output can vary and network access
is environment-dependent. The semantic eval writes:

- `llm_semantic_eval.md`
- `llm_semantic_eval.zh.md`
- `llm_semantic_eval.en.md`
- one report directory per fixture, including `review_report.zh.md`

Treat these files as review evidence, not deterministic CI gates. Deterministic
CI should use unit tests, fixture evaluation, holdout evaluation, and fake-provider
provider tests.

## Not Tested / Known Limits

- The real E2B workspace runtime remains an explicit unsupported audit path; the
  production-shaped path is `codeexecutor/container`.
- Live model output can vary by provider, model version, prompt behavior, and
  transient network failures.
- Chinese report titles and recommendations are localized for deterministic rule
  findings; model findings preserve model-provided wording.
- The SQLite reports table stores JSON plus English Markdown report bodies.
  `review_report.zh.md` is persisted as an artifact reference with digest and
  size, but the Chinese body is not stored in the reports table.
- Long-term recall trend tracking for live LLM output is not implemented; current
  scripts produce per-run evidence summaries.
