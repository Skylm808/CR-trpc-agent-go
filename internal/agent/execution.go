package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage"
	dockercontainer "github.com/docker/docker/api/types/container"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	containerexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/container"
	localexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	workspaceexec "trpc.group/trpc-go/trpc-agent-go/tool/workspaceexec"
)

// newExecutor 创建 trpc-agent-go 执行器。
func newExecutor(cfg Config) (codeexecutor.CodeExecutor, error) {
	switch cfg.Runtime {
	case RuntimeLocalFallback:
		// 本地 fallback 只用于测试和开发。
		return localexec.New(
			localexec.WithTimeout(cfg.Timeout),
			localexec.WithWorkDir(filepath.Join(os.TempDir(), "cr-agent-localexec")),
		), nil
	case RuntimeContainer:
		// 默认使用官方容器执行器。
		opts := []containerexec.Option{
			containerexec.WithContainerConfig(dockercontainer.Config{
				Image:      defaultContainerImage,
				WorkingDir: "/",
				Cmd:        []string{"tail", "-f", "/dev/null"},
				Tty:        true,
				OpenStdin:  true,
			}),
		}
		if strings.TrimSpace(cfg.ContainerRepoHostPath) != "" {
			// repo 只读挂载到固定路径。
			opts = append(opts, containerexec.WithBindMount(cfg.ContainerRepoHostPath, containerRepoMountPath, "ro"))
		}
		exec, err := containerexec.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("create container executor: %w", err)
		}
		return exec, nil
	default:
		return nil, fmt.Errorf("unsupported runtime %q", cfg.Runtime)
	}
}

// defaultPermissionPolicy 返回固定命令白名单。
func defaultPermissionPolicy() tool.PermissionPolicy {
	return tool.PermissionPolicyFunc(func(ctx context.Context, req *tool.PermissionRequest) (tool.PermissionDecision, error) {
		_ = ctx
		if req == nil {
			return tool.DenyPermission("missing permission request"), nil
		}
		args := string(req.Arguments)
		// 只允许 code-review 固定脚本。
		if req.ToolName == "skill_run" && strings.Contains(args, defaultSkillCommand) {
			return tool.AllowPermission(), nil
		}
		if req.ToolName == "workspace_exec" &&
			(strings.Contains(args, "go test ./...") ||
				strings.Contains(args, "go vet ./...") ||
				strings.Contains(args, "staticcheck ./...")) {
			return tool.AllowPermission(), nil
		}
		if req.ToolName == "execute_code" &&
			(strings.Contains(args, "go test ./...") ||
				strings.Contains(args, "go vet ./...") ||
				strings.Contains(args, "staticcheck ./...")) {
			return tool.AllowPermission(), nil
		}
		if strings.Contains(args, "go test ./...") ||
			strings.Contains(args, "go vet ./...") ||
			strings.Contains(args, "staticcheck ./...") {
			return tool.AllowPermission(), nil
		}
		return tool.AskPermission("unrecognized tool command requires human review"), nil
	})
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
		EnvWhitelist:     "PATH,HOME,TMPDIR",
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
		EnvWhitelist:     "PATH,HOME,TMPDIR",
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
	commands := []string{"go test ./...", "go vet ./..."}
	if a.cfg.EnableStaticcheck {
		commands = append(commands, "staticcheck ./...")
	}
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
	workspaceArgs, _ := json.Marshal(map[string]any{
		"command": command,
		"cwd":     "work/repo",
		"timeout": int(a.cfg.Timeout.Seconds()),
		"env": map[string]string{
			"GOCACHE": goSandboxCacheDir,
		},
	})
	legacyArgs, _ := json.Marshal(map[string]any{
		"code_blocks": []map[string]string{{
			"language": "bash",
			"code":     goSandboxCode(a.cfg.Runtime, repoPath, command),
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
		EnvWhitelist:     "PATH,HOME,TMPDIR,GOCACHE",
		At:               time.Now(),
	}
	if perm.Action != tool.PermissionActionAllow {
		// 非 allow 不进入执行器。
		run.Status = string(perm.Action)
		return decision, run
	}

	start := time.Now()
	// 优先通过官方 workspaceexec 在工作区内运行。
	raw, err := a.runWorkspaceGoChecks(ctx, repoPath, command)
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
	if a.exec == nil {
		return nil, fmt.Errorf("workspace exec is not configured")
	}
	exec := workspaceexec.NewExecTool(a.exec,
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
	args, err := json.Marshal(map[string]any{
		"command": command,
		"cwd":     "work/repo",
		"timeout": int(a.cfg.Timeout.Seconds()),
		"env": map[string]string{
			"GOCACHE": goSandboxCacheDir,
		},
	})
	if err != nil {
		return nil, err
	}
	return exec.Call(ctx, args)
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

type commandOutput struct {
	Status   string
	Text     string
	ExitCode *int
}

// shellQuote 对本地路径做 POSIX 单引号转义。
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// sandboxRepoPathForRuntime 返回 runtime 内 repo 路径。
func sandboxRepoPathForRuntime(runtime string, hostRepoPath string) string {
	if runtime == RuntimeContainer {
		return containerRepoMountPath
	}
	return hostRepoPath
}

// goSandboxCode 构造 Go 检查命令。
func goSandboxCode(runtime string, hostRepoPath string, command string) string {
	return "cd " + shellQuote(sandboxRepoPathForRuntime(runtime, hostRepoPath)) +
		" && GOCACHE=" + shellQuote(goSandboxCacheDir) + " " + command
}
