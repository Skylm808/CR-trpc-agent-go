package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestRunInfersCurrentDirectoryRepoPathWhenInputIsOmitted(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/reviewme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package foo\n\nfunc Bad() { panic(\"boom\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillsRoot, err := filepath.Abs(filepath.Join("..", "..", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	out := t.TempDir()
	opts := Options{
		OutputDir:  out,
		Mode:       cragent.ModeRuleOnly,
		Runtime:    cragent.RuntimeLocalFallback,
		SkillsRoot: skillsRoot,
	}
	if err := Run(opts); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "review_report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(data), `"module_path": "example.com/reviewme"`) ||
		!strings.Contains(string(data), `"file": "foo.go"`) {
		t.Fatalf("expected report to use inferred current directory repo input, got %s", data)
	}
}
