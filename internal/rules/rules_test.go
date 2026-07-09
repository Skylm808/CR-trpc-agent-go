package rules

import (
	"strings"
	"testing"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

func TestRunFindsDeterministicRules(t *testing.T) {
	t.Parallel()

	result := Run(review.ParsedDiff{
		Files: []review.ParsedFile{
			{
				Path: "worker.go",
				Hunks: []review.Hunk{
					{
						Lines: []review.Line{
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

	result := Run(review.ParsedDiff{
		Files: []review.ParsedFile{
			{
				Path: "service.go",
				Hunks: []review.Hunk{
					{Lines: []review.Line{
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

	result := Run(review.ParsedDiff{
		Files: []review.ParsedFile{
			{
				Path: "safe.go",
				Hunks: []review.Hunk{
					{Lines: []review.Line{
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

func TestRunParsedUnifiedDiffFindsRulesAndWarnings(t *testing.T) {
	t.Parallel()

	result := runUnifiedDiff(t, ""+
		"diff --git a/worker.go b/worker.go\n"+
		"index 1111111..2222222 100644\n"+
		"--- a/worker.go\n"+
		"+++ b/worker.go\n"+
		"@@ -1,2 +1,8 @@\n"+
		" package worker\n"+
		"+func Start() {\n"+
		"+\tgo func() {}\n"+
		"+}\n"+
		"+const apiKey = \"sk-1234567890abcdef\"\n")

	assertRule(t, result.Findings, "goroutine-leak", "high", "finding")
	assertRule(t, result.Findings, "secret-leak", "critical", "finding")
	assertRule(t, result.Warnings, "missing-test-hint", "low", "warning")
	for _, finding := range result.Findings {
		if finding.RuleID == "secret-leak" && strings.Contains(finding.Evidence, "sk-1234567890abcdef") {
			t.Fatalf("secret evidence was not redacted: %+v", finding)
		}
	}
}

func TestRunParsedUnifiedDiffFindsSecretShapesAndSuppressesPlaceholders(t *testing.T) {
	t.Parallel()

	result := runUnifiedDiff(t, ""+
		"diff --git a/config.go b/config.go\n"+
		"index 1111111..2222222 100644\n"+
		"--- a/config.go\n"+
		"+++ b/config.go\n"+
		"@@ -1,2 +1,9 @@\n"+
		" package foo\n"+
		"+const llmkey = \"llm-live-1234567890abcdef\"\n"+
		"+const openaiKey = \"sk-proj-1234567890abcdef\"\n"+
		"+const client_secret = \"github_pat_1234567890abcdef1234567890abcdef\"\n"+
		"+const tokenPlaceholder = \"dummy\"\n"+
		"+const retryTokenTimeoutSeconds = 30\n")

	if got := countRule(result.Findings, "secret-leak"); got != 3 {
		t.Fatalf("expected three high-confidence secret findings, got %d: %+v", got, result.Findings)
	}
	for _, finding := range result.Findings {
		if finding.RuleID == "secret-leak" && containsRawSecretEvidence(finding.Evidence) {
			t.Fatalf("secret evidence was not redacted: %+v", finding)
		}
	}
}

func TestRunDedupesRepeatedFindingsThroughReviewDedupe(t *testing.T) {
	t.Parallel()

	result := runUnifiedDiff(t, ""+
		"diff --git a/dedupe.go b/dedupe.go\n"+
		"index 1111111..2222222 100644\n"+
		"--- a/dedupe.go\n"+
		"+++ b/dedupe.go\n"+
		"@@ -1,2 +1,5 @@\n"+
		" package foo\n"+
		"+func Crash() { panic(\"boom\") }\n"+
		"+func CrashAgain() { panic(\"boom\") }\n")

	if got := countRule(review.DedupeFindings(result.Findings), "panic-direct"); got != 1 {
		t.Fatalf("expected exactly one panic-direct finding after dedupe, got %d", got)
	}
}

func runUnifiedDiff(t *testing.T, diff string) Analysis {
	t.Helper()
	parsed, err := review.ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff returned error: %v", err)
	}
	return Run(parsed, Options{Redact: review.RedactSecrets})
}

func assertRule(t *testing.T, findings []review.Finding, ruleID, severity, status string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity && finding.Status == status {
			return
		}
	}
	t.Fatalf("missing rule_id=%q severity=%q status=%q in %+v", ruleID, severity, status, findings)
}

func assertNoRule(t *testing.T, findings []review.Finding, ruleID string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			t.Fatalf("unexpected rule_id=%q in %+v", ruleID, findings)
		}
	}
}

func countRule(findings []review.Finding, ruleID string) int {
	total := 0
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			total++
		}
	}
	return total
}

func containsRawSecretEvidence(text string) bool {
	for _, raw := range []string{
		"llm-live-1234567890abcdef",
		"sk-proj-1234567890abcdef",
		"github_pat_1234567890abcdef1234567890abcdef",
		"dummy",
	} {
		if strings.Contains(text, raw) {
			return true
		}
	}
	return false
}
