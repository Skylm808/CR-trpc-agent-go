package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/approval"
	"github.com/Skylm808/CR-trpc-agent-go/internal/execution"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// runGoSandboxChecks executes Go project checks in the configured runtime.
func (a *Agent) runGoSandboxChecks(ctx context.Context, taskID string, repoPath string) ([]storage.DecisionRecord, []storage.SandboxRunRecord) {
	commands := approval.AllowedReviewCommands(a.cfg.EnableStaticcheck)
	decisions := make([]storage.DecisionRecord, 0, len(commands))
	runs := make([]storage.SandboxRunRecord, 0, len(commands))
	for _, command := range commands {
		decision, run := a.runGoSandboxCommand(ctx, taskID, repoPath, command)
		decisions = append(decisions, decision)
		runs = append(runs, run)
	}
	return decisions, runs
}

// runGoSandboxCommand executes one approved Go check command.
func (a *Agent) runGoSandboxCommand(ctx context.Context, taskID string, repoPath string, command string) (storage.DecisionRecord, storage.SandboxRunRecord) {
	execCommand := goSandboxExecCommand(a.cfg.Runtime, command)
	workspaceArgs, _ := execution.WorkspaceArgs(execCommand, a.cfg.Timeout, goSandboxEnv(a.cfg.Runtime))
	legacyArgs, _ := json.Marshal(map[string]any{
		"code_blocks": []map[string]string{{
			"language": "bash",
			"code":     goSandboxCode(a.cfg.Runtime, repoPath, execCommand),
		}},
		"execution_id": taskID + "-" + strings.ReplaceAll(command, " ", "-"),
	})
	permReq := &tool.PermissionRequest{
		Tool:        a.checkTool,
		ToolName:    "workspace_exec",
		Declaration: a.checkTool.Declaration(),
		Arguments:   workspaceArgs,
	}
	perm, err := a.policy.CheckToolPermission(ctx, permReq)
	if err != nil {
		perm = tool.DenyPermission(err.Error())
	}
	perm, err = tool.NormalizePermissionDecision(perm)
	if err != nil {
		perm = tool.DenyPermission(err.Error())
	}
	decision := storage.DecisionRecord{
		TaskID: taskID, Command: command,
		Action: string(perm.Action), Reason: perm.Reason, At: time.Now(),
	}
	run := storage.SandboxRunRecord{
		TaskID: taskID, Command: command, Runtime: a.cfg.Runtime,
		Status: "skipped", TimeoutMS: a.cfg.Timeout.Milliseconds(),
		OutputLimitBytes: a.cfg.OutputLimitBytes,
		EnvWhitelist:     sandboxEnvWhitelist,
		At:               time.Now(),
	}
	if perm.Action != tool.PermissionActionAllow {
		run.Status = string(perm.Action)
		return decision, run
	}

	start := time.Now()
	raw, err := a.runWorkspaceGoChecks(ctx, repoPath, execCommand)
	if err != nil {
		raw, err = a.checkTool.Call(ctx, legacyArgs)
	}
	run.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		run.Status = "error"
		run.StderrDigest = digestString(err.Error())
		return decision, run
	}
	output := sandboxCommandOutput(raw)
	run.StdoutDigest = digestString(output.Text)
	run.Output = sandboxRunOutput(output.Text, a.cfg.OutputLimitBytes)
	if output.ExitCode != nil {
		run.ExitCode = *output.ExitCode
	}
	if output.Status != "" && output.Status != "exited" && output.ExitCode == nil {
		run.Status = output.Status
		return decision, run
	}
	if run.ExitCode != 0 || strings.Contains(output.Text, "Error executing code block") {
		run.Status = "failed"
		if run.ExitCode == 0 {
			run.ExitCode = 1
		}
		return decision, run
	}
	run.Status = "ok"
	return decision, run
}
