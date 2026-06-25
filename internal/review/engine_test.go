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
