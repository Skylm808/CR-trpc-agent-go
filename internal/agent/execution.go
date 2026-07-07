package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/reviewexec"
	"github.com/Skylm808/CR-trpc-agent-go/internal/reviewgate"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// newExecutor 创建 trpc-agent-go 执行器。
func newExecutor(cfg Config) (codeexecutor.CodeExecutor, error) {
	return reviewexec.NewExecutor(reviewexec.Config{
		Runtime:               cfg.Runtime,
		Timeout:               cfg.Timeout,
		ContainerRepoHostPath: cfg.ContainerRepoHostPath,
	})
}

type unsupportedExecutor = reviewexec.UnsupportedExecutor

// defaultPermissionPolicy 返回固定命令白名单。
func defaultPermissionPolicy() tool.PermissionPolicy {
	return reviewgate.NewPermissionPolicy(defaultSkillCommand, reviewgate.AllowedReviewCommands(true))
}

// runDryRun 只加载 Skill 并记录跳过执行的治理/沙箱摘要。
func (a *Agent) runDryRun(ctx context.Context, taskID string) (review.Result, storage.SandboxRunRecord, storage.DecisionRecord, error) {
	// dry-run 仍验证 Skill 可加载。
	loadArgs := []byte(`{"skill":"code-review"}`)
	if _, err := a.loadTool.Call(ctx, loadArgs); err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}
	now := time.Now()
	// 记录跳过执行的审计摘要。
	decision := storage.DecisionRecord{
		TaskID:  taskID,
		Command: defaultSkillCommand,
		Action:  "dry_run",
		Reason:  "executor skipped by dry-run mode",
		At:      now,
	}
	runRecord := storage.SandboxRunRecord{
		TaskID:           taskID,
		Command:          defaultSkillCommand,
		Runtime:          a.cfg.Runtime,
		Status:           "skipped",
		TimeoutMS:        a.cfg.Timeout.Milliseconds(),
		OutputLimitBytes: a.cfg.OutputLimitBytes,
		EnvWhitelist:     sandboxEnvWhitelist,
		At:               now,
	}
	return review.Result{
		Warnings: []review.Finding{{
			Severity:       "low",
			Category:       "governance",
			Title:          "Sandbox execution skipped by dry-run mode",
			Evidence:       "dry-run",
			Recommendation: "Run again with rule-only or sandbox mode before merging.",
			Confidence:     "high",
			Source:         "mode",
			RuleID:         "dry-run-skipped-executor",
			Status:         "needs_human_review",
		}},
	}, runRecord, decision, nil
}

