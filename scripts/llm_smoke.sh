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

if [[ -z "${!KEY_ENV:-}" ]]; then
  echo "[SKIP] $KEY_ENV is not set"
  exit 0
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
  --model-provider "$PROVIDER"
  --model-name "$MODEL"
  --model-api-key-env "$KEY_ENV"
  --output-dir "$OUT"
)
if [[ -n "$BASE_URL" ]]; then
  args+=(--model-base-url "$BASE_URL")
fi

go "${args[@]}" >/dev/null

REPORT="$OUT/review_report.json"
if [[ ! -f "$REPORT" ]]; then
  echo "[FAIL] missing review_report.json" >&2
  exit 1
fi
if grep -Fq -- "${!KEY_ENV}" "$REPORT"; then
  echo "[FAIL] report leaked API key" >&2
  exit 1
fi
if ! grep -q '"model_call_count"[[:space:]]*:[[:space:]]*1' "$REPORT"; then
  echo "[FAIL] report missing model_call_count=1" >&2
  exit 1
fi

echo "[PASS] live LLM smoke provider=$PROVIDER model=$MODEL"
