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
