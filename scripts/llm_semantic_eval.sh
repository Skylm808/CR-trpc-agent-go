#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'USAGE'
llm_semantic_eval.sh runs opt-in live LLM semantic evaluation on fixed fixtures.

Usage:
  CR_AGENT_LLM_SMOKE=1 scripts/llm_semantic_eval.sh [options]

Options:
  --config PATH        YAML config. Defaults to CR_AGENT_LLM_CONFIG or ./cr-agent.yaml.
  --fixtures LIST     Space-separated fixture names. Defaults to model semantic holdouts.
  --fixtures-root DIR Fixture root. Defaults to testdata/holdout.
  --expected-file PATH
                       TSV expectations. Defaults to testdata/holdout/expected.tsv.
  --output-root DIR   Output root. Defaults to /tmp/cr-agent-llm-semantic-*.
  --skills-root DIR   Skills root. Defaults to ./skills.
  --runtime NAME      Runtime. Defaults to local-fallback.
  -h, --help          Show this help.

The script preserves each fixture's review_report.json, review_report.md,
review_report.zh.md, and review_diagnostics.json, then writes readable summaries:
  llm_semantic_eval.md     index
  llm_semantic_eval.zh.md  Chinese summary
  llm_semantic_eval.en.md  English summary

It measures what the live provider returns; it is not a deterministic CI gate.
If expectations are available, summaries also include fixture-level recall and
safe-fixture false-positive counts.
USAGE
}

CONFIG="${CR_AGENT_LLM_CONFIG:-}"
FIXTURES="${CR_AGENT_LLM_SEMANTIC_FIXTURES:-model-semantic.diff model-authz-bypass.diff model-nil-boundary.diff model-state-inconsistency.diff model-transaction-semantic.diff model-error-swallow.diff model-safe-semantic.diff}"
FIXTURES_ROOT="$ROOT/testdata/holdout"
EXPECTED_FILE="$ROOT/testdata/holdout/expected.tsv"
OUTPUT_ROOT=""
SKILLS_ROOT="$ROOT/skills"
RUNTIME="local-fallback"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --config)
      CONFIG="${2:-}"
      shift 2
      ;;
    --fixtures)
      FIXTURES="${2:-}"
      shift 2
      ;;
    --fixtures-root)
      FIXTURES_ROOT="${2:-}"
      shift 2
      ;;
    --expected-file)
      EXPECTED_FILE="${2:-}"
      shift 2
      ;;
    --output-root)
      OUTPUT_ROOT="${2:-}"
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
  echo "[SKIP] set CR_AGENT_LLM_SMOKE=1 to run live LLM semantic eval"
  exit 0
fi

if [[ -z "$CONFIG" && -f "$ROOT/cr-agent.yaml" ]]; then
  CONFIG="$ROOT/cr-agent.yaml"
fi

PROVIDER="${CR_AGENT_LLM_PROVIDER:-deepseek}"
MODEL="${CR_AGENT_LLM_MODEL:-}"
BASE_URL="${CR_AGENT_LLM_BASE_URL:-}"
KEY_ENV=""
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
  echo "[SKIP] configured api_key_env is invalid"
  exit 0
fi
if [[ "$USE_CONFIG" == "0" && -n "$KEY_ENV" && -z "${!KEY_ENV:-}" ]]; then
  echo "[SKIP] $KEY_ENV is not set"
  exit 0
fi
if [[ "$USE_CONFIG" == "1" && -z "$CONFIG_API_KEY" && -n "$EFFECTIVE_KEY_ENV" && -z "${!EFFECTIVE_KEY_ENV:-}" ]]; then
  echo "[SKIP] $EFFECTIVE_KEY_ENV is not set"
  exit 0
fi

if [[ -z "$OUTPUT_ROOT" ]]; then
  OUTPUT_ROOT="$(mktemp -d /tmp/cr-agent-llm-semantic-XXXXXX)"
fi
mkdir -p "$OUTPUT_ROOT"
SUMMARY="$OUTPUT_ROOT/llm_semantic_eval.md"
SUMMARY_ZH="$OUTPUT_ROOT/llm_semantic_eval.zh.md"
SUMMARY_EN="$OUTPUT_ROOT/llm_semantic_eval.en.md"

cat > "$SUMMARY" <<'MD'
# Live LLM Semantic Evaluation / 真实 LLM 语义评测

- [中文报告](llm_semantic_eval.zh.md)
- [English report](llm_semantic_eval.en.md)

This index points to separate Chinese and English summaries for readability.
It records live provider behavior, not a deterministic CI gate.

| Fixture | Provider | Model calls | Model items | Model rule IDs | Status | Report |
| --- | --- | ---: | ---: | --- | --- | --- |
MD

cat > "$SUMMARY_ZH" <<'MD'
# 真实 LLM 语义评测

| Fixture | Provider | Model calls | Model items | Model rule IDs | Status | Report |
| --- | --- | ---: | ---: | --- | --- | --- |
MD

cat > "$SUMMARY_EN" <<'MD'
# Live LLM Semantic Evaluation

