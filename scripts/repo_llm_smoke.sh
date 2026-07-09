#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'USAGE'
repo_llm_smoke.sh verifies real LLM CR against any local git repository.

Usage:
  scripts/repo_llm_smoke.sh --repo /path/to/repo [options]

Options:
  --repo PATH         Local git repository to review.
  --config PATH       YAML config. Defaults to ./cr-agent.yaml when present.
  --go-only           Review only unstaged Go diffs by passing a temporary diff file.
  --output-dir PATH   Output directory. Defaults to /tmp/cr-agent-repo-llm-smoke-*.
  --skills-root PATH  Skills root. Defaults to this repository's skills directory.
  --runtime NAME      Runtime. Defaults to local-fallback.
  --base-ref REF      Optional base ref for git diff.
  --head-ref REF      Optional head ref for git diff.
  -h, --help          Show this help.

The script requires CR_AGENT_LLM_SMOKE=1 for live calls. It validates
review_report.md, review_report.zh.md, review_report.json, review_diagnostics.json,
model_call_count=1, a non-empty model_provider audit field, and no API key leakage
in review_report.json or review_diagnostics.json.
USAGE
}

REPO=""
CONFIG="${CR_AGENT_LLM_CONFIG:-}"
GO_ONLY=0
OUT=""
SKILLS_ROOT="$ROOT/skills"
RUNTIME="local-fallback"
BASE_REF=""
HEAD_REF=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    --config)
      CONFIG="${2:-}"
      shift 2
      ;;
    --go-only)
      GO_ONLY=1
      shift
      ;;
    --output-dir)
      OUT="${2:-}"
      shift 2
      ;;
    --skills-root)
      SKILLS_ROOT="${2:-}"
      shift 2
      ;;
    --runtime)
      RUNTIME="${2:-}"
      shift 2
      ;;
    --base-ref)
      BASE_REF="${2:-}"
      shift 2
      ;;
    --head-ref)
      HEAD_REF="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[FAIL] unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "${CR_AGENT_LLM_SMOKE:-}" != "1" ]]; then
  echo "[SKIP] set CR_AGENT_LLM_SMOKE=1 to run live repo LLM smoke"
  exit 0
fi

if [[ -z "$REPO" ]]; then
  echo "[FAIL] --repo is required" >&2
  exit 2
fi
if [[ ! -d "$REPO/.git" ]]; then
  echo "[FAIL] --repo must point to a git repository: $REPO" >&2
  exit 2
fi
if [[ -z "$CONFIG" && -f "$ROOT/cr-agent.yaml" ]]; then
  CONFIG="$ROOT/cr-agent.yaml"
fi

PROVIDER="${CR_AGENT_LLM_PROVIDER:-deepseek}"
MODEL="${CR_AGENT_LLM_MODEL:-}"
KEY_ENV=""
BASE_URL="${CR_AGENT_LLM_BASE_URL:-}"
USE_CONFIG=0
CONFIG_API_KEY=""
CONFIG_KEY_ENV=""
EFFECTIVE_KEY_ENV=""

if [[ -n "$CONFIG" && -f "$CONFIG" ]]; then
  USE_CONFIG=1
  CONFIG_API_KEY="$(awk -F: '/^[[:space:]]*api_key[[:space:]]*:/ {gsub(/[[:space:]"\047]/, "", $2); print $2; exit}' "$CONFIG")"
  CONFIG_KEY_ENV="$(awk -F: '/^[[:space:]]*api_key_env[[:space:]]*:/ {gsub(/[[:space:]"\047]/, "", $2); print $2; exit}' "$CONFIG")"
  EFFECTIVE_KEY_ENV="$CONFIG_KEY_ENV"
else
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
  EFFECTIVE_KEY_ENV="$KEY_ENV"
fi

