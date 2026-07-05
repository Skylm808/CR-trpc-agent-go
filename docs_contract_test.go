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
