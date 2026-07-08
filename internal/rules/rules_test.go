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

func TestRunFindsExpandedGoReviewRules(t *testing.T) {
	t.Parallel()

	result := Run(ParsedDiff{
		Files: []ParsedFile{
			{
				Path: "service.go",
				Hunks: []Hunk{
					{Lines: []Line{
						{Kind: "context", Text: "func Serve(ctx context.Context, name string, mu *sync.Mutex, db *sql.DB) error {"},
						{Kind: "add", NewLine: 10, Text: `resp, err := http.Get("https://example.com")`},
						{Kind: "add", NewLine: 11, Text: `query := "SELECT * FROM users WHERE name = '" + name + "'"`},
						{Kind: "add", NewLine: 12, Text: `cmd := exec.Command("sh", "-c", name)`},
						{Kind: "add", NewLine: 13, Text: `child := context.Background()`},
						{Kind: "add", NewLine: 14, Text: `mu.Lock()`},
						{Kind: "add", NewLine: 15, Text: `for _, item := range items {`},
						{Kind: "add", NewLine: 16, Text: `defer item.Close()`},
						{Kind: "add", NewLine: 17, Text: `return err`},
						{Kind: "add", NewLine: 18, Text: `out := ""`},
						{Kind: "add", NewLine: 19, Text: `out += item.Name`},
					}},
				},
			},
		},
	}, Options{})

	assertRule(t, result.Findings, "http-body-close", "high", "finding")
	assertRule(t, result.Findings, "sql-string-concat", "critical", "finding")
	assertRule(t, result.Findings, "command-injection", "critical", "finding")
	assertRule(t, result.Findings, "context-background-misuse", "medium", "finding")
	assertRule(t, result.Findings, "mutex-unlock-missing", "high", "finding")
	assertRule(t, result.Findings, "defer-in-loop", "medium", "finding")
	assertRule(t, result.Findings, "bare-return-err", "medium", "finding")
	assertRule(t, result.Warnings, "string-concat-loop", "low", "needs_human_review")
}

func TestRunDoesNotFlagGuardedExpandedGoPatterns(t *testing.T) {
	t.Parallel()

	result := Run(ParsedDiff{
		Files: []ParsedFile{
			{
				Path: "safe.go",
				Hunks: []Hunk{
					{Lines: []Line{
						{Kind: "context", Text: "func Safe(ctx context.Context, mu *sync.Mutex, db *sql.DB) error {"},
						{Kind: "add", NewLine: 10, Text: `resp, err := http.Get("https://example.com")`},
						{Kind: "add", NewLine: 11, Text: `if err != nil { return fmt.Errorf("fetch: %w", err) }`},
						{Kind: "add", NewLine: 12, Text: `defer resp.Body.Close()`},
						{Kind: "add", NewLine: 13, Text: `rows, err := db.QueryContext(ctx, "SELECT * FROM users WHERE name = ?", name)`},
						{Kind: "add", NewLine: 14, Text: `cmd := exec.CommandContext(ctx, "git", "status")`},
						{Kind: "add", NewLine: 15, Text: `mu.Lock()`},
						{Kind: "add", NewLine: 16, Text: `defer mu.Unlock()`},
						{Kind: "add", NewLine: 17, Text: `return fmt.Errorf("save: %w", err)`},
						{Kind: "add", NewLine: 18, Text: `total := 0`},
						{Kind: "add", NewLine: 19, Text: `for _, value := range values { total += value }`},
						{Kind: "add", NewLine: 20, Text: `buf.WriteString(item.Name)`},
					}},
				},
			},
		},
	}, Options{})

	for _, ruleID := range []string{
		"http-body-close",
		"sql-string-concat",
		"command-injection",
		"context-background-misuse",
		"mutex-unlock-missing",
		"defer-in-loop",
		"bare-return-err",
		"secret-leak",
	} {
		assertNoRule(t, result.Findings, ruleID)
	}
	assertNoRule(t, result.Warnings, "string-concat-loop")
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

func assertNoRule(t *testing.T, findings []Finding, ruleID string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			t.Fatalf("unexpected rule_id=%q in %+v", ruleID, findings)
		}
	}
}
