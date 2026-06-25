package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

func TestStorePersistsAndLoadsTaskData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Second)
	task := Task{
		ID:         "task-1",
		InputType:  "diff",
		InputRef:   "fixture.diff",
		InputDigest:"abc123",
		RepoPath:   "/repo",
		Status:     "done",
		Mode:       "rule-only",
		CreatedAt:  now,
		StartedAt:  now,
		FinishedAt: now,
	}
	if err := store.SaveTask(context.Background(), task); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}

	finding := review.Finding{
		Severity: "high", Category: "security", File: "main.go", Line: 9,
		Title: "Hardcoded secret", Source: "rule", RuleID: "secret-leak",
	}
	if err := store.SaveFinding(context.Background(), "task-1", finding); err != nil {
		t.Fatalf("SaveFinding returned error: %v", err)
	}
	if err := store.SaveReport(context.Background(), "task-1", []byte(`{"ok":true}`), []byte("# report")); err != nil {
		t.Fatalf("SaveReport returned error: %v", err)
	}

	got, err := store.TaskByID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("TaskByID returned error: %v", err)
	}
	if got.ID != task.ID || got.Status != task.Status {
		t.Fatalf("unexpected loaded task: %+v", got)
	}
	findings, err := store.FindingsByTaskID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("FindingsByTaskID returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	report, err := store.ReportByTaskID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ReportByTaskID returned error: %v", err)
	}
	if string(report.JSON) != `{"ok":true}` {
		t.Fatalf("unexpected report json: %s", string(report.JSON))
	}

	if err := store.SaveDecision(context.Background(), DecisionRecord{
		TaskID: "task-1",
		Command: "go test ./...",
		Action: "allow",
		Reason: "ok",
		At: now,
	}); err != nil {
		t.Fatalf("SaveDecision returned error: %v", err)
	}
	if err := store.SaveSandboxRun(context.Background(), SandboxRunRecord{
		TaskID: "task-1",
		Command: "go test ./...",
		Status: "ok",
		Output: "PASS",
		At: now,
	}); err != nil {
		t.Fatalf("SaveSandboxRun returned error: %v", err)
	}
}
