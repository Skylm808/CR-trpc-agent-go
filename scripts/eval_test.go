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

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(dir)
}
