package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalScriptAcceptsExternalExpectedMatrix(t *testing.T) {
	root := repoRoot(t)
	expected := filepath.Join(t.TempDir(), "expected.tsv")
	if err := os.WriteFile(expected, []byte("secret.diff\tsecret-leak\tcritical\tfinding\ttrue\n"), 0o644); err != nil {
		t.Fatalf("write expected matrix: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=safe.diff secret.diff",
		"CR_AGENT_EVAL_EXPECTED="+expected,
		"CR_AGENT_EVAL_FIXTURES_ROOT="+filepath.Join(root, "testdata", "fixtures"),
		"CR_AGENT_EVAL_SKILLS_ROOT="+filepath.Join(root, "skills"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval.sh failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		"matrix_source=external",
		"true_positive=1",
		"false_positive=0",
		"false_negative=0",
		"recall=1.000",
		"precision=1.000",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("eval output missing %q: %s", want, text)
		}
	}
}

func TestEvalScriptAcceptsHiddenMatrixPath(t *testing.T) {
	root := repoRoot(t)
	matrix := filepath.Join(t.TempDir(), "hidden.tsv")
	if err := os.WriteFile(matrix, []byte("secret.diff\tsecret-leak\tcritical\tfinding\ttrue\n"), 0o644); err != nil {
		t.Fatalf("write hidden matrix: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=safe.diff secret.diff",
		"CR_AGENT_EVAL_MATRIX="+matrix,
		"CR_AGENT_EVAL_FIXTURES_ROOT="+filepath.Join(root, "testdata", "fixtures"),
		"CR_AGENT_EVAL_SKILLS_ROOT="+filepath.Join(root, "skills"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval.sh failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		"matrix_source=external",
		"true_positive=1",
		"false_positive=0",
		"false_negative=0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("eval output missing %q: %s", want, text)
		}
	}
}

func TestEvalScriptFailsClearlyForMissingHiddenMatrixPath(t *testing.T) {
	root := repoRoot(t)
	missing := filepath.Join(t.TempDir(), "missing-hidden.tsv")

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=safe.diff",
		"CR_AGENT_EVAL_MATRIX="+missing,
		"CR_AGENT_EVAL_FIXTURES_ROOT="+filepath.Join(root, "testdata", "fixtures"),
		"CR_AGENT_EVAL_SKILLS_ROOT="+filepath.Join(root, "skills"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected eval.sh to fail for missing matrix, output: %s", out)
	}
	if !strings.Contains(string(out), "CR_AGENT_EVAL_MATRIX does not exist") {
		t.Fatalf("expected clear missing matrix error, got: %s", out)
	}
}

func TestLLMSmokeSkipsWithoutOptIn(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_smoke.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_LLM_SMOKE=",
		"OPENAI_API_KEY=",
		"DEEPSEEK_API_KEY=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm_smoke.sh should skip without opt-in: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "[SKIP]") {
		t.Fatalf("expected skip output, got: %s", out)
	}
}

func TestLLMSmokeSkipsWithoutAPIKey(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_smoke.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_LLM_SMOKE=1",
		"CR_AGENT_LLM_PROVIDER=deepseek",
		"DEEPSEEK_API_KEY=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm_smoke.sh should skip without API key: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "[SKIP] DEEPSEEK_API_KEY is not set") {
		t.Fatalf("expected missing key skip output, got: %s", out)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(dir)
}
