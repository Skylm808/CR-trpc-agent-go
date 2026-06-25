package review

import "testing"

func TestReviewContinuesWhenSandboxFails(t *testing.T) {
	result, err := AnalyzeDiff("" +
		"diff --git a/foo.go b/foo.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/foo.go\n" +
		"+++ b/foo.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		" package foo\n" +
		"+func Add(a, b int) int { return a + b }\n")
	if err != nil {
		t.Fatalf("AnalyzeDiff returned error: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning even without sandbox")
	}
}

