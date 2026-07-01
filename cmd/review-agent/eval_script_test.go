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
	t.Parallel()

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
	t.Parallel()

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