// runSkillChecks 执行 code-review Skill。
func (a *Agent) runSkillChecks(ctx context.Context, taskID string, diff []byte) (review.Result, storage.SandboxRunRecord, storage.DecisionRecord, error) {
	// 先加载受控 Skill。
	loadArgs := []byte(`{"skill":"code-review"}`)
	if _, err := a.loadTool.Call(ctx, loadArgs); err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}

	// diff 通过 stdin 传给脚本。
	runArgs, err := json.Marshal(map[string]any{
		"skill":   defaultSkillName,
		"command": defaultSkillCommand,
		"stdin":   string(diff),
		"timeout": int(a.cfg.Timeout.Seconds()),
	})
	if err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}

	// skill_run 也必须先审批。
	permReq := &tool.PermissionRequest{
		Tool:        a.runTool,
		ToolName:    a.runTool.Declaration().Name,
		Declaration: a.runTool.Declaration(),
		Arguments:   runArgs,
	}
	perm, err := a.policy.CheckToolPermission(ctx, permReq)
	if err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}
	perm, err = tool.NormalizePermissionDecision(perm)
	if err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}
	decision := storage.DecisionRecord{
		TaskID: taskID, Command: defaultSkillCommand,
		Action: string(perm.Action), Reason: perm.Reason, At: time.Now(),
	}
	runRecord := storage.SandboxRunRecord{
		TaskID: taskID, Command: defaultSkillCommand,
		Runtime: a.cfg.Runtime, TimeoutMS: a.cfg.Timeout.Milliseconds(),
		OutputLimitBytes: a.cfg.OutputLimitBytes,
		EnvWhitelist:     sandboxEnvWhitelist,
		At:               time.Now(),
	}
	if perm.Action != tool.PermissionActionAllow {
		// 非 allow 转为人工复核项。
		runRecord.Status = string(perm.Action)
		return review.Result{Warnings: []review.Finding{{
			Severity: "low", Category: "governance", Title: "Command requires human review",
			Evidence: perm.Reason, Confidence: "high", Source: "permission",
			RuleID: "permission-non-allow", Status: "needs_human_review",
		}}}, runRecord, decision, nil
	}

	start := time.Now()
	// 通过 skill_run 进入 runtime。
	raw, err := a.runTool.Call(ctx, runArgs)
	runRecord.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		runRecord.Status = "error"
		runRecord.StderrDigest = digestString(err.Error())
		return review.Result{}, runRecord, decision, err
	}
	out, err := decodeSkillRunOutput(raw)
	if err != nil {
		runRecord.Status = "error"
		runRecord.StderrDigest = digestString(err.Error())
		return review.Result{}, runRecord, decision, err
	}
	runRecord.Status = "ok"
	// 以 skill_run 返回值记录状态。
	if out.TimedOut {
		runRecord.Status = "timed_out"
	} else if out.ExitCode != 0 {
		runRecord.Status = "failed"
	}
	runRecord.ExitCode = out.ExitCode
	runRecord.StdoutDigest = digestString(out.Stdout)
	runRecord.StderrDigest = digestString(out.Stderr)
	if runRecord.DurationMS == 0 {
		runRecord.DurationMS = out.DurationMS
	}

	// stdout 承载结构化 findings。
	result, err := parseSkillFindings(out.Stdout)
	return result, runRecord, decision, err
}

// runGoSandboxChecks 执行 Go 项目检查。
func (a *Agent) runGoSandboxChecks(ctx context.Context, taskID string, repoPath string) ([]storage.DecisionRecord, []storage.SandboxRunRecord) {
	// staticcheck 需要显式开启。
	commands := reviewgate.AllowedReviewCommands(a.cfg.EnableStaticcheck)
	decisions := make([]storage.DecisionRecord, 0, len(commands))
	runs := make([]storage.SandboxRunRecord, 0, len(commands))
	for _, command := range commands {
		decision, run := a.runGoSandboxCommand(ctx, taskID, repoPath, command)
		decisions = append(decisions, decision)
		runs = append(runs, run)
	}
	return decisions, runs
}

// runGoSandboxCommand 执行单个 Go 检查命令。
func (a *Agent) runGoSandboxCommand(ctx context.Context, taskID string, repoPath string, command string) (storage.DecisionRecord, storage.SandboxRunRecord) {
	execCommand := goSandboxExecCommand(a.cfg.Runtime, command)
	workspaceArgs, _ := reviewexec.WorkspaceArgs(execCommand, a.cfg.Timeout, goSandboxEnv(a.cfg.Runtime))
	legacyArgs, _ := json.Marshal(map[string]any{
		"code_blocks": []map[string]string{{
			"language": "bash",
			"code":     goSandboxCode(a.cfg.Runtime, repoPath, execCommand),
		}},
		"execution_id": taskID + "-" + strings.ReplaceAll(command, " ", "-"),
	})
	// 执行前必须经过 PermissionPolicy。
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
	// 规整异常决策。
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
		// 非 allow 不进入执行器。
		run.Status = string(perm.Action)
		return decision, run
	}

	start := time.Now()
	// 优先通过官方 workspaceexec 在工作区内运行。
	raw, err := a.runWorkspaceGoChecks(ctx, repoPath, execCommand)
	if err != nil {
		// 保留旧路径兜底，避免本地 fallback 的工作区条件不满足时退化。
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
		// 非零退出必须作为失败记录。
		run.Status = "failed"
		if run.ExitCode == 0 {
			run.ExitCode = 1
		}
		return decision, run
	}
	run.Status = "ok"
	return decision, run
}

