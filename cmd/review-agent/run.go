package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
)

// Options 保存 CLI 参数。
type Options struct {
	DiffFile       string
	FileList       string
	RepoPath       string
	Fixture        string
	OutputDir      string
	Mode           string
	SQLitePath     string
	RunChecks      bool
	Runtime        string
	SkillsRoot     string
	FixturesRoot   string
	Staticcheck    bool
	ModelProvider  string
	ModelEndpoint  string
	ModelAPIKeyEnv string
	ModelName      string
}

// Run 将 CLI 参数交给 Agent。
func Run(opts Options) error {
	if opts.DiffFile == "" && opts.FileList == "" && opts.RepoPath == "" && opts.Fixture == "" {
		return errors.New("diff file, file list, repo path, or fixture is required")
	}
	cfg := cragent.Config{
		SkillsRoot:            opts.SkillsRoot,
		Runtime:               opts.Runtime,
		SQLitePath:            opts.SQLitePath,
		OutputDir:             opts.OutputDir,
		FixturesRoot:          opts.FixturesRoot,
		ContainerRepoHostPath: opts.RepoPath,
		EnableStaticcheck:     opts.Staticcheck,
	}
	switch opts.ModelProvider {
	case "":
	case "http":
		cfg.ModelHTTP = cragent.HTTPModelProviderConfig{
			Enabled:   true,
			Endpoint:  opts.ModelEndpoint,
			APIKeyEnv: opts.ModelAPIKeyEnv,
			Model:     opts.ModelName,
		}
	default:
		return fmt.Errorf("unsupported model provider %q", opts.ModelProvider)
	}
	if cfg.SkillsRoot == "" {
		// 默认使用仓库内置 Skill。
		cfg.SkillsRoot = filepath.Join("skills")
	}
	if cfg.FixturesRoot == "" {
		cfg.FixturesRoot = filepath.Join("testdata", "fixtures")
	}
	ag, err := cragent.New(cfg)
	if err != nil {
		return err
	}
	defer ag.Close()

	// RunChecks 仅保留兼容性。
	_ = opts.RunChecks
	_, err = ag.Run(context.Background(), cragent.Request{
		DiffFile: opts.DiffFile,
		FileList: opts.FileList,
		RepoPath: opts.RepoPath,
		Fixture:  opts.Fixture,
		Mode:     opts.Mode,
	})
	return err
}
