#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURES_ROOT="${CR_AGENT_EVAL_FIXTURES_ROOT:-"$ROOT/testdata/fixtures"}"
SKILLS_ROOT="${CR_AGENT_EVAL_SKILLS_ROOT:-"$ROOT/skills"}"
RUNTIME="${CR_AGENT_EVAL_RUNTIME:-local-fallback}"
MODE="${CR_AGENT_EVAL_MODE:-review}"
MODEL_ENABLED="${CR_AGENT_EVAL_MODEL_ENABLED:-false}"
CONFIG="${CR_AGENT_EVAL_CONFIG:-/dev/null}"
# Fault-injection fixtures belong to acceptance tests, not the accuracy matrix:
# an unexpected infrastructure failure is always a hard evaluation failure.
FIXTURES="${CR_AGENT_EVAL_FIXTURES:-safe.diff secret.diff secret-shapes.diff panic.diff todo.diff test-missing.diff missing-test.diff goroutine.diff context.diff resource.diff db-lifecycle.diff http-body.diff sql-string-concat.diff command-injection.diff context-background.diff mutex-unlock.diff defer-in-loop.diff bare-return-err.diff string-concat-loop.diff dedupe.diff realistic-service-risk.diff}"
MATRIX_OVERRIDE="${CR_AGENT_EVAL_MATRIX:-}"
EXPECTED_OVERRIDE="${CR_AGENT_EVAL_EXPECTED:-}"
MATRIX_SOURCE_OVERRIDE="${CR_AGENT_EVAL_MATRIX_SOURCE:-}"
REPORT_ROOT="${CR_AGENT_EVAL_REPORT_ROOT:-}"
EVAL_BINARY="${CR_AGENT_EVAL_BINARY:-}"
MIN_RECALL="${CR_AGENT_EVAL_MIN_RECALL:-0.800}"
MAX_FALSE_POSITIVE_RATE="${CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE:-0.150}"
if [[ -n "$REPORT_ROOT" ]]; then
  OUT_ROOT="$REPORT_ROOT"
  mkdir -p "$OUT_ROOT"
else
  OUT_ROOT="$(mktemp -d)"
fi
START_SECONDS="$(date +%s)"
LOCK_DIR="${CR_AGENT_EVAL_LOCK_DIR:-/private/tmp/cr-agent-eval.lock}"
while ! mkdir "$LOCK_DIR" 2>/dev/null; do
  if [[ -f "$LOCK_DIR/pid" ]]; then
    lock_pid="$(cat "$LOCK_DIR/pid" 2>/dev/null || true)"
    if [[ -n "$lock_pid" ]] && ! kill -0 "$lock_pid" 2>/dev/null; then
      rm -rf "$LOCK_DIR"
      continue
    fi
  fi
  sleep 0.1
done
printf '%s\n' "$$" > "$LOCK_DIR/pid"

cleanup() {
  rm -rf "$LOCK_DIR"
  if [[ -z "$REPORT_ROOT" ]]; then
    rm -rf "$OUT_ROOT"
  fi
}
trap cleanup EXIT

EXPECTED_FILE="$OUT_ROOT/expected.tsv"
MATRIX_SOURCE="builtin"
if [[ -n "$MATRIX_OVERRIDE" ]]; then
  if [[ ! -f "$MATRIX_OVERRIDE" ]]; then
    echo "CR_AGENT_EVAL_MATRIX does not exist: $MATRIX_OVERRIDE" >&2
    exit 2
  fi
  EXPECTED_FILE="$MATRIX_OVERRIDE"
  MATRIX_SOURCE="external"
elif [[ -n "$EXPECTED_OVERRIDE" ]]; then
  if [[ ! -f "$EXPECTED_OVERRIDE" ]]; then
    echo "CR_AGENT_EVAL_EXPECTED does not exist: $EXPECTED_OVERRIDE" >&2
    exit 2
  fi
  EXPECTED_FILE="$EXPECTED_OVERRIDE"
  MATRIX_SOURCE="external"
else
  cat > "$EXPECTED_FILE" <<'TSV'
