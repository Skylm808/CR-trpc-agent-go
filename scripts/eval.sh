#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURES_ROOT="${CR_AGENT_EVAL_FIXTURES_ROOT:-"$ROOT/testdata/fixtures"}"
SKILLS_ROOT="${CR_AGENT_EVAL_SKILLS_ROOT:-"$ROOT/skills"}"
RUNTIME="${CR_AGENT_EVAL_RUNTIME:-local-fallback}"
MODE="${CR_AGENT_EVAL_MODE:-rule-only}"
FIXTURES="${CR_AGENT_EVAL_FIXTURES:-safe.diff secret.diff panic.diff todo.diff test-missing.diff missing-test.diff goroutine.diff context.diff resource.diff db-lifecycle.diff dedupe.diff sandbox-fail.diff sandbox-timeout.diff}"
EXPECTED_OVERRIDE="${CR_AGENT_EVAL_EXPECTED:-}"
REPORT_ROOT="${CR_AGENT_EVAL_REPORT_ROOT:-}"
if [[ -n "$REPORT_ROOT" ]]; then
  OUT_ROOT="$REPORT_ROOT"
  mkdir -p "$OUT_ROOT"
else
  OUT_ROOT="$(mktemp -d)"
fi
START_SECONDS="$(date +%s)"
if [[ -z "$REPORT_ROOT" ]]; then
  trap 'rm -rf "$OUT_ROOT"' EXIT
fi

EXPECTED_FILE="$OUT_ROOT/expected.tsv"
MATRIX_SOURCE="builtin"
if [[ -n "$EXPECTED_OVERRIDE" ]]; then
  EXPECTED_FILE="$EXPECTED_OVERRIDE"
  MATRIX_SOURCE="external"
else
  cat > "$EXPECTED_FILE" <<'TSV'
secret.diff	secret-leak	critical	finding	true
panic.diff	panic-direct	high	finding	true
todo.diff	todo-marker	medium	finding	true
test-missing.diff	missing-test-hint	low	warning	true
missing-test.diff	missing-test-hint	low	warning	true
goroutine.diff	goroutine-leak	high	finding	true
context.diff	context-leak	high	finding	true
resource.diff	resource-leak	high	finding	true
db-lifecycle.diff	db-lifecycle	high	finding	true
dedupe.diff	panic-direct	high	finding	true
TSV
fi

for fixture in $FIXTURES; do
  mkdir -p "$OUT_ROOT/$fixture"
  go run "$ROOT/cmd/review-agent" \
    --fixture "$fixture" \
    --fixtures-root "$FIXTURES_ROOT" \
    --skills-root "$SKILLS_ROOT" \
    --runtime "$RUNTIME" \
    --mode "$MODE" \
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
	"strings"
)

type item struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

type report struct {
	Findings []item `json:"findings"`
	Warnings []item `json:"warnings"`
}

func main() {
	if len(os.Args) < 6 {
		fmt.Fprintln(os.Stderr, "usage: eval <expected.tsv> <out-root> <duration-ms> <matrix-source> <fixtures...>")
		os.Exit(2)
	}
	expectedFile := os.Args[1]
	outRoot := os.Args[2]
	durationMS := os.Args[3]
	matrixSource := os.Args[4]
	fixtures := os.Args[5:]

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
	for _, fixture := range fixtures {
		want := expected[fixture]
		for _, entry := range want {
			if entry.Required {
				requiredExpected++
			} else {
				optionalExpected++
			}
		}
		got, err := readActual(filepath.Join(outRoot, fixture, "review_report.json"))
		if err != nil {
			panic(err)
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
	fmt.Printf("fixtures=%d expected=%d required_expected=%d optional_expected=%d true_positive=%d false_positive=%d false_negative=%d recall=%.3f precision=%.3f false_positive_rate=%.3f missing_findings=%d unexpected_findings=%d duration_ms=%s matrix_source=%s\n",
		len(fixtureSet), requiredExpected+optionalExpected, requiredExpected, optionalExpected, truePositive, falsePositive, falseNegative, recall, precision, falsePositiveRate, len(missing), len(unexpected), durationMS, matrixSource)
	if len(missing) > 0 {
		fmt.Printf("missing=%s\n", strings.Join(missing, ","))
	}
	if len(unexpected) > 0 {
		fmt.Printf("unexpected=%s\n", strings.Join(unexpected, ","))
	}
	if falsePositive > 0 || falseNegative > 0 {
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

func readActual(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rep report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, entry := range append(rep.Findings, rep.Warnings...) {
		if entry.RuleID == "" {
			continue
		}
		out[entry.RuleID+"|"+entry.Severity+"|"+entry.Status] = true
	}
	return out, nil
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
go run "$HELPER" "$EXPECTED_FILE" "$OUT_ROOT" "$DURATION_MS" "$MATRIX_SOURCE" $FIXTURES
