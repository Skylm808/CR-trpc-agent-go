#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOCACHE="${GOCACHE:-/private/tmp/cr-agent-gocache}"
DOCKER_MODE="${CR_AGENT_ACCEPTANCE_DOCKER:-auto}"

pass() {
  printf '[PASS] %s\n' "$1"
}

skip() {
  printf '[SKIP] %s\n' "$1"
}

run_step() {
  local name="$1"
  shift
  printf '[RUN] %s\n' "$name"
  "$@"
  pass "$name"
}

cd "$ROOT"

run_step "go test ./..." env GOCACHE="$GOCACHE" go test ./...
run_step "scripts/eval.sh" env GOCACHE="$GOCACHE" scripts/eval.sh
run_step "scripts/holdout_eval.sh" env GOCACHE="$GOCACHE" scripts/holdout_eval.sh
run_step "git diff --check" git diff --check

case "$DOCKER_MODE" in
  never|skip|0|false)
    skip "container E2E disabled by CR_AGENT_ACCEPTANCE_DOCKER=$DOCKER_MODE"
    ;;
  always|1|true)
    run_step "container E2E" env CR_AGENT_RUN_CONTAINER_TESTS=1 GOCACHE="$GOCACHE" \
      go test ./internal/agent -run TestAgentRunContainerRuntimeExecutesGoChecks -count=1
    ;;
  auto|"")
    if docker info >/dev/null 2>&1; then
      run_step "container E2E" env CR_AGENT_RUN_CONTAINER_TESTS=1 GOCACHE="$GOCACHE" \
        go test ./internal/agent -run TestAgentRunContainerRuntimeExecutesGoChecks -count=1
    else
      skip "container E2E requires Docker daemon; set CR_AGENT_ACCEPTANCE_DOCKER=always to require it"
    fi
    ;;
  *)
    printf '[FAIL] unknown CR_AGENT_ACCEPTANCE_DOCKER=%s\n' "$DOCKER_MODE" >&2
    exit 2
    ;;
esac

pass "acceptance workflow"