secret.diff	secret-leak	critical	finding	true
secret-shapes.diff	secret-leak	critical	finding	true
panic.diff	panic-direct	high	finding	true
todo.diff	todo-marker	medium	finding	true
test-missing.diff	missing-test-hint	low	warning	true
missing-test.diff	missing-test-hint	low	warning	true
goroutine.diff	goroutine-leak	high	finding	true
context.diff	context-leak	high	finding	true
resource.diff	resource-leak	high	finding	true
db-lifecycle.diff	db-lifecycle	high	finding	true
http-body.diff	http-body-close	high	finding	true
sql-string-concat.diff	sql-string-concat	critical	finding	true
command-injection.diff	command-injection	critical	finding	true
context-background.diff	context-background-misuse	medium	finding	true
mutex-unlock.diff	mutex-unlock-missing	high	finding	true
defer-in-loop.diff	defer-in-loop	medium	finding	true
bare-return-err.diff	bare-return-err	medium	finding	true
string-concat-loop.diff	string-concat-loop	low	needs_human_review	true
dedupe.diff	panic-direct	high	finding	true
realistic-service-risk.diff	secret-leak	critical	finding	true
realistic-service-risk.diff	panic-direct	high	finding	true
realistic-service-risk.diff	goroutine-leak	high	finding	true
realistic-service-risk.diff	context-leak	high	finding	true
realistic-service-risk.diff	resource-leak	high	finding	true
realistic-service-risk.diff	db-lifecycle	high	finding	true
realistic-service-risk.diff	todo-marker	medium	finding	true
realistic-service-risk.diff	missing-test-hint	low	warning	true
realistic-service-risk.diff	string-concat-loop	low	needs_human_review	true
TSV
fi
if [[ -n "$MATRIX_SOURCE_OVERRIDE" ]]; then
  MATRIX_SOURCE="$MATRIX_SOURCE_OVERRIDE"
fi

if [[ -z "$EVAL_BINARY" ]]; then
  EVAL_BINARY="$OUT_ROOT/review-agent"
  (cd "$ROOT" && go build -o "$EVAL_BINARY" ./cmd/review-agent)
elif [[ ! -x "$EVAL_BINARY" ]]; then
  echo "CR_AGENT_EVAL_BINARY is not executable: $EVAL_BINARY" >&2
  exit 2
fi

for fixture in $FIXTURES; do
  mkdir -p "$OUT_ROOT/$fixture"
  "$EVAL_BINARY" \
    --config "$CONFIG" \
    --fixture "$fixture" \
    --fixtures-root "$FIXTURES_ROOT" \
    --skills-root "$SKILLS_ROOT" \
    --runtime "$RUNTIME" \
	--mode "$MODE" \
	--model-enabled="$MODEL_ENABLED" \
    --output-dir "$OUT_ROOT/$fixture" >/dev/null
done

HELPER="$OUT_ROOT/eval.go"
cat > "$HELPER" <<'GO'
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type item struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

type report struct {
	Findings        []item `json:"findings"`
	Warnings        []item `json:"warnings"`
	HumanReviewItems []item `json:"human_review_items"`
	Metrics struct {
		ExceptionCounts map[string]int `json:"exception_counts"`
	} `json:"metrics"`
}

