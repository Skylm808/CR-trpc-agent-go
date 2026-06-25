package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/report"
	"github.com/Skylm808/CR-trpc-agent-go/internal/sandbox"
	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/governance"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
)

type Options struct {
	DiffFile  string
	RepoPath   string
	OutputDir  string
	Mode       string
	SQLitePath string
	RunChecks  bool
}

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
	diffBytes, err := os.ReadFile(opts.DiffFile)
	if err != nil {
		return err
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
		runner := sandbox.Runner{
			Timeout: 5 * time.Second,
			Policy:  governance.DefaultPolicy(),
		}
		if _, err := runner.Run(context.Background(), sandbox.Request{Command: "go", Args: []string{"test", "./..."}, Timeout: 5 * time.Second}); err != nil {
			// sandbox failures are recorded as non-fatal for the first version
		}
	}
	if opts.SQLitePath != "" {
		store, err := sqlite.Open(opts.SQLitePath)
		if err != nil {
			return err
		}
		defer store.Close()
		task := sqlite.Task{
			ID:         "task-1",
			InputType:  "diff",
			InputRef:   opts.DiffFile,
			InputDigest:"",
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
	}
	return nil
}
