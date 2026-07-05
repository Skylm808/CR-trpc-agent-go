package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
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

func TestRunUsesCopiedGitHubAgentRepoAsGitFixture(t *testing.T) {
	source := os.Getenv("CR_AGENT_GIT_REPO_FIXTURE")
	if source == "" {
		source = "/Users/skylm/Desktop/GOLAND/trpc-agent/trpc-GitHub-agent"
	}
	if _, err := os.Stat(filepath.Join(source, ".git")); err != nil {
		t.Skipf("git repo fixture is unavailable: %s", source)
	}

	repo := filepath.Join(t.TempDir(), "trpc-GitHub-agent")
	cloneGitRepo(t, source, repo)
	// 真实仓库 fixture 只读，测试只改临时 clone，避免污染用户工作区。
	mainPath := filepath.Join(repo, "main.go")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read cloned main.go: %v", err)
	}
	replaced := strings.Replace(string(mainBytes), "package main", "package main // cr-agent git fixture\n\nfunc crAgentGoalFixturePanic() { panic(\"goal fixture\") }", 1)
	if replaced == string(mainBytes) {
		t.Fatalf("cloned main.go does not contain package declaration")
	}
	if err := os.WriteFile(mainPath, []byte(replaced), 0o644); err != nil {
		t.Fatalf("write cloned main.go: %v", err)
	}

	out := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "review.db")
	if err := Run(Options{
		RepoPath:   repo,
		OutputDir:  out,
		SQLitePath: dbPath,
		Mode:       cragent.ModeRuleOnly,
		Runtime:    cragent.RuntimeLocalFallback,
		SkillsRoot: filepath.Join("..", "..", "skills"),
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	reportBytes, err := os.ReadFile(filepath.Join(out, "review_report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report reportData
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if !reportHasRuleID(report, "panic-direct") {
		t.Fatalf("expected panic-direct finding from copied git repo, got %+v", report.Findings)
	}
	if !strings.Contains(string(report.InputMetadata), `"module_path": "trpc-GitHub-agent"`) ||
		!strings.Contains(string(report.InputMetadata), `"main.go"`) ||
		!strings.Contains(string(report.InputMetadata), `"main"`) {
		t.Fatalf("expected git repo metadata in report, got %s", report.InputMetadata)
	}

	diagnosticsBytes, err := os.ReadFile(filepath.Join(out, "review_diagnostics.json"))
	if err != nil {
		t.Fatalf("read diagnostics: %v", err)
	}
	for _, want := range []string{`"input_metadata"`, `"module_path": "trpc-GitHub-agent"`, `"changed_go_files"`, `"main.go"`} {
		if !strings.Contains(string(diagnosticsBytes), want) {
			t.Fatalf("diagnostics missing %q: %s", want, diagnosticsBytes)
		}
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	task, err := store.TaskByID(ctx, report.TaskID)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if task.Status != "done" || task.RepoPath != repo {
		t.Fatalf("unexpected task record: %+v", task)
	}
	if findings, err := store.FindingsByTaskID(ctx, report.TaskID); err != nil || len(findings) == 0 {
		t.Fatalf("expected sqlite findings, findings=%+v err=%v", findings, err)
	}
	if decisions, err := store.DecisionsByTaskID(ctx, report.TaskID); err != nil || len(decisions) == 0 {
		t.Fatalf("expected sqlite permission decisions, decisions=%+v err=%v", decisions, err)
	}
	if runs, err := store.SandboxRunsByTaskID(ctx, report.TaskID); err != nil || len(runs) == 0 {
		t.Fatalf("expected sqlite sandbox runs, runs=%+v err=%v", runs, err)
	}
	if metrics, err := store.MetricsByTaskID(ctx, report.TaskID); err != nil || metrics.FindingCount == 0 {
		t.Fatalf("expected sqlite metrics, metrics=%+v err=%v", metrics, err)
	}
	if artifacts, err := store.ArtifactsByTaskID(ctx, report.TaskID); err != nil || len(artifacts) < 3 {
		t.Fatalf("expected sqlite artifacts, artifacts=%+v err=%v", artifacts, err)
	}
}

func cloneGitRepo(t *testing.T, source string, dest string) {
	t.Helper()
	cmd := exec.Command("git", "clone", "--quiet", "--no-hardlinks", source, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone fixture repo: %v\n%s", err, out)
	}
}

func reportHasRuleID(report reportData, ruleID string) bool {
	for _, finding := range append(report.Findings, report.Warnings...) {
		if finding.RuleID == ruleID {
			return true
		}
	}
	return false
}
