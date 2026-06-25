package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesReportFiles(t *testing.T) {
	dir := t.TempDir()
	diffPath := filepath.Join(dir, "sample.diff")
	if err := os.WriteFile(diffPath, []byte("" +
		"diff --git a/foo.go b/foo.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/foo.go\n" +
		"+++ b/foo.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		" package foo\n" +
		"+func Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		DiffFile: diffPath,
		OutputDir: dir,
		Mode: "rule-only",
	}
	if err := Run(opts); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "review_report.json")); err != nil {
		t.Fatalf("expected json report: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "review_report.md")); err != nil {
		t.Fatalf("expected md report: %v", err)
	}
}

