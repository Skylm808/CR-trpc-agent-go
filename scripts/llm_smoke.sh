#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "${CR_AGENT_LLM_SMOKE:-}" != "1" ]]; then
  echo "[SKIP] set CR_AGENT_LLM_SMOKE=1 to run live LLM smoke"
  exit 0
fi

PROVIDER="${CR_AGENT_LLM_PROVIDER:-deepseek}"
MODEL="${CR_AGENT_LLM_MODEL:-}"
BASE_URL="${CR_AGENT_LLM_BASE_URL:-}"
CONFIG="${CR_AGENT_LLM_CONFIG:-}"
if [[ -z "$CONFIG" && -f "$ROOT/cr-agent.yaml" ]]; then
  CONFIG="$ROOT/cr-agent.yaml"
fi

case "$PROVIDER" in
  deepseek)
    : "${MODEL:=deepseek-chat}"
    KEY_ENV="${CR_AGENT_LLM_API_KEY_ENV:-DEEPSEEK_API_KEY}"
    ;;
  openai|openai-compatible)
    : "${MODEL:=gpt-4o-mini}"
    KEY_ENV="${CR_AGENT_LLM_API_KEY_ENV:-OPENAI_API_KEY}"
    : "${BASE_URL:=${OPENAI_BASE_URL:-}}"
    ;;
  *)
    echo "[FAIL] unsupported CR_AGENT_LLM_PROVIDER=$PROVIDER" >&2
    exit 2
    ;;
esac

USE_CONFIG=0
if [[ -n "$CONFIG" && -f "$CONFIG" ]]; then
  USE_CONFIG=1
fi
CONFIG_API_KEY=""
CONFIG_KEY_ENV=""
EFFECTIVE_KEY_ENV="$KEY_ENV"
if [[ "$USE_CONFIG" == "1" ]]; then
  CONFIG_API_KEY="$(awk -F: '/^[[:space:]]*api_key[[:space:]]*:/ {gsub(/[[:space:]"\047]/, "", $2); print $2; exit}' "$CONFIG")"
fi

if [[ "$USE_CONFIG" == "0" && -z "${!KEY_ENV:-}" ]]; then
  echo "[SKIP] $KEY_ENV is not set"
  exit 0
fi
if [[ "$USE_CONFIG" == "1" && -z "$CONFIG_API_KEY" ]]; then
  CONFIG_KEY_ENV="$(awk -F: '/^[[:space:]]*api_key_env[[:space:]]*:/ {gsub(/[[:space:]"\047]/, "", $2); print $2; exit}' "$CONFIG")"
  if [[ -n "$CONFIG_KEY_ENV" && ! "$CONFIG_KEY_ENV" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
    echo "[SKIP] configured api_key_env is invalid; use api_key for direct keys"
    exit 0
  fi
  if [[ -n "$CONFIG_KEY_ENV" ]]; then
    EFFECTIVE_KEY_ENV="$CONFIG_KEY_ENV"
  fi
  if [[ -n "$CONFIG_KEY_ENV" && -z "${!CONFIG_KEY_ENV:-}" ]]; then
    echo "[SKIP] $CONFIG_KEY_ENV is not set"
    exit 0
  fi
  if [[ -z "$CONFIG_KEY_ENV" && -z "${!KEY_ENV:-}" ]]; then
    echo "[SKIP] $KEY_ENV is not set"
    exit 0
  fi
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

REPO="$WORK/repo"
OUT="$WORK/out"
mkdir -p "$REPO"
git -C "$REPO" init -q
git -C "$REPO" config user.email "cr-agent-smoke@example.local"
git -C "$REPO" config user.name "CR Agent Smoke"
cat > "$REPO/go.mod" <<'GOMOD'
module example.com/cragentsmoke

go 1.25.0
GOMOD
cat > "$REPO/smoke.go" <<'GO'
package smoke

func Add(a, b int) int { return a + b }
GO
git -C "$REPO" add go.mod smoke.go
git -C "$REPO" commit -q -m "base"
cat >> "$REPO/smoke.go" <<'GO'

func Divide(a, b int) int { return a / b }
GO

args=(
  run "$ROOT/cmd/review-agent"
  --repo-path "$REPO"
  --skills-root "$ROOT/skills"
  --runtime local-fallback
  --mode fake-model
  --output-dir "$OUT"
)
if [[ "$USE_CONFIG" == "1" ]]; then
  args+=(--config "$CONFIG")
else
  args+=(--model-provider "$PROVIDER" --model-name "$MODEL" --model-api-key-env "$KEY_ENV")
  if [[ -n "$BASE_URL" ]]; then
    args+=(--model-base-url "$BASE_URL")
  fi
fi

go "${args[@]}" >/dev/null

REPORT="$OUT/review_report.json"
if [[ ! -f "$REPORT" ]]; then
  echo "[FAIL] missing review_report.json" >&2
  exit 1
fi
if [[ ! -f "$OUT/review_report.md" || ! -f "$OUT/review_report.zh.md" ]]; then
  echo "[FAIL] missing Markdown review reports" >&2
  exit 1
fi
DIAGNOSTICS="$OUT/review_diagnostics.json"
if [[ ! -f "$DIAGNOSTICS" ]]; then
  echo "[FAIL] missing review_diagnostics.json" >&2
  exit 1
fi
if ! grep -q '"findings"[[:space:]]*:' "$REPORT" || ! grep -q '"input_metadata"[[:space:]]*:' "$REPORT"; then
  echo "[FAIL] report schema is incomplete" >&2
  exit 1
fi
if ! grep -q '"module_path"[[:space:]]*:[[:space:]]*"example.com/cragentsmoke"' "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] report missing git repo metadata" >&2
  exit 1
fi
if [[ -n "$EFFECTIVE_KEY_ENV" && -n "${!EFFECTIVE_KEY_ENV:-}" ]] && grep -Fq -- "${!EFFECTIVE_KEY_ENV}" "$REPORT"; then
  echo "[FAIL] report leaked API key" >&2
  exit 1
fi
if [[ -n "$CONFIG_API_KEY" ]] && grep -Fq -- "$CONFIG_API_KEY" "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] report leaked API key" >&2
  exit 1
fi
if [[ -n "$EFFECTIVE_KEY_ENV" ]] && grep -Fq -- "$EFFECTIVE_KEY_ENV" "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] report leaked API key env name" >&2
  exit 1
fi
if ! grep -q '"model_call_count"[[:space:]]*:[[:space:]]*1' "$REPORT"; then
  echo "[FAIL] report missing model_call_count=1" >&2
  exit 1
fi
if grep -q '"model-provider-failed"' "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] model provider failed; inspect $OUT" >&2
  exit 1
fi

echo "[PASS] live LLM smoke provider=$PROVIDER model=$MODEL"
