package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cragent "github.com/Skylm808/CR-trpc-agent-go/internal/agent"
)

func TestRunAutoLoadsCurrentDirectoryConfig(t *testing.T) {
	repo := t.TempDir()
	out := filepath.Join(repo, "reports")
	skillsRoot := absoluteSkillsRoot(t)
	writeReviewRepo(t, repo)
	writeFile(t, filepath.Join(repo, "cr-agent.yaml"), ""+
		"mode: rule-only\n"+
		"runtime: local-fallback\n"+
		"output_dir: "+slashPath(out)+"\n"+
		"skills_root: "+slashPath(skillsRoot)+"\n")

	withWorkingDirectory(t, repo, func() {
		if err := Run(Options{}); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	})
	assertFileExists(t, filepath.Join(out, "review_report.json"))
}

func TestRunLoadsExplicitConfigFile(t *testing.T) {
	repo := t.TempDir()
	cfgDir := t.TempDir()
	out := filepath.Join(repo, "explicit-reports")
	writeReviewRepo(t, repo)
	writeFile(t, filepath.Join(cfgDir, "review.yaml"), ""+
		"mode: rule-only\n"+
		"runtime: local-fallback\n"+
		"repo_path: "+slashPath(repo)+"\n"+
		"output_dir: "+slashPath(out)+"\n"+
		"skills_root: "+slashPath(absoluteSkillsRoot(t))+"\n")

	if err := Run(Options{ConfigFile: filepath.Join(cfgDir, "review.yaml")}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	assertFileExists(t, filepath.Join(out, "review_report.json"))
}

func TestRunCLIOptionsOverrideConfig(t *testing.T) {
	repo := t.TempDir()
	configOut := filepath.Join(repo, "config-reports")
	overrideOut := filepath.Join(repo, "override-reports")
	writeReviewRepo(t, repo)
	configPath := filepath.Join(repo, "cr-agent.yaml")
	writeFile(t, configPath, ""+
		"mode: fake-model\n"+
		"runtime: local-fallback\n"+
		"repo_path: "+slashPath(repo)+"\n"+
		"output_dir: "+slashPath(configOut)+"\n"+
		"skills_root: "+slashPath(absoluteSkillsRoot(t))+"\n"+
		"model:\n"+
		"  provider: deepseek\n"+
		"  name: deepseek-chat\n"+
		"  api_key_env: CR_AGENT_TEST_MISSING_DEEPSEEK_KEY\n")

	opts, err := parseOptions([]string{
		"--config", configPath,
		"--output-dir", overrideOut,
		"--mode", cragent.ModeRuleOnly,
		"--runtime", cragent.RuntimeLocalFallback,
	})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if err := Run(opts); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	assertFileExists(t, filepath.Join(overrideOut, "review_report.json"))
	if _, err := os.Stat(filepath.Join(configOut, "review_report.json")); err == nil {
		t.Fatalf("expected CLI output-dir to override YAML output_dir")
	}
	data, err := os.ReadFile(filepath.Join(overrideOut, "review_report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if strings.Contains(string(data), "model-provider-failed") {
		t.Fatalf("expected CLI mode=rule-only to override YAML fake-model provider: %s", data)
	}
}

func TestRunDeepSeekProviderMissingAPIKeyDoesNotAbort(t *testing.T) {
	dir := t.TempDir()
	diffPath := filepath.Join(dir, "sample.diff")
	writeFile(t, diffPath, ""+
		"diff --git a/foo.go b/foo.go\n"+
		"--- a/foo.go\n"+
		"+++ b/foo.go\n"+
		"@@ -1,1 +1,2 @@\n"+
		" package foo\n"+
		"+func Add(a, b int) int { return a + b }\n")

	err := Run(Options{
		DiffFile:       diffPath,
		OutputDir:      dir,
		Mode:           cragent.ModeFakeModel,
		Runtime:        cragent.RuntimeLocalFallback,
		SkillsRoot:     absoluteSkillsRoot(t),
		ModelProvider:  "deepseek",
		ModelName:      "deepseek-chat",
		ModelAPIKeyEnv: "CR_AGENT_TEST_MISSING_DEEPSEEK_KEY",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "review_report.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(data), "model-provider-failed") ||
		!strings.Contains(string(data), "needs_human_review") {
		t.Fatalf("expected missing DeepSeek key to be recorded as model-provider-failed: %s", data)
	}
	if strings.Contains(string(data), "CR_AGENT_TEST_MISSING_DEEPSEEK_KEY") {
		t.Fatalf("report should not leak model api key env names: %s", data)
	}
}

func writeReviewRepo(t *testing.T, repo string) {
	t.Helper()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/reviewme\n")
	writeFile(t, filepath.Join(repo, "foo.go"), "package foo\n\nfunc Bad() { panic(\"boom\") }\n")
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func absoluteSkillsRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func withWorkingDirectory(t *testing.T, dir string, fn func()) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()
	fn()
}

func slashPath(path string) string {
	return filepath.ToSlash(path)
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
}