func (a *Agent) runWorkspaceGoChecks(ctx context.Context, repoPath string, command string) (any, error) {
	return reviewexec.RunWorkspaceCommand(ctx, a.exec, repoPath, command, a.cfg.Timeout, goSandboxEnv(a.cfg.Runtime))
}

// goSandboxExecCommand 返回 runtime 内实际执行命令。
func goSandboxExecCommand(runtime string, command string) string {
	return reviewexec.SandboxExecCommand(runtime, command)
}

// goSandboxEnv 固定 Go 检查的最小环境。
func goSandboxEnv(runtime string) map[string]string {
	return reviewexec.SandboxEnv(runtime)
}

// decodeSkillRunOutput 将 trpc-agent-go 的 skill_run 返回值转为本地结构。
func decodeSkillRunOutput(raw any) (skillRunOutput, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return skillRunOutput{}, err
	}
	var out skillRunOutput
	if err := json.Unmarshal(b, &out); err != nil {
		return skillRunOutput{}, err
	}
	return out, nil
}

// skillRunOutput 是 skill_run 执行摘要。
type skillRunOutput struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMS int64  `json:"duration_ms"`
}

// parseSkillFindings 解析 Skill 脚本 stdout 中的结构化 findings。
func parseSkillFindings(stdout string) (review.Result, error) {
	// stdout 契约为 findings/warnings JSON。
	var payload struct {
		Findings []review.Finding `json:"findings"`
		Warnings []review.Finding `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		return review.Result{}, err
	}
	// Agent 层兜底脱敏。
	for i := range payload.Findings {
		payload.Findings[i] = sanitizeFinding(payload.Findings[i])
	}
	for i := range payload.Warnings {
		payload.Warnings[i] = sanitizeFinding(payload.Warnings[i])
	}
	// 分别去重，避免混淆置信度。
	return review.Result{
		Findings: review.DedupeFindings(payload.Findings),
		Warnings: review.DedupeFindings(payload.Warnings),
	}, nil
}

// sanitizeFinding 在 finding 进入报告和数据库前做兜底脱敏。
func sanitizeFinding(f review.Finding) review.Finding {
	// evidence 入库前必须脱敏。
	f.Evidence = review.RedactSecrets(f.Evidence)
	if f.Status == "" {
		f.Status = "finding"
	}
	return f
}

// newTaskID 生成任务 ID。
func newTaskID(diff []byte) string {
	return "task-" + digestBytes(diff)[:12] + fmt.Sprintf("-%d", time.Now().UnixNano())
}

// digestBytes 返回 SHA-256 摘要。
func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// digestString 返回字符串摘要。
func digestString(data string) string {
	return digestBytes([]byte(data))
}

// sandboxCommandOutput 提取沙箱命令输出。
func sandboxCommandOutput(raw any) commandOutput {
	b, err := json.Marshal(raw)
	if err != nil {
		return commandOutput{}
	}
	var out struct {
		Status   string `json:"status"`
		Output   string `json:"output"`
		ExitCode *int   `json:"exit_code"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return commandOutput{}
	}
	return commandOutput{
		Status:   out.Status,
		Text:     out.Output,
		ExitCode: out.ExitCode,
	}
}

// sandboxRunOutput 保留受限、脱敏后的失败线索。
func sandboxRunOutput(text string, limit int) string {
	text = review.RedactSecrets(text)
	if limit > 0 && len(text) > limit {
		return text[:limit]
	}
	return text
}

type commandOutput struct {
	Status   string
	Text     string
	ExitCode *int
}

// shellQuote 对本地路径做 POSIX 单引号转义。
func shellQuote(value string) string {
	return reviewexec.ShellQuote(value)
}

// sandboxRepoPathForRuntime 返回 runtime 内 repo 路径。
func sandboxRepoPathForRuntime(runtime string, hostRepoPath string) string {
	return reviewexec.SandboxRepoPathForRuntime(runtime, hostRepoPath)
}

// goSandboxCode 构造 Go 检查命令。
func goSandboxCode(runtime string, hostRepoPath string, command string) string {
	return reviewexec.SandboxCode(runtime, hostRepoPath, command)
}
