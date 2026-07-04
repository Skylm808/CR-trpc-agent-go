package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
	"gopkg.in/yaml.v3"
)

const defaultConfigFile = "cr-agent.yaml"

// fileConfig 对应 cr-agent.yaml。字段名保持贴近 CLI flag，便于用户从长命令迁移到配置文件。
type fileConfig struct {
	DiffFile     string          `yaml:"diff_file"`
	FileList     string          `yaml:"file_list"`
	RepoPath     string          `yaml:"repo_path"`
	Fixture      string          `yaml:"fixture"`
	BaseRef      string          `yaml:"base_ref"`
	HeadRef      string          `yaml:"head_ref"`
	OutputDir    string          `yaml:"output_dir"`
	Mode         string          `yaml:"mode"`
	SQLitePath   string          `yaml:"sqlite"`
	Runtime      string          `yaml:"runtime"`
	SkillsRoot   string          `yaml:"skills_root"`
	FixturesRoot string          `yaml:"fixtures_root"`
	Staticcheck  *bool           `yaml:"staticcheck"`
	Model        fileModelConfig `yaml:"model"`
}

type fileModelConfig struct {
	Provider  string `yaml:"provider"`
	Name      string `yaml:"name"`
	Endpoint  string `yaml:"endpoint"`
	APIKeyEnv string `yaml:"api_key_env"`
	BaseURL   string `yaml:"base_url"`
	Variant   string `yaml:"variant"`
}

func resolveOptions(cli Options) (Options, error) {
	configOpts, err := optionsFromConfig(cli.ConfigFile)
	if err != nil {
		return Options{}, err
	}
	opts := configOpts
	applyCLIOptions(&opts, cli)
	applyOptionDefaults(&opts)
	return opts, nil
}

func optionsFromConfig(path string) (Options, error) {
	explicit := strings.TrimSpace(path) != ""
	if !explicit {
		path = defaultConfigFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return Options{}, nil
		}
		return Options{}, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg fileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Options{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	opts := Options{
		ConfigFile:     path,
		DiffFile:       cfg.DiffFile,
		FileList:       cfg.FileList,
		RepoPath:       cfg.RepoPath,
		Fixture:        cfg.Fixture,
		BaseRef:        cfg.BaseRef,
		HeadRef:        cfg.HeadRef,
		OutputDir:      cfg.OutputDir,
		Mode:           cfg.Mode,
		SQLitePath:     cfg.SQLitePath,
		Runtime:        cfg.Runtime,
		SkillsRoot:     cfg.SkillsRoot,
		FixturesRoot:   cfg.FixturesRoot,
		ModelProvider:  cfg.Model.Provider,
		ModelEndpoint:  cfg.Model.Endpoint,
		ModelAPIKeyEnv: cfg.Model.APIKeyEnv,
		ModelName:      cfg.Model.Name,
		ModelBaseURL:   cfg.Model.BaseURL,
		ModelVariant:   cfg.Model.Variant,
	}
	if cfg.Staticcheck != nil {
		opts.Staticcheck = *cfg.Staticcheck
	}
	return opts, nil
}

func applyCLIOptions(opts *Options, cli Options) {
	applyStringOption(&opts.ConfigFile, cli.ConfigFile, cli, "config")
	applyStringOption(&opts.DiffFile, cli.DiffFile, cli, "diff-file")
	applyStringOption(&opts.FileList, cli.FileList, cli, "file-list")
	applyStringOption(&opts.RepoPath, cli.RepoPath, cli, "repo-path")
	applyStringOption(&opts.Fixture, cli.Fixture, cli, "fixture")
	applyStringOption(&opts.BaseRef, cli.BaseRef, cli, "base-ref")
	applyStringOption(&opts.HeadRef, cli.HeadRef, cli, "head-ref")
	applyStringOption(&opts.OutputDir, cli.OutputDir, cli, "output-dir")
	applyStringOption(&opts.Mode, cli.Mode, cli, "mode")
	applyStringOption(&opts.SQLitePath, cli.SQLitePath, cli, "sqlite")
	applyStringOption(&opts.Runtime, cli.Runtime, cli, "runtime")
	applyStringOption(&opts.SkillsRoot, cli.SkillsRoot, cli, "skills-root")
	applyStringOption(&opts.FixturesRoot, cli.FixturesRoot, cli, "fixtures-root")
	applyStringOption(&opts.ModelProvider, cli.ModelProvider, cli, "model-provider")
	applyStringOption(&opts.ModelEndpoint, cli.ModelEndpoint, cli, "model-endpoint")
	applyStringOption(&opts.ModelAPIKeyEnv, cli.ModelAPIKeyEnv, cli, "model-api-key-env")
	applyStringOptionAny(&opts.ModelName, cli.ModelName, cli, "model-name", "model")
	applyStringOption(&opts.ModelBaseURL, cli.ModelBaseURL, cli, "model-base-url")
	applyStringOption(&opts.ModelVariant, cli.ModelVariant, cli, "model-variant")
	if optionWasSet(cli, "staticcheck", cli.Staticcheck) {
		opts.Staticcheck = cli.Staticcheck
	}
	if optionWasSet(cli, "streaming", cli.Streaming) {
		opts.Streaming = cli.Streaming
	}
}

func applyStringOption(target *string, value string, cli Options, flagName string) {
	if optionWasSet(cli, flagName, strings.TrimSpace(value) != "") {
		*target = value
	}
}

func applyStringOptionAny(target *string, value string, cli Options, flagNames ...string) {
	if optionWasSetAny(cli, strings.TrimSpace(value) != "", flagNames...) {
		*target = value
	}
}

func optionWasSet(cli Options, flagName string, fallback bool) bool {
	if cli.ExplicitFlags != nil {
		return cli.ExplicitFlags[flagName]
	}
	return fallback
}

func optionWasSetAny(cli Options, fallback bool, flagNames ...string) bool {
	if cli.ExplicitFlags == nil {
		return fallback
	}
	for _, flagName := range flagNames {
		if cli.ExplicitFlags[flagName] {
			return true
		}
	}
	return false
}

func applyOptionDefaults(opts *Options) {
	if strings.TrimSpace(opts.OutputDir) == "" {
		opts.OutputDir = "."
	}
	if strings.TrimSpace(opts.Mode) == "" {
		opts.Mode = cragent.ModeRuleOnly
	}
	if strings.TrimSpace(opts.Runtime) == "" {
		opts.Runtime = cragent.RuntimeContainer
	}
	if strings.TrimSpace(opts.SkillsRoot) == "" {
		opts.SkillsRoot = "skills"
	}
	if strings.TrimSpace(opts.FixturesRoot) == "" {
		opts.FixturesRoot = "testdata/fixtures"
	}
}