This report records what the live model returned on fixed semantic fixtures.
It proves provider connectivity and captures model findings for review, but it
is not a deterministic CI gate.

| Fixture | Provider | Model calls | Model items | Model rule IDs | Status | Report |
| --- | --- | ---: | ---: | --- | --- | --- |
MD

total=0
model_items_total=0
expected_risky_total=0
detected_risky_total=0
expected_clean_or_unlisted_total=0
false_positive_fixture_total=0

for fixture in $FIXTURES; do
  diff_path="$FIXTURES_ROOT/$fixture"
  if [[ ! -f "$diff_path" ]]; then
    echo "[FAIL] fixture does not exist: $diff_path" >&2
    exit 1
  fi
  case_dir="$OUTPUT_ROOT/$fixture"
  mkdir -p "$case_dir"
  args=(
    run "$ROOT/cmd/review-agent"
    --diff-file "$diff_path"
    --skills-root "$SKILLS_ROOT"
    --runtime "$RUNTIME"
    --mode fake-model
    --output-dir "$case_dir"
  )
  if [[ "$USE_CONFIG" == "1" ]]; then
    args+=(--config "$CONFIG")
  else
    args+=(--model-provider "$PROVIDER" --model-name "$MODEL" --model-api-key-env "$KEY_ENV")
    if [[ -n "$BASE_URL" ]]; then
      args+=(--model-base-url "$BASE_URL")
    fi
  fi

  GOCACHE="${GOCACHE:-/private/tmp/cr-agent-gocache}" go "${args[@]}" >/dev/null

  report="$case_dir/review_report.json"
  diagnostics="$case_dir/review_diagnostics.json"
  markdown="$case_dir/review_report.md"
  markdown_zh="$case_dir/review_report.zh.md"
  if [[ ! -f "$report" || ! -f "$diagnostics" || ! -f "$markdown" || ! -f "$markdown_zh" ]]; then
    echo "[FAIL] missing report artifacts for $fixture" >&2
    exit 1
  fi
  if ! grep -q '"model_call_count"[[:space:]]*:[[:space:]]*1' "$report" "$diagnostics"; then
    echo "[FAIL] expected model_call_count=1 for $fixture" >&2
    exit 1
  fi
  if grep -q '"model-provider-failed"' "$report" "$diagnostics"; then
    echo "[FAIL] model provider failed for $fixture; inspect $case_dir" >&2
    exit 1
  fi
  if [[ -n "$CONFIG_API_KEY" ]] && grep -Fq -- "$CONFIG_API_KEY" "$report" "$diagnostics"; then
    echo "[FAIL] report leaked API key for $fixture" >&2
    exit 1
  fi
  if [[ -n "$EFFECTIVE_KEY_ENV" && -n "${!EFFECTIVE_KEY_ENV:-}" ]] && grep -Fq -- "${!EFFECTIVE_KEY_ENV}" "$report" "$diagnostics"; then
    echo "[FAIL] report leaked API key for $fixture" >&2
    exit 1
  fi
  if [[ -n "$EFFECTIVE_KEY_ENV" ]] && grep -Fq -- "$EFFECTIVE_KEY_ENV" "$report" "$diagnostics"; then
    echo "[FAIL] report leaked API key env name for $fixture" >&2
    exit 1
  fi

  row="$(ruby -rjson -e '
    path, fixture, case_dir = ARGV
    data = JSON.parse(File.read(path))
    metrics = data.fetch("metrics", {})
    seen = {}
    items = []
    %w[findings warnings human_review_items].each do |key|
      data.fetch(key, []).each do |item|
        next unless item["source"] == "model"
        dedupe = [item["rule_id"], item["file"], item["line"], item["category"], item["status"]].join("\0")
        next if seen[dedupe]
        seen[dedupe] = true
        items << item
      end
    end
    ids = items.map { |item| item["rule_id"].to_s }.reject(&:empty?).uniq.join(", ")
    ids = "-" if ids.empty?
    provider = [metrics["model_provider"], metrics["model_name"]].compact.reject(&:empty?).join("/")
    provider = "-" if provider.empty?
    status = items.empty? ? "no model finding" : "model finding"
    puts [fixture, provider, metrics["model_call_count"].to_i, items.length, ids, status, case_dir].join("\t")
  ' "$report" "$fixture" "$case_dir")"
  IFS=$'\t' read -r row_fixture row_provider row_calls row_items row_ids row_status row_dir <<< "$row"
  expected_count=0
  if [[ -n "$EXPECTED_FILE" && -f "$EXPECTED_FILE" ]]; then
    expected_count="$(awk -v fixture="$fixture" 'BEGIN { count = 0 } $1 == fixture && $5 == "true" { count++ } END { print count }' "$EXPECTED_FILE")"
  fi
  expected_label="clean-or-unlisted"
  if [[ "$expected_count" -gt 0 ]]; then
    expected_label="risk"
    expected_risky_total=$((expected_risky_total + 1))
    if [[ "$row_items" -gt 0 ]]; then
      detected_risky_total=$((detected_risky_total + 1))
    fi
  else
    expected_clean_or_unlisted_total=$((expected_clean_or_unlisted_total + 1))
    if [[ "$row_items" -gt 0 ]]; then
      false_positive_fixture_total=$((false_positive_fixture_total + 1))
    fi
  fi
  printf '| `%s` | %s | %s | %s | %s | %s | `%s` |\n' \
    "$row_fixture" "$row_provider" "$row_calls" "$row_items" "$row_ids" "$row_status ($expected_label)" "$row_dir" >> "$SUMMARY"
  printf '| `%s` | %s | %s | %s | %s | %s | `%s` |\n' \
    "$row_fixture" "$row_provider" "$row_calls" "$row_items" "$row_ids" "$row_status ($expected_label)" "$row_dir" >> "$SUMMARY_ZH"
  printf '| `%s` | %s | %s | %s | %s | %s | `%s` |\n' \
    "$row_fixture" "$row_provider" "$row_calls" "$row_items" "$row_ids" "$row_status ($expected_label)" "$row_dir" >> "$SUMMARY_EN"
  total=$((total + 1))
  model_items_total=$((model_items_total + row_items))
