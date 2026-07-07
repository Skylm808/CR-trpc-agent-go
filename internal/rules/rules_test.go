package rules

import "testing"

func TestRunFindsDeterministicRules(t *testing.T) {
	t.Parallel()

	result := Run(ParsedDiff{
		Files: []ParsedFile{
			{
				Path: "worker.go",
				Hunks: []Hunk{
					{
						Lines: []Line{
							{Kind: "context", Text: "package worker"},
							{Kind: "add", NewLine: 3, Text: "func Start() {"},
							{Kind: "add", NewLine: 4, Text: "\tgo func() {}"},
							{Kind: "add", NewLine: 5, Text: "}"},
							{Kind: "add", NewLine: 6, Text: "const apiKey = \"sk-1234567890abcdef\""},
						},
					},
				},
			},
		},
	}, Options{Redact: func(s string) string {
		if s == "const apiKey = \"sk-1234567890abcdef\"" {
			return "const apiKey = [REDACTED]"
		}
		return s
	}})

	assertRule(t, result.Findings, "goroutine-leak", "high", "finding")
	assertRule(t, result.Findings, "secret-leak", "critical", "finding")
	assertRule(t, result.Warnings, "missing-test-hint", "low", "warning")
}

func assertRule(t *testing.T, findings []Finding, ruleID, severity, status string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity && finding.Status == status {
			return
		}
	}
	t.Fatalf("missing rule_id=%q severity=%q status=%q in %+v", ruleID, severity, status, findings)
}
