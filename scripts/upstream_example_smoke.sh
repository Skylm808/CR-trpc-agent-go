#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'USAGE'
upstream_example_smoke.sh simulates moving this repository into the official
trpc-agent-go/examples/cr-agent directory.

Usage:
  scripts/upstream_example_smoke.sh [options]

Options:
  --work-dir PATH   Use an existing temporary work directory.
  --keep            Keep the temporary work directory for inspection.
  -h, --help        Show this help.

The script copies only the upstream migration package into:
  $work_dir/trpc-agent-go/examples/cr-agent

Then it runs:
  go run ./cmd/review-agent

with cr-agent.example.yaml and verifies review_report.json,
review_report.md, and review_diagnostics.json are produced.
USAGE
}

WORK_DIR=""
KEEP=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --work-dir)
      WORK_DIR="${2:-}"
      shift 2
      ;;
    --keep)
      KEEP=1
      shift
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

if [[ -z "$WORK_DIR" ]]; then
  WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cr-agent-upstream-example.XXXXXX")"
else
  mkdir -p "$WORK_DIR"
fi

cleanup() {
  if [[ "$KEEP" != "1" ]]; then
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT

EXAMPLE="$WORK_DIR/trpc-agent-go/examples/cr-agent"
OUT="$WORK_DIR/out"
mkdir -p "$EXAMPLE" "$OUT"

copy_path() {
  local src="$1"
  local dst="$2"
  mkdir -p "$(dirname "$dst")"
  cp -R "$ROOT/$src" "$dst"
}

for path in \
	go.mod \
	go.sum \
	cmd \
	internal \
	skills \
	testdata \
	scripts/acceptance.sh \
	scripts/eval.sh \
	scripts/hidden_matrix_smoke.sh \
  scripts/llm_smoke.sh \
  scripts/repo_llm_smoke.sh \
  docs/architecture.md \
  docs/data-contract.md \
  docs/eval-matrix.md \
	docs/issue-2004-traceability.md \
	docs/sandbox-safety.md \
	docs/upstream-example-migration.md; do
	copy_path "$path" "$EXAMPLE/$path"
done

cp "$ROOT/examples/cr-agent/README.md" "$EXAMPLE/README.md"
cp "$ROOT/examples/cr-agent/cr-agent.example.yaml" "$EXAMPLE/cr-agent.example.yaml"
cp "$ROOT/examples/cr-agent/sample.diff" "$EXAMPLE/sample.diff"

(
	cd "$EXAMPLE"
	GOCACHE="${GOCACHE:-/private/tmp/cr-agent-gocache}" go run ./cmd/review-agent \
		--config ./cr-agent.example.yaml \
		--diff-file ./sample.diff \
		--skills-root ./skills \
		--runtime local-fallback \
		--output-dir "$OUT" >/dev/null
)

for name in review_report.json review_report.md review_diagnostics.json; do
  if [[ ! -f "$OUT/$name" ]]; then
    echo "[FAIL] missing $name in $OUT" >&2
    exit 1
  fi
done
if ! grep -q '"task_id"[[:space:]]*:' "$OUT/review_report.json"; then
  echo "[FAIL] review_report.json missing task_id" >&2
  exit 1
fi

echo "[PASS] upstream example smoke example_dir=$EXAMPLE output_dir=$OUT"