done

recall_pct="$(ruby -e 'detected,total=ARGV.map(&:to_f); puts total.zero? ? "n/a" : format("%.1f%%", detected * 100.0 / total)' "$detected_risky_total" "$expected_risky_total")"
safe_fp_pct="$(ruby -e 'fp,total=ARGV.map(&:to_f); puts total.zero? ? "n/a" : format("%.1f%%", fp * 100.0 / total)' "$false_positive_fixture_total" "$expected_clean_or_unlisted_total")"

cat >> "$SUMMARY" <<'MD'

## Metrics
MD
cat >> "$SUMMARY" <<MD

- Fixtures: $total
- Model items: $model_items_total
- Expected risky fixtures: $expected_risky_total
- Detected risky fixtures: $detected_risky_total
- Fixture-level recall: $recall_pct
- Expected-clean or unlisted fixtures: $expected_clean_or_unlisted_total
- Safe-fixture false positives: $false_positive_fixture_total ($safe_fp_pct)
MD

cat >> "$SUMMARY" <<'MD'

Summary files are generated next to this index:

- `llm_semantic_eval.zh.md`
- `llm_semantic_eval.en.md`
MD

cat >> "$SUMMARY_ZH" <<'MD'

## 指标
MD
cat >> "$SUMMARY_ZH" <<MD

- Fixture 总数: $total
- 模型增量项: $model_items_total
- 预期有语义风险的 fixtures: $expected_risky_total
- 被模型命中的风险 fixtures: $detected_risky_total
- Fixture-level recall: $recall_pct
- 预期干净或未列入 expected 的 fixtures: $expected_clean_or_unlisted_total
- Safe-fixture 误报: $false_positive_fixture_total ($safe_fp_pct)
MD

cat >> "$SUMMARY_ZH" <<'MD'

## 说明

- 本报告记录真实模型在固定语义样本上的实际输出，用于证明真实 provider
  链路可用并保留模型发现项，不能替代 deterministic CI 门禁。
- 当 provider 返回高置信 `source=model` 项时，`review_report.md` 和
  `review_report.zh.md` 会写入模型 finding。
- `review_report.json` 和 `review_diagnostics.json` 会记录 `model_provider`、
  `model_name`、`model_backend` 和 `model_call_count`。
- `model finding` 表示真实模型返回了至少一条增量语义项；`no model finding`
  也有意义，说明模型链路跑通但该样本没有新增语义发现。
MD

cat >> "$SUMMARY_EN" <<'MD'

## Metrics
MD
cat >> "$SUMMARY_EN" <<MD

- Fixtures: $total
- Model items: $model_items_total
- Expected risky fixtures: $expected_risky_total
- Detected risky fixtures: $detected_risky_total
- Fixture-level recall: $recall_pct
- Expected-clean or unlisted fixtures: $expected_clean_or_unlisted_total
- Safe-fixture false positives: $false_positive_fixture_total ($safe_fp_pct)
MD

cat >> "$SUMMARY_EN" <<'MD'

## Notes

- `review_report.md` and `review_report.zh.md` contain the model findings when
  the provider returns high-confidence `source=model` items.
- `review_report.json` and `review_diagnostics.json` contain the audit fields
  `model_provider`, `model_name`, `model_backend`, and `model_call_count`.
- `model finding` means the live model returned at least one incremental item
  with `source=model`; `no model finding` is still useful evidence that the
  provider was called but did not add semantic findings for that fixture.
- Fixture-level recall treats a risky fixture as detected when the live model
  returns at least one incremental model item for that fixture. It does not
  claim line-level or rule-ID equivalence.
MD

echo "fixtures=$total model_items=$model_items_total detected_risky=$detected_risky_total expected_risky=$expected_risky_total recall=$recall_pct summary=$SUMMARY summary_zh=$SUMMARY_ZH summary_en=$SUMMARY_EN output_root=$OUTPUT_ROOT"
echo "[PASS] live LLM semantic eval"
