//go:build scriptcontract

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEvalScriptReportsFixtureMetrics 固定评测脚本输出。
func TestEvalScriptReportsFixtureMetrics(t *testing.T) {
	root := repoRootForEval(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=safe.diff secret.diff",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval script failed: %v\n%s", err, out)
	}
	output := string(out)
	for _, want := range []string{"fixtures=2", "recall=1.000", "precision=1.000"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected eval output to contain %q, got %s", want, output)
		}
	}
}

// TestEvalScriptAcceptsExternalExpectedMatrix 固定 hidden matrix 输入契约。
func TestEvalScriptAcceptsExternalExpectedMatrix(t *testing.T) {
	root := repoRootForEval(t)
	expected := filepath.Join(t.TempDir(), "expected.tsv")
	if err := os.WriteFile(expected, []byte(strings.Join([]string{
		"secret.diff\tsecret-leak\tcritical\tfinding\ttrue",
		"secret.diff\tmissing-test-hint\tlow\twarning\tfalse",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write expected matrix: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=secret.diff",
		"CR_AGENT_EVAL_EXPECTED="+expected,
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval script failed: %v\n%s", err, out)
	}
	output := string(out)
	for _, want := range []string{
		"fixtures=1",
		"required_expected=1",
		"optional_expected=1",
		"recall=1.000",
		"false_positive_rate=0.000",
		"missing_findings=0",
		"unexpected_findings=0",
		"matrix_source=external",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected eval output to contain %q, got %s", want, output)
		}
	}
}

// TestEvalScriptFailsWhenRecallThresholdIsMissed 固定召回率门禁。
func TestEvalScriptFailsWhenRecallThresholdIsMissed(t *testing.T) {
	root := repoRootForEval(t)
	expected := filepath.Join(t.TempDir(), "expected.tsv")
	if err := os.WriteFile(expected, []byte("secret.diff\tmissing-rule\tcritical\tfinding\ttrue\n"), 0o644); err != nil {
		t.Fatalf("write expected matrix: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=secret.diff",
		"CR_AGENT_EVAL_EXPECTED="+expected,
		"CR_AGENT_EVAL_MIN_RECALL=0.800",
		"CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE=1.000",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected eval script to fail threshold, output:\n%s", out)
	}
	output := string(out)
	for _, want := range []string{"recall=0.000", "threshold_failed=recall"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected eval output to contain %q, got %s", want, output)
		}
	}
}

// TestEvalScriptFailsWhenFalsePositiveThresholdIsMissed 固定误报率门禁。
func TestEvalScriptFailsWhenFalsePositiveThresholdIsMissed(t *testing.T) {
	root := repoRootForEval(t)
	expected := filepath.Join(t.TempDir(), "expected.tsv")
	if err := os.WriteFile(expected, []byte("secret.diff\tmissing-test-hint\tlow\twarning\tfalse\n"), 0o644); err != nil {
		t.Fatalf("write expected matrix: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=secret.diff",
		"CR_AGENT_EVAL_EXPECTED="+expected,
		"CR_AGENT_EVAL_MIN_RECALL=0.000",
		"CR_AGENT_EVAL_MAX_FALSE_POSITIVE_RATE=0.150",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected eval script to fail false positive threshold, output:\n%s", out)
	}
	output := string(out)
	for _, want := range []string{"false_positive_rate=1.000", "threshold_failed=false_positive_rate"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected eval output to contain %q, got %s", want, output)
		}
	}
}

// TestEvalScriptKeepsReportRoot 固定失败回放报告目录。
func TestEvalScriptKeepsReportRoot(t *testing.T) {
	root := repoRootForEval(t)
	reportRoot := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=secret.diff",
		"CR_AGENT_EVAL_REPORT_ROOT="+reportRoot,
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval script failed: %v\n%s", err, out)
	}
	for _, name := range []string{"review_report.json", "review_report.md", "review_report.zh.md", "review_diagnostics.json"} {
		if _, err := os.Stat(filepath.Join(reportRoot, "secret.diff", name)); err != nil {
			t.Fatalf("expected retained report %s: %v\noutput:\n%s", name, err, out)
		}
	}
}

// TestHiddenMatrixSmokeUsesExternalRootAndMatrix proves the hidden fixture entrypoint
// without committing hidden samples.
func TestHiddenMatrixSmokeUsesExternalRootAndMatrix(t *testing.T) {
	root := repoRootForEval(t)
	reportRoot := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "hidden_matrix_smoke.sh"))
	cmd.Env = append(os.Environ(),
		"CR_AGENT_HIDDEN_SMOKE_REPORT_ROOT="+reportRoot,
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hidden matrix smoke failed: %v\n%s", err, out)
	}
	output := string(out)
	for _, want := range []string{
		"fixtures=2",
		"recall=1.000",
		"precision=1.000",
		"false_positive_rate=0.000",
		"matrix_source=external",
		"[PASS] hidden-like matrix smoke",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected hidden smoke output to contain %q, got %s", want, output)
		}
	}
	for _, fixture := range []string{"safe.diff", "secret.diff"} {
		for _, name := range []string{"review_report.json", "review_report.md", "review_report.zh.md", "review_diagnostics.json"} {
			if _, err := os.Stat(filepath.Join(reportRoot, fixture, name)); err != nil {
				t.Fatalf("expected hidden-like report artifact %s/%s: %v\noutput:\n%s", fixture, name, err, out)
			}
		}
	}
}

// TestRepoLLMSmokeScriptDocumentsPortableRepoEntry 固定任意本地 git repo 的 LLM smoke 入口。
func TestRepoLLMSmokeScriptDocumentsPortableRepoEntry(t *testing.T) {
	root := repoRootForEval(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "repo_llm_smoke.sh"), "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("repo LLM smoke help failed: %v\n%s", err, out)
	}
	output := string(out)
	for _, want := range []string{
		"repo_llm_smoke.sh",
		"--repo",
		"--config",
		"--go-only",
		"--output-dir",
		"model_provider",
		"model_call_count=1",
		"review_report.zh.md",
		"no API key leakage",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected repo smoke help to contain %q, got %s", want, output)
		}
	}
}

// TestUpstreamExampleSmokeDocumentsMigrationEntry 固定官方 examples 迁移演练入口。
func TestUpstreamExampleSmokeDocumentsMigrationEntry(t *testing.T) {
	root := repoRootForEval(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "upstream_example_smoke.sh"), "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("upstream example smoke help failed: %v\n%s", err, out)
	}
	output := string(out)
	for _, want := range []string{
		"upstream_example_smoke.sh",
		"trpc-agent-go/examples/cr-agent",
		"--work-dir",
		"--keep",
		"go run ./cmd/review-agent",
		"cr-agent.example.yaml",
		"review_report.json",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected upstream example smoke help to contain %q, got %s", want, output)
		}
	}
}

// repoRootForEval 查找仓库根目录。
func repoRootForEval(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = next
	}
}
