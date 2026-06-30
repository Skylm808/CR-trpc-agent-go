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
