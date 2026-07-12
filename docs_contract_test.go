package main

import (
	"os"
	"strings"
	"testing"
)

// TestREADMETracksFrameworkFirstCLIContract 固定 README 的 trpc-agent-go 契约。
func TestREADMETracksFrameworkFirstCLIContract(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"trpc-agent-go/tool/skill",
		"tool.PermissionPolicy",
		"codeexecutor/container",
		"--fixture",
		"--runtime",
		"--staticcheck",
		"local-fallback",
		"review_report.json",
		"examples/review_report.json",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("README.md should mention %q", want)
		}
	}
}

func TestReadmeHasChineseDefaultAndEnglishVersion(t *testing.T) {
	zh, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	en, err := os.ReadFile("README.en.md")
	if err != nil {
		t.Fatalf("read README.en.md: %v", err)
	}
	if !strings.Contains(string(zh), "English version: [README.en.md](README.en.md)") ||
		!strings.Contains(string(zh), "## 快速开始") {
		t.Fatalf("README.md should be the default Chinese entrypoint")
	}
	if !strings.Contains(string(en), "Chinese version: [README.md](README.md)") ||
		!strings.Contains(string(en), "## Quick Start") {
		t.Fatalf("README.en.md should keep the English entrypoint")
	}
}

func TestReviewerGuideDocumentsReviewSurfaceAndLimits(t *testing.T) {
	data, err := os.ReadFile("docs/reviewer-guide.md")
	if err != nil {
		t.Fatalf("read reviewer guide: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"## Review Surface",
		"cmd/review-agent",
		"internal/agent/reports.go",
		"internal/report/report.go",
		"scripts/llm_semantic_eval.sh",
		"## Safety Boundaries",
		"tool.PermissionPolicy",
		"timeout",
		"output limit",
		"env whitelist",
		"redaction",
		"artifact size cap",
		"## Testing Matrix",
		"go test ./...",
		"go test -tags=integration -p 1",
		"scripts/eval.sh",
		"scripts/holdout_eval.sh",
		"scripts/llm_semantic_eval.sh",
		"scripts/repo_llm_smoke.sh",
		"## Not Tested / Known Limits",
		"real E2B",
		"model output can vary",
		"SQLite reports table",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reviewer guide should mention %q", want)
		}
	}
}

func TestReadmesPointReviewersToEvidenceAndLimits(t *testing.T) {
	for _, path := range []string{"README.md", "README.en.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, want := range []string{
			"docs/reviewer-guide.md",
			"Testing Matrix",
			"live LLM evidence",
			"Not-tested",
			"not a hard CI gate",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s should mention %q", path, want)
			}
		}
	}
}
