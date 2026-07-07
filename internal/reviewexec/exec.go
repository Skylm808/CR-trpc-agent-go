// Package reviewexec owns sandbox runtime and Go check execution helpers.
package reviewexec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	containerexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/container"
	localexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
	workspaceexec "trpc.group/trpc-go/trpc-agent-go/tool/workspaceexec"
)

const (
	// RuntimeContainer is the production sandbox path.
	RuntimeContainer = "container"
	// RuntimeLocalFallback is explicit local development fallback.
	RuntimeLocalFallback = "local-fallback"
	// RuntimeE2B is an explicit unsupported remote sandbox placeholder.
	RuntimeE2B = "e2b"
	// RuntimeFakeExecution is a test-only seam; it is not a production fallback.
	RuntimeFakeExecution = "fake-execution"

	ContainerRepoMountPath = "/workspace/repo"
	DefaultContainerImage  = "golang:1.25-bookworm"
	GoSandboxCacheDir      = "/tmp/cr-agent-gocache"
	GoSandboxBinary        = "/usr/local/go/bin/go"
	GoSandboxPath          = "/go/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
	SandboxEnvWhitelist    = "PATH,HOME,TMPDIR,GOCACHE"
)

// Config captures the runtime-specific executor settings.
type Config struct {
	Runtime               string
	Timeout               time.Duration
	ContainerRepoHostPath string
}

// NewExecutor creates the trpc-agent-go code executor for the configured runtime.
func NewExecutor(cfg Config) (codeexecutor.CodeExecutor, error) {
	switch cfg.Runtime {
	case RuntimeLocalFallback:
		workDir, err := os.MkdirTemp("", "cr-agent-localexec-*")
		if err != nil {
			return nil, fmt.Errorf("create local fallback workdir: %w", err)
		}
		return localexec.New(
			localexec.WithTimeout(cfg.Timeout),
			localexec.WithWorkDir(workDir),
		), nil
	case RuntimeContainer:
		opts := []containerexec.Option{
			containerexec.WithContainerConfig(dockercontainer.Config{
				Image:      DefaultContainerImage,
				WorkingDir: "/",
				Cmd:        []string{"tail", "-f", "/dev/null"},
				Tty:        true,
				OpenStdin:  true,
				Env: []string{
					"PATH=" + GoSandboxPath,
					"HOME=/tmp",
					"TMPDIR=/tmp",
					"GOCACHE=" + GoSandboxCacheDir,
					"GOPATH=/go",
					"GOTOOLCHAIN=local",
				},
			}),
		}
		if strings.TrimSpace(cfg.ContainerRepoHostPath) != "" {
			opts = append(opts, containerexec.WithBindMount(cfg.ContainerRepoHostPath, ContainerRepoMountPath, "ro"))
		}
		exec, err := containerexec.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("create container executor: %w", err)
		}
		return exec, nil
	case RuntimeE2B:
		return UnsupportedExecutor{Runtime: RuntimeE2B}, nil
	case RuntimeFakeExecution:
		return FakeExecutor{}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime %q", cfg.Runtime)
	}
}

// UnsupportedExecutor records an explicit unsupported runtime instead of falling back.
type UnsupportedExecutor struct {
	Runtime string
}

func (e UnsupportedExecutor) ExecuteCode(context.Context, codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
	return codeexecutor.CodeExecutionResult{}, fmt.Errorf("runtime %q is not supported by this adapter yet", e.Runtime)
}

func (e UnsupportedExecutor) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return codeexecutor.CodeBlockDelimiter{Start: "```", End: "```"}
}

// FakeExecutor is a test-only runtime seam that never invokes shell or Docker.
type FakeExecutor struct{}

func (FakeExecutor) ExecuteCode(context.Context, codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
	return codeexecutor.CodeExecutionResult{
		Output: RuntimeFakeExecution + ": test-only executor did not run code",
	}, nil
}

func (FakeExecutor) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return codeexecutor.CodeBlockDelimiter{Start: "```", End: "```"}
}

// SandboxExecCommand returns the actual command used inside the runtime.
func SandboxExecCommand(runtime string, command string) string {
	if runtime == RuntimeContainer && strings.HasPrefix(command, "go ") {
		return GoSandboxBinary + strings.TrimPrefix(command, "go")
	}
	return command
}

// SandboxEnv returns the exact environment passed to workspace execution.
func SandboxEnv(runtime string) map[string]string {
	pathValue := GoSandboxPath
	if runtime == RuntimeLocalFallback && os.Getenv("PATH") != "" {
		pathValue = os.Getenv("PATH")
	}
	homeValue := "/tmp"
	tmpdirValue := "/tmp"
	if runtime == RuntimeLocalFallback {
		homeValue = sandboxEnvValue("HOME", "/tmp")
		tmpdirValue = sandboxEnvValue("TMPDIR", "/tmp")
	}
	return map[string]string{
		"GOCACHE": GoSandboxCacheDir,
		"HOME":    homeValue,
		"PATH":    pathValue,
		"TMPDIR":  tmpdirValue,
	}
}

// AllowedSandboxEnvKey reports whether a key is allowed into sandbox command specs.
func AllowedSandboxEnvKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "PATH", "HOME", "TMPDIR", "GOCACHE":
		return true
	default:
		return false
	}
}

func sandboxEnvValue(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" || strings.Contains(value, "sk-") || strings.Contains(strings.ToLower(value), "secret") {
		return fallback
	}
	return value
}

// WorkspaceArgs builds the workspace execution arguments for a Go check command.
func WorkspaceArgs(command string, timeout time.Duration, env map[string]string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"command": command,
		"cwd":     "work/repo",
		"timeout": int(timeout.Seconds()),
		"env":     env,
	})
}

// RunWorkspaceCommand runs a command in an executor workspace populated from a host repo.
func RunWorkspaceCommand(ctx context.Context, exec codeexecutor.CodeExecutor, repoPath string, command string, timeout time.Duration, env map[string]string) (any, error) {
	if exec == nil {
		return nil, fmt.Errorf("workspace exec is not configured")
	}
	tool := workspaceexec.NewExecTool(exec,
		workspaceexec.WithWorkspaceBootstrap(codeexecutor.WorkspaceBootstrapSpec{
			Files: []codeexecutor.WorkspaceFile{{
				Target: "work/repo",
				Input: &codeexecutor.InputSpec{
					From: "host://" + repoPath,
					To:   "work/repo/.repo",
					Mode: "copy",
				},
			}},
		}),
	)
	args, err := WorkspaceArgs(command, timeout, env)
	if err != nil {
		return nil, err
	}
	return tool.Call(ctx, args)
}

// SandboxRepoPathForRuntime returns the repo path visible inside the runtime.
func SandboxRepoPathForRuntime(runtime string, hostRepoPath string) string {
	if runtime == RuntimeContainer {
		return ContainerRepoMountPath
	}
	return hostRepoPath
}

// SandboxCode builds the legacy codeexec fallback command.
func SandboxCode(runtime string, hostRepoPath string, command string) string {
	return "cd " + ShellQuote(SandboxRepoPathForRuntime(runtime, hostRepoPath)) +
		" && GOCACHE=" + ShellQuote(GoSandboxCacheDir) + " " + command
}

// ShellQuote returns a POSIX single-quoted value.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
