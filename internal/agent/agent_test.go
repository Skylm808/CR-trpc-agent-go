package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
)

// TestAgentRunUsesFrameworkSkillPermissionExecutorAndStore 固定第一版最小链路：
// 读取 fixture diff，经由 trpc-agent-go 的 skill_load/skill_run 执行脚本，
// 再把权限决策、沙箱运行、finding 和报告写入 SQLite。
func TestAgentRunUsesFrameworkSkillPermissionExecutorAndStore(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dbPath := filepath.Join(t.TempDir(), "review.db")
	outDir := t.TempDir()

	ag, err := New(Config{
		SkillsRoot: filepath.Join(root, "skills"),
		Runtime:    RuntimeLocalFallback,
		SQLitePath: dbPath,
		OutputDir:  outDir,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := ag.Run(context.Background(), Request{
		DiffFile: filepath.Join(root, "testdata", "fixtures", "secret.diff"),
		Mode:     ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.TaskID == "" {
		t.Fatalf("TaskID is empty")
	}
	if len(result.Findings) == 0 {
		t.Fatalf("expected at least one finding from skill script")
	}
	if result.Metrics.ToolCallCount == 0 {
		t.Fatalf("expected framework tool calls to be counted")
	}

	jsonReport, err := os.ReadFile(filepath.Join(outDir, "review_report.json"))
	if err != nil {
		t.Fatalf("read json report: %v", err)
	}
	if strings.Contains(string(jsonReport), "sk-1234567890abcdef") {
		t.Fatalf("json report leaked raw secret: %s", jsonReport)
	}
	for _, want := range []string{
		"\"governance_summary\"",
		"\"sandbox_summary\"",
		"\"artifacts\"",
		"\"human_review_items\"",
	} {
		if !strings.Contains(string(jsonReport), want) {
			t.Fatalf("expected json report to include %s, got %s", want, jsonReport)
		}
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	if _, err := store.TaskByID(context.Background(), result.TaskID); err != nil {
		t.Fatalf("load task: %v", err)
	}
	findings, err := store.FindingsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load findings: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected persisted findings")
	}
	if strings.Contains(findings[0].Evidence, "sk-1234567890abcdef") {
		t.Fatalf("sqlite finding leaked raw secret: %+v", findings[0])
	}

	decisions, err := store.DecisionsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load permission decisions: %v", err)
	}
	if len(decisions) == 0 || decisions[0].Action != "allow" {
		t.Fatalf("expected allow permission decision, got %+v", decisions)
	}

	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	if len(runs) == 0 || runs[0].TimeoutMS == 0 || runs[0].OutputLimitBytes == 0 {
		t.Fatalf("expected bounded sandbox run record, got %+v", runs)
	}
	filterDecisions, err := store.FilterDecisionsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load filter decisions: %v", err)
	}
	if len(filterDecisions) == 0 || filterDecisions[0].Action != "redact" {
		t.Fatalf("expected redaction filter decision, got %+v", filterDecisions)
	}
	artifacts, err := store.ArtifactsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load artifacts: %v", err)
	}
	if len(artifacts) < 2 || artifacts[0].Name == "" {
		t.Fatalf("expected report artifacts, got %+v", artifacts)
	}
}

// TestAgentRunAcceptsFixtureInput 固定 fixture 输入契约，避免 CLI 自己解析
// fixture 后绕过 Agent 编排。
func TestAgentRunAcceptsFixtureInput(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outDir := t.TempDir()
	ag, err := New(Config{
		SkillsRoot:   filepath.Join(root, "skills"),
		FixturesRoot: filepath.Join(root, "testdata", "fixtures"),
		Runtime:      RuntimeLocalFallback,
		OutputDir:    outDir,
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		Fixture: "secret.diff",
		Mode:    ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Findings) == 0 || result.Findings[0].RuleID != "secret-leak" {
		t.Fatalf("expected fixture secret finding, got %+v", result.Findings)
	}
	if _, err := os.Stat(filepath.Join(outDir, "review_report.json")); err != nil {
		t.Fatalf("expected json report: %v", err)
	}
}

// TestAgentRunRecordsSandboxFailureWithoutCrashing 固定验收要求：
// 沙箱脚本失败不能让整个评审失败，必须生成报告并落库失败 run。
func TestAgentRunRecordsSandboxFailureWithoutCrashing(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dbPath := filepath.Join(t.TempDir(), "review.db")
	outDir := t.TempDir()
	ag, err := New(Config{
		SkillsRoot:   filepath.Join(root, "skills"),
		FixturesRoot: filepath.Join(root, "testdata", "fixtures"),
		Runtime:      RuntimeLocalFallback,
		SQLitePath:   dbPath,
		OutputDir:    outDir,
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		Fixture: "sandbox-fail.diff",
		Mode:    ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run should not fail when sandbox command fails: %v", err)
	}
	if got := result.Metrics.ExceptionCounts["sandbox_failed"]; got != 1 {
		t.Fatalf("expected sandbox_failed exception count, got %+v", result.Metrics.ExceptionCounts)
	}
	if _, err := os.Stat(filepath.Join(outDir, "review_report.json")); err != nil {
		t.Fatalf("expected json report: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" || runs[0].ExitCode == 0 {
		t.Fatalf("expected failed sandbox run with nonzero exit, got %+v", runs)
	}
	metrics, err := store.MetricsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load metrics: %v", err)
	}
	if !strings.Contains(metrics.ExceptionCountsJSON, "sandbox_failed") {
		t.Fatalf("expected persisted sandbox_failed metric, got %s", metrics.ExceptionCountsJSON)
	}
}

// TestAgentRunRecordsSandboxTimeoutWithoutCrashing 固定 timeout 验收要求：
// 超时必须记录为 timed_out，并写入 exception_counts，不能中断报告生成。
func TestAgentRunRecordsSandboxTimeoutWithoutCrashing(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot:   filepath.Join(root, "skills"),
		FixturesRoot: filepath.Join(root, "testdata", "fixtures"),
		Runtime:      RuntimeLocalFallback,
		SQLitePath:   dbPath,
		OutputDir:    t.TempDir(),
		Timeout:      1 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		Fixture: "sandbox-timeout.diff",
		Mode:    ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run should not fail when sandbox times out: %v", err)
	}
	if got := result.Metrics.ExceptionCounts["sandbox_failed"]; got != 1 {
		t.Fatalf("expected sandbox_failed exception count, got %+v", result.Metrics.ExceptionCounts)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()
	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "timed_out" {
		t.Fatalf("expected timed_out sandbox run, got %+v", runs)
	}
}

// TestAgentRunDryRunRecordsSkippedSandbox 固定 dry-run 语义：不进入 executor，
// 但仍然生成报告并记录权限/沙箱 skipped 审计数据。
func TestAgentRunDryRunRecordsSkippedSandbox(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot: filepath.Join(root, "skills"),
		Runtime:    RuntimeLocalFallback,
		SQLitePath: dbPath,
		OutputDir:  t.TempDir(),
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		DiffFile: filepath.Join(root, "testdata", "fixtures", "secret.diff"),
		Mode:     ModeDryRun,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Metrics.ToolCallCount != 1 {
		t.Fatalf("dry-run should only load skill, got tool calls %d", result.Metrics.ToolCallCount)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()
	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "skipped" {
		t.Fatalf("expected skipped sandbox run, got %+v", runs)
	}
	decisions, err := store.DecisionsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load decisions: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Action != "dry_run" {
		t.Fatalf("expected dry_run permission decision, got %+v", decisions)
	}
}

// TestAgentRunFakeModelUsesDeterministicSkill 固定 fake-model 语义：不需要真实模型
// API Key，但仍走 deterministic skill_run 审查链路。
func TestAgentRunFakeModelUsesDeterministicSkill(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	ag, err := New(Config{
		SkillsRoot: filepath.Join(root, "skills"),
		Runtime:    RuntimeLocalFallback,
		OutputDir:  t.TempDir(),
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		DiffFile: filepath.Join(root, "testdata", "fixtures", "secret.diff"),
		Mode:     ModeFakeModel,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Findings) == 0 || result.Findings[0].Source != "skill_run" {
		t.Fatalf("expected fake-model to use skill_run findings, got %+v", result.Findings)
	}
}

// TestAgentRunSandboxModeExecutesGoChecks 固定 sandbox 模式的最小 Go 项目检查：
// go test 与 go vet 必须先生成权限决策，再通过官方 codeexec 工具执行并落库。
func TestAgentRunSandboxModeExecutesGoChecks(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/demo\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package demo\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo_test.go"), []byte("package demo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"bad\") } }\n"), 0o644); err != nil {
		t.Fatalf("write foo_test.go: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot: filepath.Join(root, "skills"),
		Runtime:    RuntimeLocalFallback,
		SQLitePath: dbPath,
		OutputDir:  t.TempDir(),
		Timeout:    10 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		RepoPath: repo,
		Mode:     ModeSandbox,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()
	decisions, err := store.DecisionsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load decisions: %v", err)
	}
	assertDecisionForCommand(t, decisions, "go test ./...")
	assertDecisionForCommand(t, decisions, "go vet ./...")
	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	assertRunForCommand(t, runs, "go test ./...")
	assertRunForCommand(t, runs, "go vet ./...")
}

// TestAgentRunSandboxModeOptionallyExecutesStaticcheck 固定 staticcheck 为显式
// opt-in 检查：即使本机未安装 staticcheck，也必须先记录权限决策和沙箱 run。
func TestAgentRunSandboxModeOptionallyExecutesStaticcheck(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/staticdemo\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package staticdemo\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot:        filepath.Join(root, "skills"),
		Runtime:           RuntimeLocalFallback,
		SQLitePath:        dbPath,
		OutputDir:         t.TempDir(),
		Timeout:           10 * time.Second,
		EnableStaticcheck: true,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		RepoPath: repo,
		Mode:     ModeSandbox,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()
	decisions, err := store.DecisionsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load decisions: %v", err)
	}
	assertDecisionForCommand(t, decisions, "staticcheck ./...")
	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	assertAnyRunForCommand(t, runs, "staticcheck ./...")
}

// repoRoot 从当前测试目录向上查找 go.mod，避免测试依赖固定工作目录。
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = next
	}
}

// assertDecisionForCommand 检查指定命令存在 allow 权限决策。
func assertDecisionForCommand(t *testing.T, decisions []sqlite.DecisionRecord, command string) {
	t.Helper()
	for _, decision := range decisions {
		if decision.Command == command && decision.Action == "allow" {
			return
		}
	}
	t.Fatalf("expected allow decision for %q, got %+v", command, decisions)
}

// assertRunForCommand 检查指定命令存在成功沙箱记录。
func assertRunForCommand(t *testing.T, runs []sqlite.SandboxRunRecord, command string) {
	t.Helper()
	for _, run := range runs {
		if run.Command == command && run.Status == "ok" && run.DurationMS >= 0 {
			return
		}
	}
	t.Fatalf("expected ok sandbox run for %q, got %+v", command, runs)
}

// assertAnyRunForCommand 检查指定命令存在沙箱记录，不限制成功或失败。
func assertAnyRunForCommand(t *testing.T, runs []sqlite.SandboxRunRecord, command string) {
	t.Helper()
	for _, run := range runs {
		if run.Command == command && run.Status != "" {
			return
		}
	}
	t.Fatalf("expected sandbox run for %q, got %+v", command, runs)
}
