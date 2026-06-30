// Package agent 编排基于 trpc-agent-go Skill、权限策略、执行器和存储的自动代码评审链路。
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/report"
	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"
	dockercontainer "github.com/docker/docker/api/types/container"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	containerexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/container"
	localexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
	skillrepo "trpc.group/trpc-go/trpc-agent-go/skill"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	toolcodeexec "trpc.group/trpc-go/trpc-agent-go/tool/codeexec"
	toolskill "trpc.group/trpc-go/trpc-agent-go/tool/skill"
)

const (
	// RuntimeContainer 是生产默认沙箱运行时。当前最小版本保留该默认值，
	// Docker 连接失败时由调用方显式切换到 RuntimeLocalFallback 做开发测试。
	RuntimeContainer = "container"
	// RuntimeLocalFallback 仅用于本地开发和自动化测试，不应作为生产默认值。
	RuntimeLocalFallback = "local-fallback"

	// ModeRuleOnly 表示不依赖真实模型，只执行确定性 Skill 规则脚本。
	ModeRuleOnly = "rule-only"
	// ModeDryRun 表示只演练治理和持久化链路。
	ModeDryRun = "dry-run"
	// ModeSandbox 表示执行确定性规则并保留沙箱审计摘要。
	ModeSandbox = "sandbox"
	// ModeFakeModel 表示不使用真实模型 API，复用确定性 Skill 链路。
	ModeFakeModel = "fake-model"
)

const (
	defaultSkillName        = "code-review"
	defaultSkillCommand     = "scripts/check.sh"
	defaultOutputLimitBytes = 64 * 1024
	defaultTimeout          = 30 * time.Second
	containerRepoMountPath  = "/workspace/repo"
	defaultContainerImage   = "golang:1.25-bookworm"
)

// Config 描述 Agent 运行一轮审查所需的稳定依赖和安全边界。
type Config struct {
	// SkillsRoot 指向 skills 根目录，Agent 会从这里加载 code-review Skill。
	SkillsRoot string
	// Runtime 决定代码执行器类型，生产默认 container，本地测试显式使用 local-fallback。
	Runtime string
	// SQLitePath 非空时启用 SQLite 审计落库，空值表示只生成报告。
	SQLitePath string
	// OutputDir 是 review_report.json 和 review_report.md 的输出目录。
	OutputDir string
	// FixturesRoot 是 --fixture 模式读取 diff 样本的根目录。
	FixturesRoot string
	// ContainerRepoHostPath 是 container runtime 挂载宿主机 repo 的只读路径。
	ContainerRepoHostPath string
	// Timeout 限制 Skill 脚本和沙箱命令的最长执行时间。
	Timeout time.Duration
	// OutputLimitBytes 限制工具 stdout/stderr 进入报告和数据库前的最大字节数。
	OutputLimitBytes int
	// EnableStaticcheck 控制 sandbox 模式是否额外执行 staticcheck。
	EnableStaticcheck bool
}

// Request 描述一次审查输入；DiffFile、RepoPath、Fixture 至少需要提供一个。
type Request struct {
	// DiffFile 指向外部传入的 unified diff 或 PR patch 文件。
	DiffFile string
	// RepoPath 指向本地 Git 工作区；Agent 会在该目录执行 git diff。
	RepoPath string
	// Fixture 指向 FixturesRoot 下的内置测试 diff 文件名。
	Fixture string
	// Mode 决定执行深度，例如 rule-only、dry-run 或 sandbox。
	Mode string
}

// Agent 持有 trpc-agent-go 工具和本项目持久化实现。
type Agent struct {
	// cfg 保存 CLI 转换后的运行参数和安全边界。
	cfg Config
	// loadTool 是官方 skill_load 工具，用于加载 code-review Skill。
	loadTool tool.CallableTool
	// runTool 是官方 skill_run 工具，用于执行 Skill 内的固定脚本。
	runTool tool.CallableTool
	// checkTool 是官方 codeexec 工具，用于 sandbox 模式下执行 Go 检查命令。
	checkTool tool.CallableTool
	// policy 是官方 PermissionPolicy，所有高风险工具调用都先经过它。
	policy tool.PermissionPolicy
	// store 是本项目的存储接口，当前默认实现是 SQLite。
	store storage.Store
}

