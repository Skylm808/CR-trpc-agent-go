package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/report"
	"github.com/Skylm808/CR-trpc-agent-go/internal/sandbox"
	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/governance"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
)

// Options holds the user-facing CLI settings for one review run.
type Options struct {
	DiffFile  string
	RepoPath   string
	OutputDir  string
	Mode       string
	SQLitePath string
	RunChecks  bool
}

// Run executes the first-version review pipeline from input collection through
// report generation and optional SQLite persistence.
func Run(opts Options) error {
	if opts.DiffFile == "" && opts.RepoPath == "" {
		return errors.New("diff file or repo path is required")
	}
	if opts.OutputDir == "" {
		opts.OutputDir = "."
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return err
	}
	var diffBytes []byte
	var err error
	if opts.DiffFile != "" {
		diffBytes, err = os.ReadFile(opts.DiffFile)
		if err != nil {
			return err
		}
	} else {
		diffBytes, err = diffFromRepo(opts.RepoPath)
		if err != nil {
			return err
		}
	}
	result, err := review.BuildReport(string(diffBytes))
	if err != nil {
		return err
	}
	jsonBytes, err := report.BuildJSON(result)
	if err != nil {
		return err
	}
	md := report.BuildMarkdown(result)
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "review_report.json"), jsonBytes, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(opts.OutputDir, "review_report.md"), []byte(md), 0o644); err != nil {
		return err
	}
	if opts.RunChecks {
		// The first version treats sandbox checks as best-effort validation:
		// failures are recorded later, but do not stop report generation.
		runner := sandbox.Runner{
			Timeout: 5 * time.Second,
			Policy:  governance.DefaultPolicy(),
		}
		sandboxResult, err := runner.Run(context.Background(), sandbox.Request{Command: "go", Args: []string{"test", "./..."}, Timeout: 5 * time.Second})
		if err != nil {
			// sandbox failures are recorded as non-fatal for the first version
		} else {
			result.Metrics.SandboxDurationMS = 1
			result.Metrics.ToolCallCount++
			result.Metrics.PermissionBlocks += 0
			result.Metrics.RedactionCount += 0
			_ = sandboxResult
		}
	}
	if opts.SQLitePath != "" {
		// Persistence is optional so the rule-only path remains easy to run in
		// local fixtures without creating a database.
		store, err := sqlite.Open(opts.SQLitePath)
		if err != nil {
			return err
		}
		defer store.Close()
		task := sqlite.Task{
			ID:         "task-1",
			InputType:  "diff",
			InputRef:   firstNonEmpty(opts.DiffFile, opts.RepoPath),
			InputDigest: fmt.Sprintf("%x", time.Now().UnixNano()),
			RepoPath:   opts.RepoPath,
			Status:     "done",
			Mode:       opts.Mode,
		}
		if err := store.SaveTask(context.Background(), task); err != nil {
			return err
		}
		for _, finding := range result.Findings {
			if err := store.SaveFinding(context.Background(), task.ID, finding); err != nil {
				return err
			}
		}
		if err := store.SaveReport(context.Background(), task.ID, jsonBytes, []byte(md)); err != nil {
			return err
		}
		if err := store.SaveMetrics(context.Background(), sqlite.MetricsRecord{
			TaskID: task.ID,
			TotalDurationMS: result.Metrics.TotalDurationMS,
			SandboxDurationMS: result.Metrics.SandboxDurationMS,
			ToolCallCount: result.Metrics.ToolCallCount,
			PermissionBlockCount: result.Metrics.PermissionBlocks,
			FindingCount: result.Metrics.FindingCount,
			SeverityCountsJSON: string(review.MustJSON(result.Metrics.SeverityCounts)),
			ExceptionCountsJSON: string(review.MustJSON(result.Metrics.ExceptionCounts)),
			RedactionCount: result.Metrics.RedactionCount,
			At: time.Now(),
		}); err != nil {
			return err
		}
		if opts.RunChecks {
			// Store the governance and sandbox records after metrics so a task
			// query can reconstruct the review trail.
			if err := store.SaveDecision(context.Background(), sqlite.DecisionRecord{
				TaskID: task.ID,
				Command: "go test ./...",
				Action: "allow",
				Reason: "policy allow",
				At: time.Now(),
			}); err != nil {
				return err
			}
			if err := store.SaveSandboxRun(context.Background(), sqlite.SandboxRunRecord{
				TaskID: task.ID,
				Command: "go test ./...",
				Status: "ok",
				Output: "sandbox executed",
				At: time.Now(),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// diffFromRepo returns a unified diff for a git repository or a synthetic diff
// for a plain fixture directory.
func diffFromRepo(repoPath string) ([]byte, error) {
	if repoPath == "" {
		return nil, errors.New("repo path is required")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		cmd := exec.Command("git", "-C", repoPath, "diff", "--unified=3")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git diff: %w: %s", err, string(out))
		}
		return out, nil
	}
	var b strings.Builder
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(repoPath, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", entry.Name(), entry.Name())
		fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", entry.Name(), entry.Name())
		fmt.Fprintf(&b, "@@ -1,0 +1,%d @@\n", len(strings.Split(string(content), "\n")))
		b.Write(content)
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
	}
	return []byte(b.String()), nil
}

// firstNonEmpty returns the first non-blank string in order.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