func main() {
	if len(os.Args) < 8 {
		fmt.Fprintln(os.Stderr, "usage: eval <expected.tsv> <out-root> <duration-ms> <matrix-source> <min-recall> <max-fpr> <fixtures...>")
		os.Exit(2)
	}
	expectedFile := os.Args[1]
	outRoot := os.Args[2]
	durationMS := os.Args[3]
	matrixSource := os.Args[4]
	minRecall, err := parseThreshold(os.Args[5], "min recall")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	maxFalsePositiveRate, err := parseThreshold(os.Args[6], "max false positive rate")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fixtures := os.Args[7:]

	expected, err := readExpected(expectedFile)
	if err != nil {
		panic(err)
	}
	fixtureSet := map[string]bool{}
	for _, fixture := range fixtures {
		fixtureSet[fixture] = true
	}

	requiredExpected := 0
	optionalExpected := 0
	truePositive := 0
	falsePositive := 0
	falseNegative := 0
	var missing []string
	var unexpected []string
	var infrastructureFailures []string
	for _, fixture := range fixtures {
		want := expected[fixture]
		for _, entry := range want {
			if entry.Required {
				requiredExpected++
			} else {
				optionalExpected++
			}
		}
		got, infrastructureFailure, err := readActual(filepath.Join(outRoot, fixture, "review_report.json"))
		if err != nil {
			panic(err)
		}
		if infrastructureFailure {
			infrastructureFailures = append(infrastructureFailures, fixture)
		}
		for key := range got {
			if entry, ok := want[key]; ok && entry.Required {
				truePositive++
			} else if _, ok := want[key]; ok {
				continue
			} else {
				falsePositive++
				unexpected = append(unexpected, fixture+"|"+key)
			}
		}
		for key, entry := range want {
			if entry.Required && !got[key] {
				falseNegative++
				missing = append(missing, fixture+"|"+key)
			}
		}
	}
	recall := ratio(truePositive, truePositive+falseNegative)
	precision := ratio(truePositive, truePositive+falsePositive)
	falsePositiveRate := ratioZero(falsePositive, truePositive+falsePositive)
	fmt.Printf("fixtures=%d expected=%d required_expected=%d optional_expected=%d true_positive=%d false_positive=%d false_negative=%d recall=%.3f precision=%.3f false_positive_rate=%.3f missing_findings=%d unexpected_findings=%d infrastructure_failures=%d duration_ms=%s matrix_source=%s\n",
		len(fixtureSet), requiredExpected+optionalExpected, requiredExpected, optionalExpected, truePositive, falsePositive, falseNegative, recall, precision, falsePositiveRate, len(missing), len(unexpected), len(infrastructureFailures), durationMS, matrixSource)
	if len(missing) > 0 {
		fmt.Printf("missing=%s\n", strings.Join(missing, ","))
	}
	if len(unexpected) > 0 {
		fmt.Printf("unexpected=%s\n", strings.Join(unexpected, ","))
	}
	if len(infrastructureFailures) > 0 {
		fmt.Printf("infrastructure_failure_fixtures=%s\n", strings.Join(infrastructureFailures, ","))
		fmt.Printf("threshold_failed=infrastructure_failure count=%d\n", len(infrastructureFailures))
		os.Exit(1)
	}
	if recall < minRecall {
		fmt.Printf("threshold_failed=recall actual=%.3f min=%.3f\n", recall, minRecall)
		os.Exit(1)
	}
	if falsePositiveRate > maxFalsePositiveRate {
		fmt.Printf("threshold_failed=false_positive_rate actual=%.3f max=%.3f\n", falsePositiveRate, maxFalsePositiveRate)
		os.Exit(1)
	}
	if matrixSource == "builtin" && (falsePositive > 0 || falseNegative > 0) {
		fmt.Printf("threshold_failed=public_matrix_exact_match missing_findings=%d unexpected_findings=%d\n", len(missing), len(unexpected))
		os.Exit(1)
	}
}

type expectedItem struct {
	Required bool
}

func readExpected(path string) (map[string]map[string]expectedItem, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := map[string]map[string]expectedItem{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 && len(fields) != 5 {
			return nil, fmt.Errorf("invalid expected row %q: want 4 or 5 tab-separated fields", line)
		}
		fixture := fields[0]
		if out[fixture] == nil {
			out[fixture] = map[string]expectedItem{}
		}
		required := true
		if len(fields) == 5 {
			switch strings.ToLower(strings.TrimSpace(fields[4])) {
			case "true", "required", "yes", "1", "must":
				required = true
			case "false", "optional", "no", "0":
				required = false
			default:
				return nil, fmt.Errorf("invalid required value %q in row %q", fields[4], line)
			}
		}
		out[fixture][strings.Join(fields[1:4], "|")] = expectedItem{Required: required}
	}
	return out, scanner.Err()
}

func parseThreshold(raw, name string) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	if value < 0 || value > 1 {
		return 0, fmt.Errorf("invalid %s %.3f: want 0..1", name, value)
	}
	return value, nil
}

func readActual(path string) (map[string]bool, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var rep report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, false, err
	}
	out := map[string]bool{}
	for _, entry := range append(rep.Findings, rep.Warnings...) {
		if entry.RuleID == "" {
			continue
		}
		out[entry.RuleID+"|"+entry.Severity+"|"+entry.Status] = true
	}
	infrastructureFailure := rep.Metrics.ExceptionCounts["sandbox_failed"] > 0 || rep.Metrics.ExceptionCounts["sandbox_timeout"] > 0
	for _, entry := range rep.HumanReviewItems {
		if entry.RuleID == "sandbox-command-failed" || entry.RuleID == "sandbox-command-timeout" {
			infrastructureFailure = true
		}
	}
	return out, infrastructureFailure, nil
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}

func ratioZero(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
GO

DURATION_MS="$(( ($(date +%s) - START_SECONDS) * 1000 ))"
go run "$HELPER" "$EXPECTED_FILE" "$OUT_ROOT" "$DURATION_MS" "$MATRIX_SOURCE" "$MIN_RECALL" "$MAX_FALSE_POSITIVE_RATE" $FIXTURES