// New 创建一个框架优先的 CR Agent。
func New(cfg Config) (*Agent, error) {
	cfg = normalizeConfig(cfg)
	if cfg.SkillsRoot == "" {
		return nil, errors.New("skills root is required")
	}

	// 先建立官方 Skill 仓库，让后续 skill_load 和 skill_run 共用同一个根目录。
	repo, err := skillrepo.NewFSRepository(cfg.SkillsRoot)
	if err != nil {
		return nil, fmt.Errorf("load skill repository: %w", err)
	}
	// 执行器在这里创建，确保 skill_run 和 codeexec 使用相同 runtime 边界。
	exec, err := newExecutor(cfg)
	if err != nil {
		return nil, err
	}

	var store storage.Store
	if cfg.SQLitePath != "" {
		// SQLite 是默认持久化实现，但 Agent 只依赖 storage.Store 接口。
		store, err = sqlite.Open(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite store: %w", err)
		}
	}

	// skill_run 的 allowlist 会禁用 shell 组合语法，只允许执行 Skill 内脚本。
	runTool := toolskill.NewRunTool(
		repo,
		exec,
		toolskill.WithAllowedCommands(defaultSkillCommand),
		toolskill.WithRunOutputLimits(toolskill.RunOutputLimits{
			StdoutStderrBytes:  cfg.OutputLimitBytes,
			PrimaryOutputBytes: cfg.OutputLimitBytes,
		}),
	)

	return &Agent{
		cfg:       cfg,
		loadTool:  toolskill.NewLoadTool(repo),
		runTool:   runTool,
		checkTool: toolcodeexec.NewTool(exec, toolcodeexec.WithName("execute_code"), toolcodeexec.WithLanguages("bash")),
		policy:    defaultPermissionPolicy(),
		store:     store,
	}, nil
}

