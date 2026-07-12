//go:build integration

package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalScriptBuildsCLIOnceAndReusesIt(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go.log")
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("find go: %v", err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	fakeGo := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${EVAL_TEST_LOG}"
if [[ "$1" == "run" ]]; then exec "${EVAL_TEST_REAL_GO}" "$@"; fi
if [[ "$1" != "build" ]]; then exit 2; fi
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then out="$2"; shift 2; continue; fi
  shift
done
cat > "$out" <<'CLI'
#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--output-dir" ]]; then out="$2"; shift 2; continue; fi
  shift
done
mkdir -p "$out"
printf '%s\n' '{"findings":[],"warnings":[],"human_review_items":[],"metrics":{"exception_counts":{}}}' > "$out/review_report.json"
CLI
chmod +x "$out"
`
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(fakeGo), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_FIXTURES=safe.diff safe.diff",
		"CR_AGENT_EVAL_EXPECTED="+writeEmptyExpected(t),
		"EVAL_TEST_LOG="+logPath,
		"EVAL_TEST_REAL_GO="+realGo,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval.sh failed: %v\n%s", err, out)
	}
	invocations, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read go log: %v", err)
	}
	if got := strings.Count(string(invocations), "build -o "); got != 1 {
		t.Fatalf("CLI build count = %d, want 1; log=%q", got, invocations)
	}
	if strings.Contains(string(invocations), "run "+filepath.Join(root, "cmd", "review-agent")) {
		t.Fatalf("eval invoked go run for CLI: %q", invocations)
	}
}

func TestEvalScriptFailsInfrastructureSeparatelyFromRuleFalsePositives(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	fakeCLI := filepath.Join(dir, "review-agent")
	script := `#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--output-dir" ]]; then out="$2"; shift 2; continue; fi
  shift
done
mkdir -p "$out"
cat > "$out/review_report.json" <<'JSON'
{"findings":[],"warnings":[],"human_review_items":[{"rule_id":"sandbox-command-failed","severity":"medium","status":"needs_human_review"}],"metrics":{"exception_counts":{"sandbox_failed":1}}}
JSON
`
	if err := os.WriteFile(fakeCLI, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake CLI: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "eval.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_EVAL_BINARY="+fakeCLI,
		"CR_AGENT_EVAL_FIXTURES=safe.diff",
		"CR_AGENT_EVAL_EXPECTED="+writeEmptyExpected(t),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("eval.sh should fail for infrastructure failure:\n%s", out)
	}
	text := string(out)
	for _, want := range []string{"false_positive=0", "infrastructure_failures=1", "threshold_failed=infrastructure_failure"} {
		if !strings.Contains(text, want) {
			t.Fatalf("eval output missing %q: %s", want, text)
		}
	}
}

func writeEmptyExpected(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "expected.tsv")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty expected matrix: %v", err)
	}
	return path
}

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

func TestHoldoutEvalScriptRunsSelfContainedMatrix(t *testing.T) {
	if os.Getenv("CR_AGENT_RUN_HOLDOUT_MATRIX_TEST") != "1" {
		t.Skip("full holdout matrix is owned by scripts/holdout_eval.sh; set CR_AGENT_RUN_HOLDOUT_MATRIX_TEST=1 for the redundant Go harness")
	}
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "holdout_eval.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("holdout_eval.sh failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		"fixtures=17",
		"expected=31",
		"matrix_source=holdout",
		"recall=1.000",
		"precision=1.000",
		"false_positive_rate=0.000",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("holdout eval output missing %q: %s", want, text)
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
		"CR_AGENT_LLM_CONFIG="+filepath.Join(t.TempDir(), "missing.yaml"),
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

func TestLLMSmokeConfigSkipDoesNotPrintConfigContent(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cr-agent.yaml")
	secret := "sk-smokeconfig-1234567890abcdef"
	if err := os.WriteFile(configPath, []byte("mode: fake-model\nmodel:\n  provider: deepseek\n  api_key: "+secret+"\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_smoke.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_LLM_SMOKE=",
		"CR_AGENT_LLM_CONFIG="+configPath,
		"DEEPSEEK_API_KEY=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm_smoke.sh should skip without opt-in: %v\n%s", err, out)
	}
	if strings.Contains(string(out), secret) || strings.Contains(string(out), "api_key") {
		t.Fatalf("smoke output leaked config content: %s", out)
	}
}

func TestLLMSmokeConfigInvalidAPIKeyEnvDoesNotPrintValue(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cr-agent.yaml")
	secretLikeValue := "sk-invalid-env-name-1234567890abcdef"
	if err := os.WriteFile(configPath, []byte("mode: fake-model\nmodel:\n  provider: deepseek\n  api_key_env: "+secretLikeValue+"\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_smoke.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_LLM_SMOKE=1",
		"CR_AGENT_LLM_CONFIG="+configPath,
		"DEEPSEEK_API_KEY=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm_smoke.sh should skip invalid api_key_env without failing: %v\n%s", err, out)
	}
	if strings.Contains(string(out), secretLikeValue) {
		t.Fatalf("smoke output leaked invalid api_key_env value: %s", out)
	}
	if !strings.Contains(string(out), "configured api_key_env is invalid") {
		t.Fatalf("expected invalid api_key_env skip output, got: %s", out)
	}
}

func TestLLMSmokeFailsWhenProviderFailed(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cr-agent.yaml")
	if err := os.WriteFile(configPath, []byte("mode: fake-model\nmodel:\n  provider: deepseek\n  api_key: sk-smokefail-1234567890\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	fakeGo := `#!/usr/bin/env bash
set -euo pipefail
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--output-dir" ]]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
mkdir -p "$out"
cat > "$out/review_report.json" <<'JSON'
{"findings":[],"metrics":{"model_call_count":1,"model_provider":"deepseek"},"human_review_items":[{"rule_id":"model-provider-failed","status":"needs_human_review"}],"input_metadata":{"module_path":"example.com/cragentsmoke"}}
JSON
cat > "$out/review_diagnostics.json" <<'JSON'
{"metrics":{"model_call_count":1,"model_provider":"deepseek"},"conclusion":{"status":"needs_human_review"}}
JSON
`
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(fakeGo), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_smoke.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_LLM_SMOKE=1",
		"CR_AGENT_LLM_CONFIG="+configPath,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("llm_smoke.sh should fail when provider failed, output:\n%s", out)
	}
	if !strings.Contains(string(out), "model provider failed") {
		t.Fatalf("expected provider failure output, got: %s", out)
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
