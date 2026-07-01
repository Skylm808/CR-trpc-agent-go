package agent

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
	"go.opentelemetry.io/otel/attribute"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"trpc.group/trpc-go/trpc-agent-go/artifact"
	"trpc.group/trpc-go/trpc-agent-go/artifact/inmemory"
	telemetrytrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// TestAgentRunUsesFrameworkSkillPermissionExecutorAndStore 固定最小审查链路。
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
		"\"conclusion\"",
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
	if runs[0].EnvWhitelist != sandboxEnvWhitelist {
		t.Fatalf("expected env whitelist %q, got %+v", sandboxEnvWhitelist, runs[0])
	}
	if runs[0].FinishedAt.IsZero() || runs[0].ArtifactCount != 3 {
		t.Fatalf("expected sandbox audit completion fields, got %+v", runs[0])
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
	for _, artifact := range artifacts {
		if artifact.Size == 0 {
			t.Fatalf("expected artifact size to be persisted, got %+v", artifacts)
		}
	}
}

// TestAgentRunDoesNotPersistRawSecretsInSQLite 固定明文密钥不落库。
func TestAgentRunDoesNotPersistRawSecretsInSQLite(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot:   filepath.Join(root, "skills"),
		FixturesRoot: filepath.Join(root, "testdata", "fixtures"),
		Runtime:      RuntimeLocalFallback,
		SQLitePath:   dbPath,
		OutputDir:    t.TempDir(),
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	if _, err := ag.Run(context.Background(), Request{
		Fixture: "secret.diff",
		Mode:    ModeRuleOnly,
	}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite directly: %v", err)
	}
	defer db.Close()

	leaks, err := scanSQLiteForRawSecrets(context.Background(), db, []string{
		"sk-1234567890abcdef",
	})
	if err != nil {
		t.Fatalf("scan sqlite: %v", err)
	}
	if len(leaks) > 0 {
		t.Fatalf("sqlite persisted raw secrets: %s", strings.Join(leaks, ", "))
	}
}

// TestAgentRunPersistsWarningsForReplay 固定 warning 可回放。
func TestAgentRunPersistsWarningsForReplay(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot:   filepath.Join(root, "skills"),
		FixturesRoot: filepath.Join(root, "testdata", "fixtures"),
		Runtime:      RuntimeLocalFallback,
		SQLitePath:   dbPath,
		OutputDir:    t.TempDir(),
		Timeout:      5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		Fixture: "test-missing.diff",
		Mode:    ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected fixture to produce warning")
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	items, err := store.FindingsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load review items: %v", err)
	}
	for _, item := range items {
		if item.RuleID == "missing-test-hint" && item.Status == "warning" {
			return
		}
	}
	t.Fatalf("expected warning to be persisted for replay, got %+v", items)
}

// TestAgentRunDoesNotExecuteNonAllowPermission 固定非 allow 不执行。
func TestAgentRunDoesNotExecuteNonAllowPermission(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		decision tool.PermissionDecision
	}{
		{name: "deny", decision: tool.DenyPermission("blocked by test policy")},
		{name: "ask", decision: tool.AskPermission("requires approval in test policy")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := repoRoot(t)
			dbPath := filepath.Join(t.TempDir(), "review.db")
			ag, err := New(Config{
				SkillsRoot:   filepath.Join(root, "skills"),
				FixturesRoot: filepath.Join(root, "testdata", "fixtures"),
				Runtime:      RuntimeLocalFallback,
				SQLitePath:   dbPath,
				OutputDir:    t.TempDir(),
				Timeout:      5 * time.Second,
			})
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			defer ag.Close()
			ag.policy = tool.PermissionPolicyFunc(func(ctx context.Context, req *tool.PermissionRequest) (tool.PermissionDecision, error) {
				_ = ctx
				_ = req
				return tc.decision, nil
			})

			result, err := ag.Run(context.Background(), Request{
				Fixture: "secret.diff",
				Mode:    ModeRuleOnly,
			})
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			for _, finding := range result.Findings {
				if finding.RuleID == "secret-leak" {
					t.Fatalf("skill_run appears to have executed after %s decision: %+v", tc.decision.Action, result.Findings)
				}
			}
			if len(result.HumanReviewItems) == 0 {
				t.Fatalf("expected non-allow decision to create a human review item")
			}
			if len(result.GovernanceSummary.PermissionDecisions) == 0 ||
				result.GovernanceSummary.PermissionDecisions[0].Action != string(tc.decision.Action) {
				t.Fatalf("expected governance summary action %q, got %+v", tc.decision.Action, result.GovernanceSummary)
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
			if len(decisions) != 1 || decisions[0].Action != string(tc.decision.Action) {
				t.Fatalf("expected persisted %s decision, got %+v", tc.decision.Action, decisions)
			}
			runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
			if err != nil {
				t.Fatalf("load sandbox runs: %v", err)
			}
			if len(runs) != 1 || runs[0].Status != string(tc.decision.Action) || runs[0].ExitCode != 0 {
				t.Fatalf("expected non-executed %s sandbox record, got %+v", tc.decision.Action, runs)
			}
		})
	}
}

