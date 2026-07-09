#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FIXTURES_ROOT="$WORK/fixtures"
MATRIX="$WORK/expected.tsv"
REPORT_ROOT="${CR_AGENT_HIDDEN_SMOKE_REPORT_ROOT:-}"
if [[ -z "$REPORT_ROOT" ]]; then
  REPORT_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/cr-agent-hidden-smoke-reports.XXXXXX")"
fi

mkdir -p "$FIXTURES_ROOT" "$REPORT_ROOT"
cp "$ROOT/testdata/fixtures/safe.diff" "$FIXTURES_ROOT/safe.diff"
cp "$ROOT/testdata/fixtures/secret.diff" "$FIXTURES_ROOT/secret.diff"

cat > "$MATRIX" <<'TSV'
secret.diff	secret-leak	critical	finding	true
TSV

set +e
OUTPUT="$(
  CR_AGENT_EVAL_FIXTURES_ROOT="$FIXTURES_ROOT" \
  CR_AGENT_EVAL_FIXTURES="safe.diff secret.diff" \
  CR_AGENT_EVAL_MATRIX="$MATRIX" \
  CR_AGENT_EVAL_REPORT_ROOT="$REPORT_ROOT" \
  CR_AGENT_EVAL_CONFIG=/dev/null \
  CR_AGENT_EVAL_MIN_RECALL=1.000 \
  CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE=0.000 \
  "$ROOT/scripts/eval.sh" 2>&1
)"
STATUS=$?
set -e

if [[ $STATUS -ne 0 ]]; then
  echo "[FAIL] hidden-like matrix smoke failed" >&2
  echo "$OUTPUT" >&2
  echo "report_root=$REPORT_ROOT" >&2
  exit "$STATUS"
fi

for fixture in safe.diff secret.diff; do
  for artifact in review_report.json review_report.md review_report.zh.md review_diagnostics.json; do
    if [[ ! -f "$REPORT_ROOT/$fixture/$artifact" ]]; then
      echo "[FAIL] missing retained artifact: $REPORT_ROOT/$fixture/$artifact" >&2
      echo "$OUTPUT" >&2
      echo "report_root=$REPORT_ROOT" >&2
      exit 1
    fi
  done
done

for want in "fixtures=2" "recall=1.000" "precision=1.000" "false_positive_rate=0.000" "matrix_source=external"; do
  if [[ "$OUTPUT" != *"$want"* ]]; then
    echo "[FAIL] hidden-like matrix smoke output missing $want" >&2
    echo "$OUTPUT" >&2
    echo "report_root=$REPORT_ROOT" >&2
    exit 1
  fi
done

echo "$OUTPUT"
echo "[PASS] hidden-like matrix smoke report_root=$REPORT_ROOT"
