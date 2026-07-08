package agent

import (
	"context"

	"github.com/Skylm808/CR-trpc-agent-go/internal/approval"
	"github.com/Skylm808/CR-trpc-agent-go/internal/execution"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// newExecutor creates the trpc-agent-go executor selected by Config.Runtime.
func newExecutor(cfg Config) (codeexecutor.CodeExecutor, error) {
	return execution.NewExecutor(execution.Config{
		Runtime:               cfg.Runtime,
		Timeout:               cfg.Timeout,
		ContainerRepoHostPath: cfg.ContainerRepoHostPath,
	})
}

type unsupportedExecutor = execution.UnsupportedExecutor

// defaultPermissionPolicy returns the fixed review-command allowlist.
func defaultPermissionPolicy() tool.PermissionPolicy {
	return approval.NewPermissionPolicy(defaultSkillCommand, approval.AllowedReviewCommands(true))
}

func (a *Agent) runWorkspaceGoChecks(ctx context.Context, repoPath string, command string) (any, error) {
	return execution.RunWorkspaceCommand(ctx, a.exec, repoPath, command, a.cfg.Timeout, goSandboxEnv(a.cfg.Runtime))
}

func goSandboxExecCommand(runtime string, command string) string {
	return execution.SandboxExecCommand(runtime, command)
}

func goSandboxEnv(runtime string) map[string]string {
	return execution.SandboxEnv(runtime)
}

func shellQuote(value string) string {
	return execution.ShellQuote(value)
}

func sandboxRepoPathForRuntime(runtime string, hostRepoPath string) string {
	return execution.SandboxRepoPathForRuntime(runtime, hostRepoPath)
}

func goSandboxCode(runtime string, hostRepoPath string, command string) string {
	return execution.SandboxCode(runtime, hostRepoPath, command)
}
