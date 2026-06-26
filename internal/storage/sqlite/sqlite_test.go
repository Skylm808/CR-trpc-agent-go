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
		ID:          "task-1",
		InputType:   "diff",
		InputRef:    "fixture.diff",
		InputDigest: "abc123",
		RepoPath:    "/repo",
		Status:      "done",
		Mode:        "rule-only",
		CreatedAt:   now,
		StartedAt:   now,
		FinishedAt:  now,
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
		TaskID:  "task-1",
		Command: "go test ./...",
		Action:  "allow",
		Reason:  "ok",
		At:      now,
	}); err != nil {
		t.Fatalf("SaveDecision returned error: %v", err)
	}
	if err := store.SaveSandboxRun(context.Background(), SandboxRunRecord{
		TaskID:  "task-1",
		Command: "go test ./...",
		Status:  "ok",
		Output:  "PASS",
		At:      now,
	}); err != nil {
		t.Fatalf("SaveSandboxRun returned error: %v", err)
	}
	if err := store.SaveMetrics(context.Background(), MetricsRecord{
		TaskID:               "task-1",
		TotalDurationMS:      10,
		SandboxDurationMS:    5,
		ToolCallCount:        1,
		PermissionBlockCount: 0,
		FindingCount:         1,
		SeverityCountsJSON:   `{"high":1}`,
		ExceptionCountsJSON:  `{}`,
		RedactionCount:       1,
		At:                   now,
	}); err != nil {
		t.Fatalf("SaveMetrics returned error: %v", err)
	}

	gotMetrics, err := store.MetricsByTaskID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("MetricsByTaskID returned error: %v", err)
	}
	if gotMetrics.FindingCount != 1 || gotMetrics.ToolCallCount != 1 {
		t.Fatalf("unexpected metrics: %+v", gotMetrics)
	}
}

func TestStorePersistsFilterDecisionsAndArtifacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Second)
	task := Task{
		ID:          "task-artifacts",
		InputType:   "diff",
		InputRef:    "fixture.diff",
		InputDigest: "abc123",
		Status:      "done",
		Mode:        "rule-only",
		CreatedAt:   now,
	}
	if err := store.SaveTask(context.Background(), task); err != nil {
		t.Fatalf("SaveTask returned error: %v", err)
	}
	if err := store.SaveFilterDecision(context.Background(), FilterDecisionRecord{
		TaskID: "task-artifacts",
		Target: "finding.evidence",
		Action: "redact",
		Reason: "secret pattern",
		At:     now,
	}); err != nil {
		t.Fatalf("SaveFilterDecision returned error: %v", err)
	}
	if err := store.SaveArtifact(context.Background(), ArtifactRecord{
		TaskID: "task-artifacts",
		Name:   "review_report.json",
		Kind:   "report",
		Path:   "review_report.json",
		Digest: "digest-1",
		At:     now,
	}); err != nil {
		t.Fatalf("SaveArtifact returned error: %v", err)
	}

	decisions, err := store.FilterDecisionsByTaskID(context.Background(), "task-artifacts")
	if err != nil {
		t.Fatalf("FilterDecisionsByTaskID returned error: %v", err)
	}
	if len(decisions) != 1 || decisions[0].Action != "redact" {
		t.Fatalf("unexpected filter decisions: %+v", decisions)
	}
	artifacts, err := store.ArtifactsByTaskID(context.Background(), "task-artifacts")
	if err != nil {
		t.Fatalf("ArtifactsByTaskID returned error: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "review_report.json" || artifacts[0].Digest != "digest-1" {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}
}
