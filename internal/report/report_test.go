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

