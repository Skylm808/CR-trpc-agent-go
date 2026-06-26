package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var opts Options
	flag.StringVar(&opts.DiffFile, "diff-file", "", "path to unified diff")
	flag.StringVar(&opts.RepoPath, "repo-path", "", "path to repository")
	flag.StringVar(&opts.OutputDir, "output-dir", ".", "directory for reports")
	flag.StringVar(&opts.Mode, "mode", "rule-only", "review mode")
	flag.StringVar(&opts.SQLitePath, "sqlite", "", "sqlite database path")
	flag.StringVar(&opts.Runtime, "runtime", "container", "executor runtime: container or local-fallback")
	flag.StringVar(&opts.SkillsRoot, "skills-root", "skills", "path to skills root")
	flag.Parse()

	if err := Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