// Run 执行一次完整审查：采集输入、加载 Skill、权限判定、执行脚本、生成报告并落库。
func (a *Agent) Run(ctx context.Context, req Request) (review.Result, error) {
	start := time.Now()
	// 输入统一收敛成 diff 字节流，后续 Skill、报告和数据库都只依赖这一份输入。
	diff, inputRef, err := readInput(a.cfg, req)
	if err != nil {
		return review.Result{}, err
	}
	// taskID 用 diff 摘要和时间戳生成，既方便追踪又避免不同运行互相覆盖。
	taskID := newTaskID(diff)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = ModeRuleOnly
	}

	if a.store != nil {
		// 先写 running 任务，保证即使后续沙箱失败也能在数据库中看到审查尝试。
		if err := a.store.SaveTask(ctx, storage.Task{
			ID: taskID, InputType: "diff", InputRef: inputRef,
			InputDigest: digestBytes(diff), RepoPath: req.RepoPath,
			Status: "running", Mode: mode, CreatedAt: start,
			StartedAt: start,
		}); err != nil {
			return review.Result{}, err
		}
	}

	toolCallCount := 2
	var result review.Result
	var runRecord storage.SandboxRunRecord
	var decision storage.DecisionRecord
	if mode == ModeDryRun {
		// dry-run 只验证 Skill 可加载和治理链路，不进入真实执行器。
		toolCallCount = 1
		result, runRecord, decision, err = a.runDryRun(ctx, taskID)
	} else {
		// rule-only、fake-model 和 sandbox 都先执行确定性的 code-review Skill。
		result, runRecord, decision, err = a.runSkillChecks(ctx, taskID, diff)
	}
	decisions := []storage.DecisionRecord{decision}
	runs := []storage.SandboxRunRecord{runRecord}
	if mode == ModeSandbox && strings.TrimSpace(req.RepoPath) != "" {
		// sandbox 模式在 Skill 规则之外，再对真实 Go repo 执行 go test/go vet。
		checkDecisions, checkRuns := a.runGoSandboxChecks(ctx, taskID, req.RepoPath)
		decisions = append(decisions, checkDecisions...)
		runs = append(runs, checkRuns...)
		toolCallCount += len(checkRuns)
	}
	if err != nil {
		// Skill 或沙箱失败不直接终止评审，转成需要人工复核的 warning。
		result = resultWithRunError(result, err)
	}
	// 从执行记录反推监控指标，保证报告和数据库看到的是同一套统计口径。
	result.TaskID = taskID
	result.Created = time.Now()
	result.Metrics.TotalDurationMS = time.Since(start).Milliseconds()
	result.Metrics.ToolCallCount = toolCallCount
	result.Metrics.SandboxDurationMS = totalSandboxDuration(runs)
	result.Metrics.FindingCount = len(result.Findings)
	result.Metrics.RedactionCount = redactionCount(result.Findings, result.Warnings)
	result.Metrics.SeverityCounts = severityCounts(result.Findings, result.Warnings)
	if result.Metrics.ExceptionCounts == nil {
		result.Metrics.ExceptionCounts = map[string]int{}
	}
	for _, run := range runs {
		if run.Status == "failed" || run.Status == "error" || run.Status == "timed_out" {
			incrementException(result.Metrics.ExceptionCounts, "sandbox_failed")
		}
	}
	if decision.Action != string(tool.PermissionActionAllow) {
		// 当前 MVP 只有主 Skill 命令计入 permission_blocks。
		result.Metrics.PermissionBlocks = 1
	}
	// 报告层需要治理、沙箱、artifact 摘要；这些字段也会跟随结果落库。
	result.HumanReviewItems = humanReviewItems(result.Warnings)
	result.GovernanceSummary = governanceSummary(decisions, result.Metrics.PermissionBlocks)
	result.SandboxSummary = sandboxSummary(runs)
	result.Artifacts = reportArtifacts()
	if result.Summary == "" {
		result.Summary = fmt.Sprintf("%d findings, %d warnings", len(result.Findings), len(result.Warnings))
	}

	// 报告先生成并写入文件，再把同一份内容写入 SQLite，避免报告和数据库不一致。
	jsonReport, jsonErr := report.BuildJSON(result)
	if jsonErr != nil {
		return review.Result{}, jsonErr
	}
	md := report.BuildMarkdown(result)
	if err := writeReports(a.cfg.OutputDir, jsonReport, []byte(md)); err != nil {
		return review.Result{}, err
	}
	if a.store != nil {
		// persist 写入可回放审计数据：权限、沙箱、finding、指标、artifact 和报告。
		if err := a.persist(ctx, taskID, result, decisions, runs, jsonReport, []byte(md)); err != nil {
			return review.Result{}, err
		}
		// 最后把任务状态从 running 更新为 done。
		if err := a.store.SaveTask(ctx, storage.Task{
			ID: taskID, InputType: "diff", InputRef: inputRef,
			InputDigest: digestBytes(diff), RepoPath: req.RepoPath,
			Status: "done", Mode: mode, CreatedAt: start,
			StartedAt: start, FinishedAt: time.Now(),
		}); err != nil {
			return review.Result{}, err
		}
	}
	return result, nil
}

// runDryRun 只加载 Skill 并记录跳过执行的治理/沙箱摘要。
func (a *Agent) runDryRun(ctx context.Context, taskID string) (review.Result, storage.SandboxRunRecord, storage.DecisionRecord, error) {
	// dry-run 仍然调用 skill_load，用来验证 Skill 目录和 SKILL.md 可被框架识别。
	loadArgs := []byte(`{"skill":"code-review"}`)
	if _, err := a.loadTool.Call(ctx, loadArgs); err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}
	now := time.Now()
	// dry-run 不执行脚本，但仍记录治理和沙箱摘要，方便验收落库链路。
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