if [[ -n "$EFFECTIVE_KEY_ENV" && ! "$EFFECTIVE_KEY_ENV" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
  echo "[FAIL] configured api_key_env is invalid" >&2
  exit 2
fi
if [[ "$USE_CONFIG" == "0" && -n "$KEY_ENV" && -z "${!KEY_ENV:-}" ]]; then
  echo "[SKIP] $KEY_ENV is not set"
  exit 0
fi
if [[ "$USE_CONFIG" == "1" && -z "$CONFIG_API_KEY" && -n "$EFFECTIVE_KEY_ENV" && -z "${!EFFECTIVE_KEY_ENV:-}" ]]; then
  echo "[SKIP] $EFFECTIVE_KEY_ENV is not set"
  exit 0
fi

if [[ -z "$OUT" ]]; then
  OUT="$(mktemp -d /tmp/cr-agent-repo-llm-smoke-XXXXXX)"
fi
mkdir -p "$OUT"

TMP_DIFF=""
cleanup() {
  if [[ -n "$TMP_DIFF" ]]; then
    rm -f "$TMP_DIFF"
  fi
}
trap cleanup EXIT

args=(
  run ./cmd/review-agent
  --repo-path "$REPO"
  --skills-root "$SKILLS_ROOT"
  --runtime "$RUNTIME"
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
if [[ -n "$BASE_REF" ]]; then
  args+=(--base-ref "$BASE_REF")
fi
if [[ -n "$HEAD_REF" ]]; then
  args+=(--head-ref "$HEAD_REF")
fi
if [[ "$GO_ONLY" == "1" ]]; then
  TMP_DIFF="$(mktemp /tmp/cr-agent-go-only-XXXXXX.diff)"
  if [[ -n "$BASE_REF" && -n "$HEAD_REF" ]]; then
    git -C "$REPO" diff --unified=3 "$BASE_REF...$HEAD_REF" -- '*.go' > "$TMP_DIFF"
  else
    git -C "$REPO" diff --unified=3 -- '*.go' > "$TMP_DIFF"
  fi
  if [[ ! -s "$TMP_DIFF" ]]; then
    echo "[FAIL] --go-only produced an empty Go diff" >&2
    exit 1
  fi
  args+=(--diff-file "$TMP_DIFF")
fi

(
  cd "$ROOT"
  GOCACHE="${GOCACHE:-/private/tmp/cr-agent-gocache}" go "${args[@]}" >/dev/null
)

REPORT="$OUT/review_report.json"
DIAGNOSTICS="$OUT/review_diagnostics.json"
MARKDOWN="$OUT/review_report.md"
MARKDOWN_ZH="$OUT/review_report.zh.md"
if [[ ! -f "$REPORT" || ! -f "$DIAGNOSTICS" || ! -f "$MARKDOWN" || ! -f "$MARKDOWN_ZH" ]]; then
  echo "[FAIL] missing review report artifacts in $OUT" >&2
  exit 1
fi
if ! grep -q '"model_call_count"[[:space:]]*:[[:space:]]*1' "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] expected model_call_count=1" >&2
  exit 1
fi
if ! grep -q '"model_provider"[[:space:]]*:[[:space:]]*"[^"]' "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] expected non-empty model_provider audit field" >&2
  exit 1
fi
if grep -q '"model-provider-failed"' "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] model provider failed; inspect $OUT" >&2
  exit 1
fi
if [[ -n "$CONFIG_API_KEY" ]] && grep -Fq -- "$CONFIG_API_KEY" "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] report leaked API key" >&2
  exit 1
fi
if [[ -n "$EFFECTIVE_KEY_ENV" && -n "${!EFFECTIVE_KEY_ENV:-}" ]] && grep -Fq -- "${!EFFECTIVE_KEY_ENV}" "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] report leaked API key" >&2
  exit 1
fi
if [[ -n "$EFFECTIVE_KEY_ENV" ]] && grep -Fq -- "$EFFECTIVE_KEY_ENV" "$REPORT" "$DIAGNOSTICS"; then
  echo "[FAIL] report leaked API key env name" >&2
  exit 1
fi

echo "[PASS] repo LLM smoke output_dir=$OUT"
