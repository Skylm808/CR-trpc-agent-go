package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
)

// Options 保存 CLI 参数。
type Options struct {
	ConfigFile     string
	DiffFile       string
	FileList       string
	RepoPath       string
	Fixture        string
	BaseRef        string
	HeadRef        string
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
	ModelBaseURL   string
	ModelVariant   string
	Streaming      bool
	ExplicitFlags  map[string]bool
}

// Run 将 CLI 参数交给 Agent。
func Run(opts Options) error {
	var err error
	opts, err = resolveOptions(opts)
	if err != nil {
		return err
	}
	opts = withInferredInput(opts)
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
	case "openai", "openai-compatible", "deepseek":
		cfg.ModelOpenAI = cragent.OpenAIModelProviderConfig{
			Enabled:   true,
			Provider:  opts.ModelProvider,
			Model:     opts.ModelName,
			APIKeyEnv: opts.ModelAPIKeyEnv,
			BaseURL:   opts.ModelBaseURL,
			Variant:   opts.ModelVariant,
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
	// Streaming 兼容官方 examples/runner 的 -streaming 参数；当前报告仍一次性生成。
	_ = opts.Streaming
	_, err = ag.Run(context.Background(), cragent.Request{
		DiffFile: opts.DiffFile,
		FileList: opts.FileList,
		RepoPath: opts.RepoPath,
		Fixture:  opts.Fixture,
		BaseRef:  opts.BaseRef,
		HeadRef:  opts.HeadRef,
		Mode:     opts.Mode,
	})
	return err
}

func withInferredInput(opts Options) Options {
	if strings.TrimSpace(opts.DiffFile) == "" &&
		strings.TrimSpace(opts.FileList) == "" &&
		strings.TrimSpace(opts.RepoPath) == "" &&
		strings.TrimSpace(opts.Fixture) == "" {
		opts.RepoPath = "."
	}
	return opts
}
