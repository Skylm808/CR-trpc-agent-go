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

DIFF="$WORK/smoke.diff"
OUT="$WORK/out"
cat > "$DIFF" <<'DIFF'
diff --git a/smoke.go b/smoke.go
--- a/smoke.go
+++ b/smoke.go
@@ -1,1 +1,4 @@
 package smoke
+
+func Divide(a, b int) int { return a / b }
DIFF

args=(
  run "$ROOT/cmd/review-agent"
  --diff-file "$DIFF"
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
if [[ -n "$EFFECTIVE_KEY_ENV" && -n "${!EFFECTIVE_KEY_ENV:-}" ]] && grep -Fq -- "${!EFFECTIVE_KEY_ENV}" "$REPORT"; then
  echo "[FAIL] report leaked API key" >&2
  exit 1
fi
DIAGNOSTICS="$OUT/review_diagnostics.json"
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

echo "[PASS] live LLM smoke provider=$PROVIDER model=$MODEL"
