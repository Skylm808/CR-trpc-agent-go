package main

import (
	"context"
	"errors"
	"path/filepath"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
)

// Options 保存一次 CLI 审查运行的用户输入参数。
type Options struct {
	DiffFile     string
	RepoPath     string
	Fixture      string
	OutputDir    string
	Mode         string
	SQLitePath   string
	RunChecks    bool
	Runtime      string
	SkillsRoot   string
	FixturesRoot string
	Staticcheck  bool
}

// Run 只做 CLI 参数到 Agent 请求的适配，实际审查链路必须进入 internal/agent。
func Run(opts Options) error {
	if opts.DiffFile == "" && opts.RepoPath == "" && opts.Fixture == "" {
		return errors.New("diff file, repo path, or fixture is required")
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
	if cfg.SkillsRoot == "" {
		// 默认使用仓库内交付的 code-review Skill；生产运行可通过 CLI 覆盖。
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

	// RunChecks 保留旧 CLI 字段兼容性；框架 MVP 中 Skill 脚本总是通过
	// skill_run 进入 executor，后续 go test/go vet 再接入独立开关。
	_ = opts.RunChecks
	_, err = ag.Run(context.Background(), cragent.Request{
		DiffFile: opts.DiffFile,
		RepoPath: opts.RepoPath,
		Fixture:  opts.Fixture,
		Mode:     opts.Mode,
	})
	return err
}
