package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var opts Options
	flag.StringVar(&opts.DiffFile, "diff-file", "", "path to unified diff")
	flag.StringVar(&opts.FileList, "file-list", "", "path to newline-delimited changed file list")
	flag.StringVar(&opts.RepoPath, "repo-path", "", "path to repository")
	flag.StringVar(&opts.Fixture, "fixture", "", "fixture diff name under fixtures root")
	flag.StringVar(&opts.BaseRef, "base-ref", "", "base git ref for review metadata")
	flag.StringVar(&opts.HeadRef, "head-ref", "", "head git ref for review metadata")
	flag.StringVar(&opts.OutputDir, "output-dir", ".", "directory for reports")
	flag.StringVar(&opts.Mode, "mode", "rule-only", "review mode")
	flag.StringVar(&opts.SQLitePath, "sqlite", "", "sqlite database path")
	flag.StringVar(&opts.Runtime, "runtime", "container", "executor runtime: container, local-fallback, or e2b")
	flag.StringVar(&opts.SkillsRoot, "skills-root", "skills", "path to skills root")
	flag.StringVar(&opts.FixturesRoot, "fixtures-root", "testdata/fixtures", "path to fixture diffs")
	flag.StringVar(&opts.ModelProvider, "model-provider", "", "optional model provider: http")
	flag.StringVar(&opts.ModelEndpoint, "model-endpoint", "", "HTTP model provider endpoint")
	flag.StringVar(&opts.ModelAPIKeyEnv, "model-api-key-env", "", "environment variable containing the HTTP model provider API key")
	flag.StringVar(&opts.ModelName, "model-name", "", "model name sent to the HTTP model provider")
	flag.BoolVar(&opts.Staticcheck, "staticcheck", false, "run optional staticcheck in sandbox mode")
	flag.Parse()

	if err := Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
