#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURES_ROOT="${CR_AGENT_EVAL_FIXTURES_ROOT:-"$ROOT/testdata/fixtures"}"
SKILLS_ROOT="${CR_AGENT_EVAL_SKILLS_ROOT:-"$ROOT/skills"}"
RUNTIME="${CR_AGENT_EVAL_RUNTIME:-local-fallback}"
MODE="${CR_AGENT_EVAL_MODE:-rule-only}"
FIXTURES="${CR_AGENT_EVAL_FIXTURES:-safe.diff secret.diff panic.diff todo.diff test-missing.diff missing-test.diff goroutine.diff context.diff resource.diff db-lifecycle.diff dedupe.diff sandbox-fail.diff sandbox-timeout.diff}"
OUT_ROOT="$(mktemp -d)"
START_SECONDS="$(date +%s)"
trap 'rm -rf "$OUT_ROOT"' EXIT

EXPECTED_FILE="$OUT_ROOT/expected.tsv"
cat > "$EXPECTED_FILE" <<'TSV'
secret.diff	secret-leak	critical	finding
panic.diff	panic-direct	high	finding
todo.diff	todo-marker	medium	finding
test-missing.diff	missing-test-hint	low	warning
missing-test.diff	missing-test-hint	low	warning
goroutine.diff	goroutine-leak	high	finding
context.diff	context-leak	high	finding
resource.diff	resource-leak	high	finding
db-lifecycle.diff	db-lifecycle	high	finding
dedupe.diff	panic-direct	high	finding
TSV

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
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "usage: eval <expected.tsv> <out-root> <duration-ms> <fixtures...>")
		os.Exit(2)
	}
	expectedFile := os.Args[1]
	outRoot := os.Args[2]
	durationMS := os.Args[3]
	fixtures := os.Args[4:]

	expected, err := readExpected(expectedFile)
	if err != nil {
		panic(err)
	}
	fixtureSet := map[string]bool{}
	for _, fixture := range fixtures {
		fixtureSet[fixture] = true
	}

	truePositive := 0
	falsePositive := 0
	falseNegative := 0
	expectedTotal := 0
	for _, fixture := range fixtures {
		want := expected[fixture]
		expectedTotal += len(want)
		got, err := readActual(filepath.Join(outRoot, fixture, "review_report.json"))
		if err != nil {
			panic(err)
		}
		for key := range got {
			if want[key] {
				truePositive++
			} else {
				falsePositive++
			}
		}
		for key := range want {
			if !got[key] {
				falseNegative++
			}
		}
	}
	recall := ratio(truePositive, truePositive+falseNegative)
	precision := ratio(truePositive, truePositive+falsePositive)
	fmt.Printf("fixtures=%d expected=%d true_positive=%d false_positive=%d false_negative=%d recall=%.3f precision=%.3f duration_ms=%s\n",
		len(fixtureSet), expectedTotal, truePositive, falsePositive, falseNegative, recall, precision, durationMS)
	if falsePositive > 0 || falseNegative > 0 {
		os.Exit(1)
	}
}

func readExpected(path string) (map[string]map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := map[string]map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 4 {
			continue
		}
		fixture := fields[0]
		if out[fixture] == nil {
			out[fixture] = map[string]bool{}
		}
		out[fixture][strings.Join(fields[1:], "|")] = true
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
GO

DURATION_MS="$(( ($(date +%s) - START_SECONDS) * 1000 ))"
go run "$HELPER" "$EXPECTED_FILE" "$OUT_ROOT" "$DURATION_MS" $FIXTURES