// TestAgentRunCountsAllPermissionBlocks 固定所有非 allow 决策都会计数。
func TestAgentRunCountsAllPermissionBlocks(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/blocked\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package blocked\n"), 0o644); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}

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
	ag.policy = tool.PermissionPolicyFunc(func(ctx context.Context, req *tool.PermissionRequest) (tool.PermissionDecision, error) {
		_ = ctx
		if req.ToolName == "workspace_exec" {
			return tool.DenyPermission("go checks blocked by test policy"), nil
		}
		return tool.AllowPermission(), nil
	})

	result, err := ag.Run(context.Background(), Request{
		RepoPath: repo,
		Mode:     ModeSandbox,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Metrics.PermissionBlocks != 2 || result.GovernanceSummary.PermissionBlocks != 2 {
		t.Fatalf("expected two blocked Go check decisions, got metrics=%+v governance=%+v", result.Metrics, result.GovernanceSummary)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()
	metrics, err := store.MetricsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load metrics: %v", err)
	}
	if metrics.PermissionBlockCount != 2 {
		t.Fatalf("expected persisted permission block count 2, got %+v", metrics)
	}
	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	for _, run := range runs {
		if strings.HasPrefix(run.Command, "go ") && run.Status != "deny" {
			t.Fatalf("expected denied Go check run to skip executor, got %+v", run)
		}
	}
}

// TestAgentRunAcceptsFixtureInput 固定 fixture 输入路径。
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

// TestReadInputFromRepoReturnsRepoPath 固定仓库输入仍按 repo path 读取。
func TestReadInputFromRepoReturnsRepoPath(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	diff, ref, err := readInput(Config{}, Request{
		RepoPath: root,
	})
	if err != nil {
		t.Fatalf("readInput returned error: %v", err)
	}
	if ref != root {
		t.Fatalf("expected repo path ref %q, got %q", root, ref)
	}
	if diff == nil {
		t.Fatalf("expected repo diff bytes")
	}
}

// TestReadInputFromRepoReadsWorkingTreeDiff 固定仓库输入按工作区 diff 读取。
func TestReadInputFromRepoReadsWorkingTreeDiff(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	diff, ref, err := readInput(Config{}, Request{RepoPath: repo})
	if err != nil {
		t.Fatalf("readInput returned error: %v", err)
	}
	if ref != repo {
		t.Fatalf("expected repo path ref %q, got %q", repo, ref)
	}
	if len(diff) == 0 {
		t.Fatalf("expected repo diff content")
	}
}

// TestReadInputFromFileListBuildsDiff 固定文件路径列表输入。
func TestReadInputFromFileListBuildsDiff(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	src := filepath.Join(repo, "foo.go")
	if err := os.WriteFile(src, []byte("package demo\n\nfunc Bad() { panic(\"boom\") }\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	listPath := filepath.Join(repo, "files.txt")
	if err := os.WriteFile(listPath, []byte("# changed files\nfoo.go\n\n"), 0o644); err != nil {
		t.Fatalf("write file list: %v", err)
	}

	diff, ref, err := readInput(Config{}, Request{FileList: listPath, RepoPath: repo})
	if err != nil {
		t.Fatalf("readInput returned error: %v", err)
	}
	if ref != listPath {
		t.Fatalf("expected file list ref %q, got %q", listPath, ref)
	}
	for _, want := range []string{"diff --git a/foo.go b/foo.go", "+++ b/foo.go", "+func Bad() { panic(\"boom\") }"} {
		if !strings.Contains(string(diff), want) {
			t.Fatalf("expected generated diff to include %q, got %s", want, diff)
		}
	}
}

// TestReadInputFromFileListRejectsRepoEscape 固定路径列表不能跳出 repo。
func TestReadInputFromFileListRejectsRepoEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("make repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.go"), []byte("package secret\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	listPath := filepath.Join(repo, "files.txt")
	if err := os.WriteFile(listPath, []byte("../secret.go\n"), 0o644); err != nil {
		t.Fatalf("write file list: %v", err)
	}

	if _, _, err := readInput(Config{}, Request{FileList: listPath, RepoPath: repo}); err == nil {
		t.Fatalf("expected repo escape to be rejected")
	}
}

// TestAgentRunAcceptsFileListInput 固定路径列表进入完整审查链路。
func TestAgentRunAcceptsFileListInput(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package demo\n\nfunc Bad() { panic(\"boom\") }\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	listPath := filepath.Join(repo, "files.txt")
	if err := os.WriteFile(listPath, []byte("foo.go\n"), 0o644); err != nil {
		t.Fatalf("write file list: %v", err)
	}
	outDir := t.TempDir()
	ag, err := New(Config{
		SkillsRoot: filepath.Join(root, "skills"),
		Runtime:    RuntimeLocalFallback,
		OutputDir:  outDir,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		FileList: listPath,
		RepoPath: repo,
		Mode:     ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Findings) == 0 || result.Findings[0].RuleID != "panic-direct" {
		t.Fatalf("expected panic finding from file list input, got %+v", result.Findings)
	}
	if _, err := os.Stat(filepath.Join(outDir, "review_report.json")); err != nil {
		t.Fatalf("expected json report: %v", err)
	}
}

// TestRequestInputKindRecognizesFileList 固定路径列表 telemetry 类型。
func TestRequestInputKindRecognizesFileList(t *testing.T) {
	t.Parallel()

	if got := requestInputKind(Request{FileList: "files.txt"}); got != "file_list" {
		t.Fatalf("input kind = %q, want file_list", got)
	}
}

// TestReportArtifactsRemainStable 固定报告和诊断产物语义不变。
func TestReportArtifactsRemainStable(t *testing.T) {
	t.Parallel()

	arts := reportArtifacts()
	if len(arts) != 3 {
		t.Fatalf("expected 3 artifacts, got %+v", arts)
	}
	if arts[0].Name != "review_report.json" || arts[1].Name != "review_report.md" || arts[2].Name != "review_diagnostics.json" {
		t.Fatalf("unexpected artifacts: %+v", arts)
	}
}

// TestEnforceArtifactLimitsBlocksOversizedReports 固定产物大小边界。
func TestEnforceArtifactLimitsBlocksOversizedReports(t *testing.T) {
	t.Parallel()

	err := enforceArtifactLimits(Config{MaxArtifactBytes: 4}, []artifactPayload{{
		Name: "review_report.json",
		Data: []byte("12345"),
	}})
	if err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("expected artifact size limit error, got %v", err)
	}
}

// TestAgentRunRejectsOversizedArtifacts 固定超大产物不落盘。
func TestAgentRunRejectsOversizedArtifacts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot:       filepath.Join(root, "skills"),
		Runtime:          RuntimeLocalFallback,
		SQLitePath:       dbPath,
		OutputDir:        outDir,
		Timeout:          5 * time.Second,
		MaxArtifactBytes: 1,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	_, err = ag.Run(context.Background(), Request{
		DiffFile: filepath.Join(root, "testdata", "fixtures", "secret.diff"),
		Mode:     ModeRuleOnly,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("expected artifact size limit error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "review_report.json")); !os.IsNotExist(statErr) {
		t.Fatalf("oversized report should not be written, stat err=%v", statErr)
	}
	store, openErr := sqlite.Open(dbPath)
	if openErr != nil {
		t.Fatalf("open sqlite: %v", openErr)
	}
	defer store.Close()
	var tasks []sqlite.Task
	db, openDBErr := sql.Open("sqlite", dbPath)
	if openDBErr != nil {
		t.Fatalf("open sqlite directly: %v", openDBErr)
	}
	defer db.Close()
	rows, queryErr := db.QueryContext(context.Background(), `SELECT task_id FROM review_tasks`)
	if queryErr != nil {
		t.Fatalf("query tasks: %v", queryErr)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			t.Fatalf("scan task id: %v", scanErr)
		}
		task, loadErr := store.TaskByID(context.Background(), id)
		if loadErr != nil {
			t.Fatalf("load task: %v", loadErr)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate task rows: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != "failed" || tasks[0].FinishedAt.IsZero() {
		t.Fatalf("expected failed task after artifact error, got %+v", tasks)
	}
}

// TestConclusionStatuses 固定最终结论规则。
func TestConclusionStatuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result review.Result
		want   string
	}{
		{
			name: "blocking finding",
			result: review.Result{Findings: []review.Finding{{
				Severity: "high",
			}}},
			want: "fail",
		},
		{
			name: "human review",
			result: review.Result{HumanReviewItems: []review.Finding{{
				Severity: "low",
			}}},
			want: "needs_human_review",
		},
		{
			name: "sandbox exception",
			result: review.Result{Metrics: review.Metrics{
				ExceptionCounts: map[string]int{"sandbox_failed": 1},
			}},
			want: "needs_human_review",
		},
		{
			name:   "pass",
			result: review.Result{},
			want:   "pass",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := conclusion(tc.result)
			if got.Status != tc.want {
				t.Fatalf("conclusion status = %q, want %q", got.Status, tc.want)
			}
		})
	}
}

// TestArtifactServiceReportsCanBeSavedAsArtifacts 固定报告和诊断可进入官方 artifact service。
func TestArtifactServiceReportsCanBeSavedAsArtifacts(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	svc := inmemory.NewService()
	dbPath := filepath.Join(t.TempDir(), "review.db")
	outDir := t.TempDir()
	ag, err := New(Config{
		SkillsRoot:      filepath.Join(root, "skills"),
		Runtime:         RuntimeLocalFallback,
		SQLitePath:      dbPath,
		OutputDir:       outDir,
		Timeout:         5 * time.Second,
		ArtifactService: svc,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()

	result, err := ag.Run(context.Background(), Request{
		DiffFile: filepath.Join(root, "testdata", "fixtures", "secret.diff"),
		Mode:     ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	keys, err := svc.ListArtifactKeys(context.Background(), artifact.SessionInfo{
		AppName:   "cr-agent",
		UserID:    "local",
		SessionID: result.TaskID,
	})
	if err != nil {
		t.Fatalf("list artifact keys: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 artifacts to be saved in official artifact service, got %+v", keys)
	}
	if _, err := os.Stat(filepath.Join(outDir, "review_diagnostics.json")); err != nil {
		t.Fatalf("expected diagnostics artifact: %v", err)
	}
	diagnostics, err := os.ReadFile(filepath.Join(outDir, "review_diagnostics.json"))
	if err != nil {
		t.Fatalf("read diagnostics artifact: %v", err)
	}
	for _, want := range []string{`"conclusion"`, result.Conclusion.Status, result.Conclusion.Reason} {
		if !strings.Contains(string(diagnostics), want) {
			t.Fatalf("expected diagnostics artifact to include %q, got %s", want, diagnostics)
		}
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()
	recs, err := store.ArtifactsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load artifact records: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected persisted artifact records, got %+v", recs)
	}
	for _, rec := range recs {
		if rec.Size == 0 {
			t.Fatalf("expected artifact size, got %+v", recs)
		}
	}
}

// TestAgentRunRecordsTelemetryAttributes 固定官方 telemetry span 摘要。
func TestAgentRunRecordsTelemetryAttributes(t *testing.T) {
	recorder := useAgentTelemetrySpanRecorder(t)

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
		Mode:     ModeRuleOnly,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	span := findAgentReviewSpan(t, recorder)
	attrs := agentSpanAttributes(span.Attributes())
	if attrs["cr_agent.task_id"].AsString() != result.TaskID {
		t.Fatalf("task id attribute mismatch: got %q want %q", attrs["cr_agent.task_id"].AsString(), result.TaskID)
	}
	if attrs["cr_agent.mode"].AsString() != ModeRuleOnly {
		t.Fatalf("mode attribute mismatch: %+v", attrs["cr_agent.mode"])
	}
	if attrs["cr_agent.input_type"].AsString() != "diff_file" {
		t.Fatalf("input type attribute mismatch: %+v", attrs["cr_agent.input_type"])
	}
	if attrs["cr_agent.finding_count"].AsInt64() != int64(len(result.Findings)) {
		t.Fatalf("finding count attribute mismatch: %+v", attrs["cr_agent.finding_count"])
	}
	if attrs["cr_agent.artifact_count"].AsInt64() != 3 {
		t.Fatalf("expected 3 artifact telemetry records, got %+v", attrs["cr_agent.artifact_count"])
	}
	if attrs["cr_agent.tool_call_count"].AsInt64() != int64(result.Metrics.ToolCallCount) {
		t.Fatalf("tool call count attribute mismatch: %+v", attrs["cr_agent.tool_call_count"])
	}
	if attrs["cr_agent.conclusion_status"].AsString() != result.Conclusion.Status {
		t.Fatalf("conclusion status attribute mismatch: got %q want %q", attrs["cr_agent.conclusion_status"].AsString(), result.Conclusion.Status)
	}
	if attrs["cr_agent.conclusion_reason"].AsString() != result.Conclusion.Reason {
		t.Fatalf("conclusion reason attribute mismatch: got %q want %q", attrs["cr_agent.conclusion_reason"].AsString(), result.Conclusion.Reason)
	}
}

// TestAgentRunRecordsSandboxFailureWithoutCrashing 固定失败不崩溃。
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

// TestAgentRunRecordsSandboxTimeoutWithoutCrashing 固定超时可审计。
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

// TestAgentRunDryRunRecordsSkippedSandbox 固定 dry-run 审计记录。
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
	if runs[0].EnvWhitelist != sandboxEnvWhitelist {
		t.Fatalf("expected dry-run env whitelist %q, got %+v", sandboxEnvWhitelist, runs[0])
	}
	decisions, err := store.DecisionsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load decisions: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Action != "dry_run" {
		t.Fatalf("expected dry_run permission decision, got %+v", decisions)
	}
}

// TestAgentRunFakeModelUsesDeterministicSkill 固定 fake-model 规则链路。
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

// TestAgentRunSandboxModeExecutesGoChecks 固定 sandbox Go 检查。
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

// TestRunGoSandboxCommandPrefersWorkspaceExec 固定 workspaceexec 成功时不触发 codeexec 兜底。
func TestRunGoSandboxCommandPrefersWorkspaceExec(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/workspaceprimary\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package workspaceprimary\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo_test.go"), []byte("package workspaceprimary\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"bad\") } }\n"), 0o644); err != nil {
		t.Fatalf("write foo_test.go: %v", err)
	}

	ag, err := New(Config{
		SkillsRoot: filepath.Join(root, "skills"),
		Runtime:    RuntimeLocalFallback,
		OutputDir:  t.TempDir(),
		Timeout:    10 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	defer ag.Close()
	fallback := &recordingTool{
		name: "execute_code",
		call: func(ctx context.Context, jsonArgs []byte) (any, error) {
			_ = ctx
			t.Fatalf("codeexec fallback should not be called after workspaceexec success: %s", jsonArgs)
			return nil, nil
		},
	}
	ag.checkTool = fallback

	decision, run := ag.runGoSandboxCommand(context.Background(), "task-workspace-primary", repo, "go test ./...")
	if decision.Action != "allow" {
		t.Fatalf("expected allow decision, got %+v", decision)
	}
	if run.Status != "ok" {
		t.Fatalf("expected workspaceexec go test to succeed, got %+v", run)
	}
	if fallback.calls != 0 {
		t.Fatalf("codeexec fallback should not be called after workspaceexec success, calls=%d", fallback.calls)
	}
}

// TestRunGoSandboxCommandFallsBackToCodeExec 固定 workspaceexec 不可用时保留 codeexec 兜底。
func TestRunGoSandboxCommandFallsBackToCodeExec(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	repo := t.TempDir()
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
	ag.exec = nil
	fallback := &recordingTool{
		name: "execute_code",
		call: func(ctx context.Context, jsonArgs []byte) (any, error) {
			_ = ctx
			if !strings.Contains(string(jsonArgs), "go vet ./...") {
				t.Fatalf("expected fallback args to include go vet command, got %s", jsonArgs)
			}
			return map[string]any{"output": "fallback ok"}, nil
		},
	}
	ag.checkTool = fallback

	decision, run := ag.runGoSandboxCommand(context.Background(), "task-workspace-fallback", repo, "go vet ./...")
	if decision.Action != "allow" {
		t.Fatalf("expected allow decision, got %+v", decision)
	}
	if fallback.calls != 1 {
		t.Fatalf("expected exactly one codeexec fallback call, got %d", fallback.calls)
	}
	if run.Status != "ok" || run.StdoutDigest == "" {
		t.Fatalf("expected successful fallback sandbox run with digest, got %+v", run)
	}
}

// TestAgentRunSandboxModeRecordsGoCheckFailure 固定 Go 检查失败可审计。
func TestAgentRunSandboxModeRecordsGoCheckFailure(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/faildemo\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package faildemo\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo_test.go"), []byte("package faildemo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 4 { t.Fatal(\"bad\") } }\n"), 0o644); err != nil {
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
	if got := result.Metrics.ExceptionCounts["sandbox_failed"]; got == 0 {
		t.Fatalf("expected sandbox_failed metric, got %+v", result.Metrics.ExceptionCounts)
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
	for _, run := range runs {
		if run.Command == "go test ./..." {
			if run.Status != "failed" || run.ExitCode == 0 {
				t.Fatalf("expected failed go test run with exit code, got %+v", run)
			}
			return
		}
	}
	t.Fatalf("go test sandbox run not found: %+v", runs)
}

// TestAgentRunSandboxModeOptionallyExecutesStaticcheck 固定 staticcheck 显式开启。
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

// TestAgentRunContainerRuntimeExecutesGoChecks 验证真实容器链路。
func TestAgentRunContainerRuntimeExecutesGoChecks(t *testing.T) {
	if os.Getenv("CR_AGENT_RUN_CONTAINER_TESTS") != "1" {
		t.Skip("set CR_AGENT_RUN_CONTAINER_TESTS=1 to run Docker container integration test")
	}

	root := repoRoot(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/containerdemo\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo.go"), []byte("package containerdemo\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatalf("write foo.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "foo_test.go"), []byte("package containerdemo\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"bad\") } }\n"), 0o644); err != nil {
		t.Fatalf("write foo_test.go: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "review.db")
	ag, err := New(Config{
		SkillsRoot:            filepath.Join(root, "skills"),
		Runtime:               RuntimeContainer,
		SQLitePath:            dbPath,
		OutputDir:             t.TempDir(),
		Timeout:               60 * time.Second,
		ContainerRepoHostPath: repo,
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
	runs, err := store.SandboxRunsByTaskID(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("load sandbox runs: %v", err)
	}
	assertRunForCommand(t, runs, "go test ./...")
	assertRunForCommand(t, runs, "go vet ./...")
	for _, run := range runs {
		if strings.Contains(run.Command, "go ") && run.Runtime != RuntimeContainer {
			t.Fatalf("go check should run in container runtime, got %+v", run)
		}
	}
}

func TestSandboxRepoPathForRuntime(t *testing.T) {
	t.Parallel()

	hostRepo := filepath.Join(t.TempDir(), "repo")
	localPath := sandboxRepoPathForRuntime(RuntimeLocalFallback, hostRepo)
	if localPath != hostRepo {
		t.Fatalf("local fallback path = %q, want %q", localPath, hostRepo)
	}
	containerPath := sandboxRepoPathForRuntime(RuntimeContainer, hostRepo)
	if containerPath != containerRepoMountPath {
		t.Fatalf("container path = %q, want %q", containerPath, containerRepoMountPath)
	}
}

func TestGoSandboxCodeUsesRuntimeRepoPath(t *testing.T) {
	t.Parallel()

	hostRepo := filepath.Join(t.TempDir(), "repo")
	code := goSandboxCode(RuntimeContainer, hostRepo, "go test ./...")
	if !strings.Contains(code, "cd "+shellQuote(containerRepoMountPath)) {
		t.Fatalf("container command should cd into mount path, got %q", code)
	}
	if !strings.Contains(code, "GOCACHE="+shellQuote(goSandboxCacheDir)) {
		t.Fatalf("container command should set sandbox Go cache, got %q", code)
	}
	if strings.Contains(code, hostRepo) {
		t.Fatalf("container command leaked host repo path %q: %q", hostRepo, code)
	}
}

// repoRoot 查找仓库根目录。
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

// useAgentTelemetrySpanRecorder 捕获官方 telemetry trace。
func useAgentTelemetrySpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := tracesdk.NewTracerProvider(tracesdk.WithSpanProcessor(recorder))
	originalProvider := telemetrytrace.TracerProvider
	originalTracer := telemetrytrace.Tracer
	telemetrytrace.TracerProvider = provider
	telemetrytrace.Tracer = provider.Tracer("cr-agent-test")
	t.Cleanup(func() {
		telemetrytrace.TracerProvider = originalProvider
		telemetrytrace.Tracer = originalTracer
		_ = provider.Shutdown(context.Background())
	})
	return recorder
}

func findAgentReviewSpan(t *testing.T, recorder *tracetest.SpanRecorder) tracesdk.ReadOnlySpan {
	t.Helper()

	for _, span := range recorder.Ended() {
		if span.Name() == "cr-agent.review" {
			return span
		}
	}
	t.Fatalf("cr-agent.review span not found; got %d spans", len(recorder.Ended()))
	return nil
}

func agentSpanAttributes(attrs []attribute.KeyValue) map[string]attribute.Value {
	out := make(map[string]attribute.Value, len(attrs))
	for _, attr := range attrs {
		out[string(attr.Key)] = attr.Value
	}
	return out
}

type recordingTool struct {
	name  string
	calls int
	call  func(context.Context, []byte) (any, error)
}

func (t *recordingTool) Declaration() *tool.Declaration {
	return &tool.Declaration{Name: t.name}
}

func (t *recordingTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	t.calls++
	return t.call(ctx, jsonArgs)
}

// assertDecisionForCommand 检查 allow 决策。
func assertDecisionForCommand(t *testing.T, decisions []sqlite.DecisionRecord, command string) {
	t.Helper()
	for _, decision := range decisions {
		if decision.Command == command && decision.Action == "allow" {
			return
		}
	}
	t.Fatalf("expected allow decision for %q, got %+v", command, decisions)
}

// assertRunForCommand 检查成功沙箱记录。
func assertRunForCommand(t *testing.T, runs []sqlite.SandboxRunRecord, command string) {
	t.Helper()
	for _, run := range runs {
		if run.Command == command && run.Status == "ok" && run.DurationMS >= 0 {
			if run.EnvWhitelist != sandboxEnvWhitelist {
				t.Fatalf("expected sandbox env whitelist %q, got %+v", sandboxEnvWhitelist, run)
			}
			return
		}
	}
	t.Fatalf("expected ok sandbox run for %q, got %+v", command, runs)
}

// assertAnyRunForCommand 检查沙箱记录存在。
func assertAnyRunForCommand(t *testing.T, runs []sqlite.SandboxRunRecord, command string) {
	t.Helper()
	for _, run := range runs {
		if run.Command == command && run.Status != "" {
			return
		}
	}
	t.Fatalf("expected sandbox run for %q, got %+v", command, runs)
}

// scanSQLiteForRawSecrets 扫描明文密钥。
func scanSQLiteForRawSecrets(ctx context.Context, db *sql.DB, secrets []string) ([]string, error) {
	tables, err := sqliteTableNames(ctx, db)
	if err != nil {
		return nil, err
	}
	var leaks []string
	for _, table := range tables {
		columns, err := sqliteTextColumns(ctx, db, table)
		if err != nil {
			return nil, err
		}
		for _, column := range columns {
			values, err := sqliteColumnValues(ctx, db, table, column)
			if err != nil {
				return nil, err
			}
			for _, value := range values {
				for _, secret := range secrets {
					if strings.Contains(value, secret) {
						leaks = append(leaks, table+"."+column+" contains "+secret)
					}
				}
			}
		}
	}
	return leaks, nil
}

// sqliteTableNames 返回用户表名。
func sqliteTableNames(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT name FROM sqlite_schema
WHERE type='table' AND name NOT LIKE 'sqlite_%'
ORDER BY name
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

// sqliteTextColumns 返回文本列。
func sqliteTextColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		upperType := strings.ToUpper(columnType)
		if strings.Contains(upperType, "TEXT") || strings.Contains(upperType, "BLOB") {
			columns = append(columns, name)
		}
	}
	return columns, rows.Err()
}

// sqliteColumnValues 读取列值。
func sqliteColumnValues(ctx context.Context, db *sql.DB, table string, column string) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+column+" FROM "+table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value sql.NullString
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if value.Valid {
			values = append(values, value.String)
		}
	}
	return values, rows.Err()
}
