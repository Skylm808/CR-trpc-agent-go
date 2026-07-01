package main

import (
	"os"
	"path/filepath"
	"testing"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
)

func TestRunWritesReportFiles(t *testing.T) {
	dir := t.TempDir()
	diffPath := filepath.Join(dir, "sample.diff")
	if err := os.WriteFile(diffPath, []byte(""+
		"diff --git a/foo.go b/foo.go\n"+
		"index 1111111..2222222 100644\n"+
		"--- a/foo.go\n"+
		"+++ b/foo.go\n"+
		"@@ -1,1 +1,2 @@\n"+
		" package foo\n"+
		"+func Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		DiffFile:   diffPath,
		OutputDir:  dir,
		Mode:       cragent.ModeRuleOnly,
		Runtime:    cragent.RuntimeLocalFallback,
		SkillsRoot: filepath.Join("..", "..", "skills"),
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

func TestRunCanUseFileListForDiffGeneration(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package foo\n\nfunc Bad() { panic(\"boom\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	listPath := filepath.Join(repo, "files.txt")
	if err := os.WriteFile(listPath, []byte("foo.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	opts := Options{
		FileList:   listPath,
		RepoPath:   repo,
		OutputDir:  out,
		Mode:       cragent.ModeRuleOnly,
		Runtime:    cragent.RuntimeLocalFallback,
		SkillsRoot: filepath.Join("..", "..", "skills"),
	}
	if err := Run(opts); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "review_report.json")); err != nil {
		t.Fatalf("expected json report: %v", err)
	}
}
