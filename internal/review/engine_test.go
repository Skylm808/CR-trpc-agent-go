package review

import "testing"

func TestAnalyzeDiffWarnsOnMissingTests(t *testing.T) {
	diff := "" +
		"diff --git a/foo.go b/foo.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/foo.go\n" +
		"+++ b/foo.go\n" +
		"@@ -1,2 +1,8 @@\n" +
		" package foo\n" +
		"+func Add(a, b int) int { return a + b }\n"

	result, err := AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected at least one warning")
	}
}

func TestAnalyzeDiffFindsSecrets(t *testing.T) {
	diff := "" +
		"diff --git a/config.go b/config.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/config.go\n" +
		"+++ b/config.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package foo\n" +
		"+const apiKey = \"sk-1234567890abcdef\"\n"

	result, err := AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected at least one finding")
	}
}

func TestAnalyzeDiffFindsGoroutineLeak(t *testing.T) {
	diff := "" +
		"diff --git a/worker.go b/worker.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/worker.go\n" +
		"+++ b/worker.go\n" +
		"@@ -1,2 +1,6 @@\n" +
		" package foo\n" +
		"+func Start() {\n" +
		"+\tgo func() {}\n" +
		"+}\n"

	result, err := AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	assertHasRule(t, result.Findings, "goroutine-leak", "high", "finding")
}

func TestAnalyzeDiffFindsContextLeak(t *testing.T) {
	diff := "" +
		"diff --git a/context.go b/context.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/context.go\n" +
		"+++ b/context.go\n" +
		"@@ -1,2 +1,6 @@\n" +
		" package foo\n" +
		"+func Handle() {\n" +
		"+\t_, cancel := context.WithTimeout(context.Background(), 0)\n" +
		"+\t_ = cancel\n" +
		"+}\n"

	result, err := AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	assertHasRule(t, result.Findings, "context-leak", "high", "finding")
}

func TestAnalyzeDiffFindsResourceLeak(t *testing.T) {
	diff := "" +
		"diff --git a/resource.go b/resource.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/resource.go\n" +
		"+++ b/resource.go\n" +
		"@@ -1,2 +1,6 @@\n" +
		" package foo\n" +
		"+func Open() *os.File {\n" +
		"+\tf, _ := os.Open(\"x\")\n" +
		"+\treturn f\n" +
		"+}\n"

	result, err := AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	assertHasRule(t, result.Findings, "resource-leak", "high", "finding")
}

func TestAnalyzeDiffFindsDBLifecycleLeak(t *testing.T) {
	diff := "" +
		"diff --git a/db.go b/db.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/db.go\n" +
		"+++ b/db.go\n" +
		"@@ -1,2 +1,6 @@\n" +
		" package foo\n" +
		"+func Query() error {\n" +
		"+\tdb, _ := sql.Open(\"sqlite\", \":memory:\")\n" +
		"+\t_ = db\n" +
		"+\treturn nil\n" +
		"+}\n"

	result, err := AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	assertHasRule(t, result.Findings, "db-lifecycle", "high", "finding")
}

func TestAnalyzeDiffDedupesRepeatedFindings(t *testing.T) {
	diff := "" +
		"diff --git a/dedupe.go b/dedupe.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/dedupe.go\n" +
		"+++ b/dedupe.go\n" +
		"@@ -1,2 +1,5 @@\n" +
		" package foo\n" +
		"+func Crash() { panic(\"boom\") }\n" +
		"+func CrashAgain() { panic(\"boom\") }\n"

	result, err := AnalyzeDiff(diff)
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	if got := countRule(result.Findings, "panic-direct"); got != 1 {
		t.Fatalf("expected exactly one panic-direct finding, got %d", got)
	}
}

func assertHasRule(t *testing.T, findings []Finding, ruleID, severity, status string) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID && finding.Severity == severity && finding.Status == status {
			return
		}
	}
	t.Fatalf("expected finding with rule_id=%q severity=%q status=%q", ruleID, severity, status)
}

func countRule(findings []Finding, ruleID string) int {
	total := 0
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			total++
		}
	}
	return total
}
