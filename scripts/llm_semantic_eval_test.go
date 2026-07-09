package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLLMSemanticEvalSkipsWithoutOptIn(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_semantic_eval.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_LLM_SMOKE=",
		"OPENAI_API_KEY=",
		"DEEPSEEK_API_KEY=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm_semantic_eval.sh should skip without opt-in: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "[SKIP]") {
		t.Fatalf("expected skip output, got: %s", out)
	}
}

func TestLLMSemanticEvalHelpDocumentsBilingualSummary(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_semantic_eval.sh"), "--help")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm_semantic_eval.sh --help failed: %v\n%s", err, out)
	}
	output := string(out)
	for _, want := range []string{
		"llm_semantic_eval.sh",
		"--fixtures",
		"--output-root",
		"--expected-file",
		"llm_semantic_eval.md",
		"llm_semantic_eval.zh.md",
		"llm_semantic_eval.en.md",
		"Chinese summary",
		"English summary",
		"fixture-level recall",
		"not a deterministic CI gate",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected help to contain %q, got %s", want, output)
		}
	}
}

func TestLLMSemanticEvalFakeProviderWritesSummariesAndMetrics(t *testing.T) {
	root := repoRoot(t)
	outDir := t.TempDir()
	config := filepath.Join(t.TempDir(), "cr-agent.yaml")
	if err := os.WriteFile(config, []byte("mode: fake-model\nruntime: local-fallback\nstaticcheck: false\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "llm_semantic_eval.sh"),
		"--config", config,
		"--fixtures", "model-semantic.diff",
		"--output-root", outDir,
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CR_AGENT_LLM_SMOKE=1",
		"GOCACHE=/private/tmp/cr-agent-gocache",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm_semantic_eval.sh fake provider failed: %v\n%s", err, out)
	}

	for _, name := range []string{"llm_semantic_eval.md", "llm_semantic_eval.zh.md", "llm_semantic_eval.en.md"} {
		data, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(data)
		for _, want := range []string{"model-semantic.diff", "Fixture-level recall"} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected %s to contain %q, got %s", name, want, text)
			}
		}
		if !strings.Contains(text, "Expected risky fixtures") && !strings.Contains(text, "预期有语义风险") {
			t.Fatalf("expected %s to contain risky-fixture metrics, got %s", name, text)
		}
	}
}
