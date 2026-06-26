package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
)

type reportFixture struct {
	Findings []findingExpectation
	Warnings []findingExpectation
	Secrets  []string
}

type findingExpectation struct {
	RuleID   string
	Severity string
	Status   string
}

func TestAllFixturesMatchExpectedReviewResults(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "fixtures")
	cases := map[string]reportFixture{
		"safe.diff": {},
		"secret.diff": {
			Findings: []findingExpectation{{RuleID: "secret-leak", Severity: "critical", Status: "finding"}},
			Secrets:  []string{"sk-1234567890abcdef"},
		},
		"panic.diff": {
			Findings: []findingExpectation{{RuleID: "panic-direct", Severity: "high", Status: "finding"}},
		},
		"todo.diff": {
			Findings: []findingExpectation{{RuleID: "todo-marker", Severity: "medium", Status: "finding"}},
		},
		"test-missing.diff": {
			Warnings: []findingExpectation{{RuleID: "missing-test-hint", Severity: "low", Status: "warning"}},
		},
		"missing-test.diff": {
			Warnings: []findingExpectation{{RuleID: "missing-test-hint", Severity: "low", Status: "warning"}},
		},
		"goroutine.diff": {
			Findings: []findingExpectation{{RuleID: "goroutine-leak", Severity: "high", Status: "finding"}},
		},
		"context.diff": {
			Findings: []findingExpectation{{RuleID: "context-leak", Severity: "high", Status: "finding"}},
		},
		"resource.diff": {
			Findings: []findingExpectation{{RuleID: "resource-leak", Severity: "high", Status: "finding"}},
		},
		"db-lifecycle.diff": {
			Findings: []findingExpectation{{RuleID: "db-lifecycle", Severity: "high", Status: "finding"}},
		},
		"dedupe.diff": {
			Findings: []findingExpectation{{RuleID: "panic-direct", Severity: "high", Status: "finding"}},
		},
		"sandbox-fail.diff": {},
	}

	for fileName, fixture := range cases {
		t.Run(fileName, func(t *testing.T) {
			result := runFixture(t, root, fileName)
			assertExpectedFindings(t, result.Findings, fixture.Findings)
			assertExpectedFindings(t, result.Warnings, fixture.Warnings)
			assertSecretsRedacted(t, result, fixture.Secrets)
			if fileName == "safe.diff" && len(result.Findings) != 0 {
				t.Fatalf("safe fixture should not produce findings, got %d", len(result.Findings))
			}
			if fileName == "dedupe.diff" && len(result.Findings) != 1 {
				t.Fatalf("dedupe fixture should keep exactly one finding, got %d", len(result.Findings))
			}
		})
	}
}

func TestRunCanUseFixtureName(t *testing.T) {
	out := t.TempDir()
	if err := Run(Options{
		Fixture:      "secret.diff",
		FixturesRoot: filepath.Join("..", "..", "testdata", "fixtures"),
		OutputDir:    out,
		Mode:         cragent.ModeRuleOnly,
		Runtime:      cragent.RuntimeLocalFallback,
		SkillsRoot:   filepath.Join("..", "..", "skills"),
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	jsonPath := filepath.Join(out, "review_report.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read report json: %v", err)
	}
	if !strings.Contains(string(data), "secret-leak") {
		t.Fatalf("expected fixture report to include secret finding, got %s", data)
	}
}

func runFixture(t *testing.T, root, name string) reportData {
	t.Helper()
	out := t.TempDir()
	if err := Run(Options{
		DiffFile:   filepath.Join(root, name),
		OutputDir:  out,
		Mode:       cragent.ModeRuleOnly,
		Runtime:    cragent.RuntimeLocalFallback,
		SkillsRoot: filepath.Join("..", "..", "skills"),
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	jsonPath := filepath.Join(out, "review_report.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read report json: %v", err)
	}
	var result reportData
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal report json: %v", err)
	}
	return result
}

type reportData struct {
	Findings []findingData `json:"findings"`
	Warnings []findingData `json:"warnings"`
}

type findingData struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

func assertExpectedFindings(t *testing.T, got []findingData, want []findingExpectation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d results, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i].RuleID != want[i].RuleID {
			t.Fatalf("result[%d] rule_id = %q, want %q", i, got[i].RuleID, want[i].RuleID)
		}
		if got[i].Severity != want[i].Severity {
			t.Fatalf("result[%d] severity = %q, want %q", i, got[i].Severity, want[i].Severity)
		}
		if got[i].Status != want[i].Status {
			t.Fatalf("result[%d] status = %q, want %q", i, got[i].Status, want[i].Status)
		}
	}
}

func assertSecretsRedacted(t *testing.T, result reportData, secrets []string) {
	t.Helper()
	if len(secrets) == 0 {
		return
	}
	for _, item := range append(result.Findings, result.Warnings...) {
		for _, secret := range secrets {
			if strings.Contains(item.Evidence, secret) {
				t.Fatalf("evidence still contains secret %q: %q", secret, item.Evidence)
			}
		}
	}
}
