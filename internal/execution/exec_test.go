package execution

import (
	"context"
	"strings"
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

func TestSandboxEnvUsesOnlyWhitelistedKeysAndDropsSecrets(t *testing.T) {
	t.Setenv("PATH", "/host/bin")
	t.Setenv("HOME", "/Users/example")
	t.Setenv("TMPDIR", "/tmp/host")
	t.Setenv("OPENAI_API_KEY", "sk-openai-secret-1234567890")
	t.Setenv("DEEPSEEK_API_KEY", "sk-deepseek-secret-1234567890")
	t.Setenv("CR_AGENT_TEST_SECRET", "secret-value")

	env := SandboxEnv(RuntimeLocalFallback)
	for key, value := range env {
		if !AllowedSandboxEnvKey(key) {
			t.Fatalf("sandbox env included non-whitelisted key %q=%q", key, value)
		}
		if strings.Contains(value, "secret") || strings.Contains(value, "sk-") {
			t.Fatalf("sandbox env leaked secret value through %q=%q", key, value)
		}
	}
	for _, forbidden := range []string{"OPENAI_API_KEY", "DEEPSEEK_API_KEY", "CR_AGENT_TEST_SECRET"} {
		if _, ok := env[forbidden]; ok {
			t.Fatalf("sandbox env must not include secret key %q: %+v", forbidden, env)
		}
	}
	if env["GOCACHE"] != GoSandboxCacheDir {
		t.Fatalf("GOCACHE = %q, want %q", env["GOCACHE"], GoSandboxCacheDir)
	}
	if env["PATH"] != "/host/bin" {
		t.Fatalf("local fallback PATH = %q, want host PATH", env["PATH"])
	}
}

func TestSandboxEnvWhitelistMatchesActualEnvKeys(t *testing.T) {
	t.Parallel()

	env := SandboxEnv(RuntimeContainer)
	for _, key := range []string{"PATH", "GOCACHE"} {
		if _, ok := env[key]; !ok {
			t.Fatalf("container env missing expected key %q: %+v", key, env)
		}
		if !strings.Contains(SandboxEnvWhitelist, key) {
			t.Fatalf("audit whitelist %q does not include actual key %q", SandboxEnvWhitelist, key)
		}
	}
	for _, audited := range strings.Split(SandboxEnvWhitelist, ",") {
		if strings.TrimSpace(audited) == "" {
			t.Fatalf("empty env whitelist entry in %q", SandboxEnvWhitelist)
		}
		if !AllowedSandboxEnvKey(strings.TrimSpace(audited)) {
			t.Fatalf("audit whitelist contains non-allowed key %q", audited)
		}
	}
}

func TestContainerSandboxEnvUsesContainerLocalPaths(t *testing.T) {
	t.Setenv("HOME", "/Users/example")
	t.Setenv("TMPDIR", "/var/folders/example-host-tmp")

	env := SandboxEnv(RuntimeContainer)
	if env["HOME"] != "/tmp" {
		t.Fatalf("container HOME = %q, want /tmp", env["HOME"])
	}
	if env["TMPDIR"] != "/tmp" {
		t.Fatalf("container TMPDIR = %q, want /tmp", env["TMPDIR"])
	}
}

func TestFakeExecutionRuntimeIsTestOnlyAndSeparateFromLocalFallback(t *testing.T) {
	t.Parallel()

	if RuntimeFakeExecution == RuntimeLocalFallback {
		t.Fatalf("fake execution runtime must not alias local fallback")
	}
	exec, err := NewExecutor(Config{Runtime: RuntimeFakeExecution})
	if err != nil {
		t.Fatalf("NewExecutor fake runtime returned error: %v", err)
	}
	if _, ok := exec.(FakeExecutor); !ok {
		t.Fatalf("expected FakeExecutor, got %T", exec)
	}
	result, err := exec.ExecuteCode(context.Background(), codeexecutor.CodeExecutionInput{
		CodeBlocks: []codeexecutor.CodeBlock{{
			Language: "bash",
			Code:     "echo should-not-run",
		}},
	})
	if err != nil {
		t.Fatalf("fake executor should not error: %v", err)
	}
	if !strings.Contains(result.Output, RuntimeFakeExecution) {
		t.Fatalf("fake executor output should identify test-only runtime, got %q", result.Output)
	}
}
