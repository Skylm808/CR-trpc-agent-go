package report

import (
	"strings"
	"testing"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

func TestJSONAndMarkdownReportsIncludeFindings(t *testing.T) {
	rep := review.Result{
		TaskID: "task-1",
		Findings: []review.Finding{{
			Severity: "high",
			Category: "security",
			File:     "main.go",
			Line:     10,
			Title:    "Hardcoded secret",
			Source:   "rule",
			RuleID:   "secret-leak",
		}},
	}

	j, err := BuildJSON(rep)
	if err != nil {
		t.Fatalf("BuildJSON returned error: %v", err)
	}
	if !strings.Contains(string(j), "\"high\"") {
		t.Fatalf("expected JSON report to include finding severity, got %s", string(j))
	}

	md := BuildMarkdown(rep)
	if !strings.Contains(md, "Hardcoded secret") {
		t.Fatalf("expected Markdown report to include finding title, got %s", md)
	}
}

func TestReportsIncludeGovernanceSandboxArtifactsAndHumanReviewContract(t *testing.T) {
	rep := review.Result{
		TaskID: "task-contract",
		Findings: []review.Finding{{
			Severity:       "critical",
			Category:       "security",
			File:           "config.go",
			Line:           3,
			Title:          "Potential secret appears in added code",
			Recommendation: "Move the value to a secret manager.",
			Source:         "skill_run",
			RuleID:         "secret-leak",
			Status:         "finding",
		}},
		Warnings: []review.Finding{{
			Severity: "low",
			Category: "governance",
			Title:    "Command requires human review",
			Source:   "permission",
			RuleID:   "permission-ask",
			Status:   "needs_human_review",
		}},
		Metrics: review.Metrics{
			FindingCount:      1,
			SeverityCounts:    map[string]int{"critical": 1, "low": 1},
			ExceptionCounts:   map[string]int{"skill_run": 1},
			PermissionBlocks:  1,
			ToolCallCount:     2,
			SandboxDurationMS: 12,
			TotalDurationMS:   20,
		},
		GovernanceSummary: review.GovernanceSummary{
			PermissionDecisions: []review.PermissionDecisionSummary{{
				Command: "scripts/check.sh",
				Action:  "allow",
				Reason:  "policy allow",
			}},
		},
		SandboxSummary: review.SandboxSummary{
			Runs: []review.SandboxRunSummary{{
				Command:          "scripts/check.sh",
				Runtime:          "local-fallback",
				Status:           "ok",
				TimeoutMS:        5000,
				OutputLimitBytes: 65536,
				DurationMS:       12,
			}},
		},
		Artifacts: []review.ArtifactSummary{{
			Name: "review_report.json",
			Kind: "report",
			Path: "review_report.json",
		}},
	}

	j, err := BuildJSON(rep)
	if err != nil {
		t.Fatalf("BuildJSON returned error: %v", err)
	}
	jsonText := string(j)
	for _, want := range []string{
		"\"human_review_items\"",
		"\"governance_summary\"",
		"\"sandbox_summary\"",
		"\"artifacts\"",
		"\"severity_counts\"",
		"\"recommendation\"",
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("expected JSON report to include %s, got %s", want, jsonText)
		}
	}

	md := BuildMarkdown(rep)
	for _, want := range []string{
		"Human Review",
		"Governance",
		"Sandbox",
		"Artifacts",
		"Move the value to a secret manager.",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected Markdown report to include %q, got %s", want, md)
		}
	}
}