// Close 释放 Agent 持有的存储连接。
func (a *Agent) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

// normalizeConfig 填充运行时、安全限制和输出目录的默认值。
func normalizeConfig(cfg Config) Config {
	if cfg.Runtime == "" {
		cfg.Runtime = RuntimeContainer
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.OutputLimitBytes <= 0 {
		cfg.OutputLimitBytes = defaultOutputLimitBytes
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "."
	}
	return cfg
}

// newExecutor 根据配置创建官方 trpc-agent-go CodeExecutor。
func newExecutor(cfg Config) (codeexecutor.CodeExecutor, error) {
	switch cfg.Runtime {
	case RuntimeLocalFallback:
		// 本地 fallback 使用隔离临时 workspace，只服务测试和开发。
		return localexec.New(
			localexec.WithTimeout(cfg.Timeout),
			localexec.WithWorkDir(filepath.Join(os.TempDir(), "cr-agent-localexec")),
		), nil
	case RuntimeContainer:
		// 默认生产路径走官方 codeexecutor/container。测试不依赖 Docker，
		// 因此必须显式选择 RuntimeLocalFallback。
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
			// 容器运行时无法访问主机绝对路径，显式把 repo 只读挂到固定路径。
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

// defaultPermissionPolicy 返回第一版固定命令白名单的权限策略。
func defaultPermissionPolicy() tool.PermissionPolicy {
	return tool.PermissionPolicyFunc(func(ctx context.Context, req *tool.PermissionRequest) (tool.PermissionDecision, error) {
		_ = ctx
		if req == nil {
			return tool.DenyPermission("missing permission request"), nil
		}
		// 第一版只允许 code-review Skill 的固定脚本进入 executor。
		if req.ToolName == "skill_run" && strings.Contains(string(req.Arguments), defaultSkillCommand) {
			return tool.AllowPermission(), nil
		}
		if req.ToolName == "execute_code" &&
			(strings.Contains(string(req.Arguments), "go test ./...") ||
				strings.Contains(string(req.Arguments), "go vet ./...") ||
				strings.Contains(string(req.Arguments), "staticcheck ./...")) {
			return tool.AllowPermission(), nil
		}
		return tool.AskPermission("unrecognized tool command requires human review"), nil
	})
}

// runGoSandboxChecks 在 sandbox 模式下执行 Go 项目的最小静态/测试检查。
func (a *Agent) runGoSandboxChecks(ctx context.Context, taskID string, repoPath string) ([]storage.DecisionRecord, []storage.SandboxRunRecord) {
	// 基础检查固定为 go test 和 go vet；staticcheck 因依赖额外工具，保持显式开关。
	commands := []string{"go test ./...", "go vet ./..."}
	if a.cfg.EnableStaticcheck {
		// staticcheck 是可选检查，只有显式开启时才进入权限和沙箱链路。
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

// runGoSandboxCommand 对单个 Go 检查命令做权限检查并通过 codeexec 执行。
func (a *Agent) runGoSandboxCommand(ctx context.Context, taskID string, repoPath string, command string) (storage.DecisionRecord, storage.SandboxRunRecord) {
	// codeexec 接收代码块参数，这里只允许 bash 语言并填入受控的 Go 检查命令。
	args, _ := json.Marshal(map[string]any{
		"code_blocks": []map[string]string{{
			"language": "bash",
			"code":     goSandboxCode(a.cfg.Runtime, repoPath, command),
		}},
		"execution_id": taskID + "-" + strings.ReplaceAll(command, " ", "-"),
	})
	// 高风险命令必须先构造 PermissionRequest，再交给 PermissionPolicy 判断。
	permReq := &tool.PermissionRequest{
		Tool:        a.checkTool,
		ToolName:    a.checkTool.Declaration().Name,
		Declaration: a.checkTool.Declaration(),
		Arguments:   args,
	}
	perm, err := a.policy.CheckToolPermission(ctx, permReq)
	if err != nil {
		perm = tool.DenyPermission(err.Error())
	}
	// Normalize 能把空 action 等异常决策规整为框架定义的安全结果。
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
		EnvWhitelist:     "PATH,HOME,TMPDIR",
		At:               time.Now(),
	}
	if perm.Action != tool.PermissionActionAllow {
		// deny/ask/needs_human_review 不能进入执行器，只记录决策和跳过状态。
		run.Status = string(perm.Action)
		return decision, run
	}

	start := time.Now()
	// 只有 allow 的命令才进入 codeexec；执行耗时和输出摘要都会写入审计记录。
	raw, err := a.checkTool.Call(ctx, args)
	run.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		run.Status = "error"
		run.StderrDigest = digestString(err.Error())
		return decision, run
	}
	output := codeExecOutput(raw)
	run.StdoutDigest = digestString(output)
	if strings.Contains(output, "Error executing code block") {
		// 当前 codeexec 返回值没有统一 exit code 字段，先用标准错误文本兜底标记失败。
		run.Status = "failed"
		run.ExitCode = 1
		return decision, run
	}
	run.Status = "ok"
	return decision, run
}

// runSkillChecks 通过 skill_load 与 skill_run 执行 code-review Skill。
func (a *Agent) runSkillChecks(ctx context.Context, taskID string, diff []byte) (review.Result, storage.SandboxRunRecord, storage.DecisionRecord, error) {
	// 第一步显式加载 Skill，保证后续执行的脚本来自受控 Skill 仓库。
	loadArgs := []byte(`{"skill":"code-review"}`)
	if _, err := a.loadTool.Call(ctx, loadArgs); err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}

	// diff 通过 stdin 传给 Skill 脚本，脚本 stdout 必须输出结构化 JSON。
	runArgs, err := json.Marshal(map[string]any{
		"skill":   defaultSkillName,
		"command": defaultSkillCommand,
		"stdin":   string(diff),
		"timeout": int(a.cfg.Timeout.Seconds()),
	})
	if err != nil {
		return review.Result{}, storage.SandboxRunRecord{}, storage.DecisionRecord{}, err
	}

	// skill_run 同样需要 PermissionPolicy 放行，避免任意 Skill 命令被直接执行。
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
		// 非 allow 的治理结果降级为人工复核项，报告仍然可以正常生成。
		runRecord.Status = string(perm.Action)
		return review.Result{Warnings: []review.Finding{{
			Severity: "low", Category: "governance", Title: "Command requires human review",
			Evidence: perm.Reason, Confidence: "high", Source: "permission",
			RuleID: "permission-non-allow", Status: "needs_human_review",
		}}}, runRecord, decision, nil
	}

	start := time.Now()
	// 通过官方 skill_run 进入 workspace runtime，而不是在 Agent 内直接执行脚本。
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
	// skill_run 返回的超时和退出码是沙箱运行结果的事实来源。
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

	// 最后只解析 stdout 中的 JSON findings，stderr 只作为审计摘要保存。
	result, err := parseSkillFindings(out.Stdout)
	return result, runRecord, decision, err
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

// skillRunOutput 是 skill_run 返回值里本项目关心的执行摘要字段。
type skillRunOutput struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMS int64  `json:"duration_ms"`
}

// parseSkillFindings 解析 Skill 脚本 stdout 中的结构化 findings。
func parseSkillFindings(stdout string) (review.Result, error) {
	// Skill stdout 的契约是 {"findings":[],"warnings":[]}，后续报告直接消费该结构。
	var payload struct {
		Findings []review.Finding `json:"findings"`
		Warnings []review.Finding `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		return review.Result{}, err
	}
	// 即使脚本已经脱敏，Agent 层仍然做一次兜底，保护报告和数据库。
	for i := range payload.Findings {
		payload.Findings[i] = sanitizeFinding(payload.Findings[i])
	}
	for i := range payload.Warnings {
		payload.Warnings[i] = sanitizeFinding(payload.Warnings[i])
	}
	// findings 与 warnings 分别去重，避免同一文件同一行同一规则重复报。
	return review.Result{
		Findings: review.DedupeFindings(payload.Findings),
		Warnings: review.DedupeFindings(payload.Warnings),
	}, nil
}

// sanitizeFinding 在 finding 进入报告和数据库前做兜底脱敏。
func sanitizeFinding(f review.Finding) review.Finding {
	// 所有进入报告和数据库的证据先脱敏，避免脚本输出泄漏 secret。
	f.Evidence = review.RedactSecrets(f.Evidence)
	if f.Status == "" {
		f.Status = "finding"
	}
	return f
}

// persist 保存一次审查的治理、沙箱、finding、指标和报告数据。
func (a *Agent) persist(ctx context.Context, taskID string, result review.Result, decisions []storage.DecisionRecord, runs []storage.SandboxRunRecord, jsonReport, markdownReport []byte) error {
	// 权限决策先落库，后续排查可以知道命令是 allow、ask 还是 deny。
	for _, decision := range decisions {
		if decision.Command == "" && decision.Action == "" {
			continue
		}
		if err := a.store.SaveDecision(ctx, decision); err != nil {
			return err
		}
	}
	if result.Metrics.RedactionCount > 0 {
		// 只要发生脱敏，就记录一条 filter decision，证明敏感信息经过治理处理。
		if err := a.store.SaveFilterDecision(ctx, storage.FilterDecisionRecord{
			TaskID: taskID,
			Target: "finding.evidence",
			Action: "redact",
			Reason: "secret pattern",
			At:     time.Now(),
		}); err != nil {
			return err
		}
	}
	// 沙箱运行记录只保存摘要和 digest，不把完整 stdout/stderr 直接写进数据库。
	for _, run := range runs {
		if run.Command == "" && run.Status == "" {
			continue
		}
		if err := a.store.SaveSandboxRun(ctx, run); err != nil {
			return err
		}
	}
	// findings、warnings 和人工复核项统一落到 finding 表，通过 status 区分。
	for _, finding := range persistedReviewItems(result) {
		if err := a.store.SaveFinding(ctx, taskID, finding); err != nil {
			return err
		}
	}
	// metrics 保存聚合指标，方便之后按 task id 回放或做批量评测。
	if err := a.store.SaveMetrics(ctx, storage.MetricsRecord{
		TaskID: taskID, TotalDurationMS: result.Metrics.TotalDurationMS,
		SandboxDurationMS:    result.Metrics.SandboxDurationMS,
		ToolCallCount:        result.Metrics.ToolCallCount,
		PermissionBlockCount: result.Metrics.PermissionBlocks,
		FindingCount:         result.Metrics.FindingCount,
		SeverityCountsJSON:   string(review.MustJSON(result.Metrics.SeverityCounts)),
		ExceptionCountsJSON:  string(review.MustJSON(result.Metrics.ExceptionCounts)),
		RedactionCount:       result.Metrics.RedactionCount,
		At:                   time.Now(),
	}); err != nil {
		return err
	}
	// artifact 记录报告文件的名称、路径和摘要，内容本身由 reports 表保存。
	for _, artifact := range result.Artifacts {
		digest := artifact.Digest
		if artifact.Name == "review_report.json" {
			digest = digestBytes(jsonReport)
		}
		if artifact.Name == "review_report.md" {
			digest = digestBytes(markdownReport)
		}
		if err := a.store.SaveArtifact(ctx, storage.ArtifactRecord{
			TaskID: taskID,
			Name:   artifact.Name,
			Kind:   artifact.Kind,
			Path:   artifact.Path,
			Digest: digest,
			At:     time.Now(),
		}); err != nil {
			return err
		}
	}
	// 最终报告单独保存，保证命令行输出和数据库查询都能复现同一份结论。
	return a.store.SaveReport(ctx, taskID, jsonReport, markdownReport)
}

// persistedReviewItems 返回需要进入 SQLite 回放链路的结构化审查项。
func persistedReviewItems(result review.Result) []review.Finding {
	// findings 和 warnings 共用同一张表，通过 status 区分高置信问题和低置信复核项。
	items := make([]review.Finding, 0, len(result.Findings)+len(result.Warnings)+len(result.HumanReviewItems))
	items = append(items, result.Findings...)
	items = append(items, result.Warnings...)
	items = append(items, result.HumanReviewItems...)
	return review.DedupeFindings(items)
}

// readInput 读取 diff 文件或从 repo path 生成统一 diff。
func readInput(cfg Config, req Request) ([]byte, string, error) {
	if req.DiffFile != "" {
		b, err := os.ReadFile(req.DiffFile)
		return b, req.DiffFile, err
	}
	if req.Fixture != "" {
		return readFixtureInput(cfg.FixturesRoot, req.Fixture)
	}
	if req.RepoPath != "" {
		b, err := diffFromRepo(req.RepoPath)
		return b, req.RepoPath, err
	}
	return nil, "", errors.New("diff file, repo path, or fixture is required")
}

// readFixtureInput 从受控 fixture 根目录读取 diff 样本。
func readFixtureInput(root string, name string) ([]byte, string, error) {
	if strings.TrimSpace(root) == "" {
		return nil, "", errors.New("fixtures root is required")
	}
	cleanName := filepath.Clean(strings.TrimSpace(name))
	if cleanName == "." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, "..") {
		return nil, "", fmt.Errorf("invalid fixture name %q", name)
	}
	path := filepath.Join(root, cleanName)
	b, err := os.ReadFile(path)
	return b, path, err
}

// diffFromRepo 统一在 Agent 层采集工作区变更，保证 CLI 不绕过审查编排。
func diffFromRepo(repoPath string) ([]byte, error) {
	if repoPath == "" {
		return nil, errors.New("repo path is required")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		// Git 工作区使用统一 diff，后续 hunk/package 解析都基于该输入。
		cmd := exec.Command("git", "-C", repoPath, "diff", "--unified=3")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("git diff: %w: %s", err, string(out))
		}
		return out, nil
	}
	var b strings.Builder
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(repoPath, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		// 普通目录 fixture 被转换为新增文件 diff，便于同一条 Skill 流程处理。
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", entry.Name(), entry.Name())
		fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n", entry.Name())
		fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
		for _, line := range lines {
			if line == "" {
				continue
			}
			fmt.Fprintf(&b, "+%s\n", line)
		}
	}
	return []byte(b.String()), nil
}

// writeReports 将 JSON 与 Markdown 报告写入输出目录。
func writeReports(dir string, jsonReport, markdownReport []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "review_report.json"), jsonReport, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "review_report.md"), markdownReport, 0o644)
}

// newTaskID 基于 diff 摘要和时间戳生成可查询任务 ID。
func newTaskID(diff []byte) string {
	return "task-" + digestBytes(diff)[:12] + fmt.Sprintf("-%d", time.Now().UnixNano())
}

// digestBytes 返回输入内容的 SHA-256 十六进制摘要。
func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// digestString 返回字符串内容的 SHA-256 十六进制摘要。
func digestString(data string) string {
	return digestBytes([]byte(data))
}

// codeExecOutput 从官方 codeexec tool 返回值中提取 stdout 文本。
func codeExecOutput(raw any) string {
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	var out struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return ""
	}
	return out.Output
}

// shellQuote 对本地路径做 POSIX 单引号转义。
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// sandboxRepoPathForRuntime 返回 sandbox 命令在目标 runtime 中应访问的 repo 路径。
func sandboxRepoPathForRuntime(runtime string, hostRepoPath string) string {
	if runtime == RuntimeContainer {
		return containerRepoMountPath
	}
	return hostRepoPath
}

// goSandboxCode 构造 Go 检查命令，避免 container runtime 泄漏主机路径。
func goSandboxCode(runtime string, hostRepoPath string, command string) string {
	return "cd " + shellQuote(sandboxRepoPathForRuntime(runtime, hostRepoPath)) + " && " + command
}

// severityCounts 汇总 findings 与 warnings 的严重级别分布。
func severityCounts(findings, warnings []review.Finding) map[string]int {
	out := map[string]int{}
	for _, f := range findings {
		out[f.Severity]++
	}
	for _, f := range warnings {
		out[f.Severity]++
	}
	return out
}

// redactionCount 统计报告证据中出现的脱敏占位符数量。
func redactionCount(findings, warnings []review.Finding) int {
	total := 0
	for _, finding := range append(findings, warnings...) {
		if strings.Contains(finding.Evidence, "[REDACTED]") {
			total++
		}
	}
	return total
}

// humanReviewItems 从 warnings 中挑出需要人工处理的项目。
func humanReviewItems(warnings []review.Finding) []review.Finding {
	var out []review.Finding
	for _, warning := range warnings {
		if warning.Status == "needs_human_review" || warning.Status == "ask" {
			out = append(out, warning)
		}
	}
	return review.DedupeFindings(out)
}

// governanceSummary 将权限决策转换为报告摘要。
func governanceSummary(decisions []storage.DecisionRecord, blocks int) review.GovernanceSummary {
	out := review.GovernanceSummary{PermissionBlocks: blocks}
	for _, decision := range decisions {
		if decision.Command == "" && decision.Action == "" {
			continue
		}
		out.PermissionDecisions = append(out.PermissionDecisions, review.PermissionDecisionSummary{
			Command: decision.Command,
			Action:  decision.Action,
			Reason:  decision.Reason,
		})
	}
	return out
}

// sandboxSummary 将沙箱运行记录转换为报告摘要。
func sandboxSummary(runs []storage.SandboxRunRecord) review.SandboxSummary {
	out := review.SandboxSummary{}
	for _, run := range runs {
		if run.Command == "" {
			continue
		}
		out.Runs = append(out.Runs, review.SandboxRunSummary{
			Command:          run.Command,
			Runtime:          run.Runtime,
			Status:           run.Status,
			TimeoutMS:        run.TimeoutMS,
			OutputLimitBytes: run.OutputLimitBytes,
			EnvWhitelist:     run.EnvWhitelist,
			ExitCode:         run.ExitCode,
			StdoutDigest:     run.StdoutDigest,
			StderrDigest:     run.StderrDigest,
			DurationMS:       run.DurationMS,
		})
	}
	return out
}

// reportArtifacts 声明本地报告文件产物，后续可替换为 artifact service 引用。
func reportArtifacts() []review.ArtifactSummary {
	return []review.ArtifactSummary{
		{Name: "review_report.json", Kind: "report", Path: "review_report.json"},
		{Name: "review_report.md", Kind: "report", Path: "review_report.md"},
	}
}

// totalSandboxDuration 汇总多条沙箱运行耗时。
func totalSandboxDuration(runs []storage.SandboxRunRecord) int64 {
	var total int64
	for _, run := range runs {
		total += run.DurationMS
	}
	return total
}

// resultWithRunError 把 skill_run 错误降级为人工复核 warning，保证评审流程继续产出报告。
func resultWithRunError(result review.Result, err error) review.Result {
	if result.Metrics.ExceptionCounts == nil {
		result.Metrics.ExceptionCounts = map[string]int{}
	}
	incrementException(result.Metrics.ExceptionCounts, "skill_run")
	result.Warnings = append(result.Warnings, review.Finding{
		Severity:       "low",
		Category:       "sandbox",
		Title:          "Sandbox command failed",
		Evidence:       review.RedactSecrets(err.Error()),
		Recommendation: "Inspect sandbox stderr digest and rerun the command in an isolated workspace.",
		Confidence:     "high",
		Source:         "sandbox",
		RuleID:         "sandbox-command-failed",
		Status:         "needs_human_review",
	})
	return result
}

// incrementException 增加指定异常类型计数。
func incrementException(counts map[string]int, key string) {
	if counts == nil {
		return
	}
	counts[key]++
}
