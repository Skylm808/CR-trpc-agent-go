package review

import "testing"

func TestParseUnifiedDiffExtractsFileAndHunk(t *testing.T) {
	diff := "" +
		"diff --git a/main.go b/main.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package main\n" +
		"+func main() {}\n"

	parsed, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff returned error: %v", err)
	}
	if len(parsed.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(parsed.Files))
	}
	if parsed.Files[0].Path != "main.go" {
		t.Fatalf("expected file main.go, got %q", parsed.Files[0].Path)
	}
	if len(parsed.Files[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(parsed.Files[0].Hunks))
	}
}

