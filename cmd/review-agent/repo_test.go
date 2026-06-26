package main

import (
	"os"
	"path/filepath"
	"testing"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
)

func TestRunCanUseRepoPathForDiffGeneration(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	opts := Options{
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
