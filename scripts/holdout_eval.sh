#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

CR_AGENT_EVAL_FIXTURES_ROOT="${CR_AGENT_HOLDOUT_FIXTURES_ROOT:-"$ROOT/testdata/holdout"}" \
CR_AGENT_EVAL_FIXTURES="${CR_AGENT_HOLDOUT_FIXTURES:-"holdout-safe-refactor.diff holdout-placeholder-secret.diff holdout-secret-private-key.diff holdout-lifecycle-combo.diff holdout-pr-shaped-service.diff holdout-guarded-lifecycle.diff holdout-batch-worker-combo.diff holdout-env-secret-guard.diff holdout-expanded-go-risks.diff holdout-expanded-safe-patterns.diff model-semantic.diff model-authz-bypass.diff model-nil-boundary.diff model-state-inconsistency.diff model-transaction-semantic.diff model-error-swallow.diff model-safe-semantic.diff"}" \
CR_AGENT_EVAL_MATRIX="${CR_AGENT_HOLDOUT_MATRIX:-"$ROOT/testdata/holdout/expected.tsv"}" \
CR_AGENT_EVAL_MATRIX_SOURCE=holdout \
CR_AGENT_EVAL_SKILLS_ROOT="${CR_AGENT_EVAL_SKILLS_ROOT:-"$ROOT/skills"}" \
CR_AGENT_EVAL_CONFIG="${CR_AGENT_EVAL_CONFIG:-/dev/null}" \
CR_AGENT_EVAL_RUNTIME="${CR_AGENT_EVAL_RUNTIME:-local-fallback}" \
CR_AGENT_EVAL_MODE="${CR_AGENT_EVAL_MODE:-fake-model}" \
CR_AGENT_EVAL_MIN_RECALL="${CR_AGENT_EVAL_MIN_RECALL:-1.000}" \
CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE="${CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE:-0.000}" \
"$ROOT/scripts/eval.sh"
