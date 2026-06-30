// Package agent 编排基于 trpc-agent-go 的代码评审链路。
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
	// RuntimeContainer 是默认沙箱运行时。
	RuntimeContainer = "container"
	// RuntimeLocalFallback 仅用于本地开发和测试。
	RuntimeLocalFallback = "local-fallback"

	// ModeRuleOnly 只执行确定性规则。
	ModeRuleOnly = "rule-only"
	// ModeDryRun 只演练治理和落库。
	ModeDryRun = "dry-run"
	// ModeSandbox 执行规则和 Go 检查。
	ModeSandbox = "sandbox"
	// ModeFakeModel 复用规则链路，不调用模型。
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

// Config 保存一次审查的依赖和边界。
type Config struct {
	// SkillsRoot 是 Skill 根目录。
	SkillsRoot string
	// Runtime 是执行器类型。
	Runtime string
	// SQLitePath 非空时启用落库。
	SQLitePath string
	// OutputDir 是报告目录。
	OutputDir string
	// FixturesRoot 是样本 diff 根目录。
	FixturesRoot string
	// ContainerRepoHostPath 是容器只读挂载源。
	ContainerRepoHostPath string
	// Timeout 是执行超时。
	Timeout time.Duration
	// OutputLimitBytes 是输出上限。
	OutputLimitBytes int
	// EnableStaticcheck 控制可选 staticcheck。
	EnableStaticcheck bool
}

// Request 描述一次审查输入。
type Request struct {
	// DiffFile 是外部 diff 文件。
	DiffFile string
	// RepoPath 是本地 Git 工作区。
	RepoPath string
	// Fixture 是内置样本名。
	Fixture string
	// Mode 是执行模式。
	Mode string
}

// Agent 持有工具、策略和存储。
type Agent struct {
	// cfg 是运行配置。
	cfg Config
	// loadTool 加载 Skill。
	loadTool tool.CallableTool
	// runTool 执行 Skill 脚本。
	runTool tool.CallableTool
	// checkTool 执行 Go 检查。
	checkTool tool.CallableTool
	// policy 审批工具调用。
	policy tool.PermissionPolicy
	// store 持久化审计数据。
	store storage.Store
}

// New 创建基于 trpc-agent-go 的 CR Agent。
func New(cfg Config) (*Agent, error) {
	cfg = normalizeConfig(cfg)
	if cfg.SkillsRoot == "" {
		return nil, errors.New("skills root is required")
	}

	// 建立 Skill 仓库，供 skill_load 和 skill_run 共用。
	repo, err := skillrepo.NewFSRepository(cfg.SkillsRoot)
	if err != nil {
		return nil, fmt.Errorf("load skill repository: %w", err)
	}
	// skill_run 和 codeexec 共用同一个执行器。
	exec, err := newExecutor(cfg)
	if err != nil {
		return nil, err
	}

	var store storage.Store
	if cfg.SQLitePath != "" {
		// Agent 只依赖 storage.Store 接口。
		store, err = sqlite.Open(cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("open sqlite store: %w", err)
		}
	}

	// allowlist 只放行 Skill 内固定脚本。
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

// Run 执行一次完整审查。
func (a *Agent) Run(ctx context.Context, req Request) (review.Result, error) {
	start := time.Now()
	// 统一把输入收敛成 diff。
	diff, inputRef, err := readInput(a.cfg, req)
	if err != nil {
		return review.Result{}, err
	}
	// taskID 便于报告和数据库关联。
	taskID := newTaskID(diff)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = ModeRuleOnly
	}

	if a.store != nil {
		// 先记录 running，失败也可回放。
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
		// dry-run 不进入执行器。
		toolCallCount = 1
		result, runRecord, decision, err = a.runDryRun(ctx, taskID)
	} else {
		// 其他模式先执行 code-review Skill。
		result, runRecord, decision, err = a.runSkillChecks(ctx, taskID, diff)
	}
	decisions := []storage.DecisionRecord{decision}
	runs := []storage.SandboxRunRecord{runRecord}
	if mode == ModeSandbox && strings.TrimSpace(req.RepoPath) != "" {
		// sandbox 模式追加 Go 检查。
		checkDecisions, checkRuns := a.runGoSandboxChecks(ctx, taskID, req.RepoPath)
		decisions = append(decisions, checkDecisions...)
		runs = append(runs, checkRuns...)
		toolCallCount += len(checkRuns)
	}
	if err != nil {
		// 执行失败降级为人工复核项。
		result = resultWithRunError(result, err)
	}
	// 汇总报告和数据库共用的指标。
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
		// 当前只统计主 Skill 权限拦截。
		result.Metrics.PermissionBlocks = 1
	}
	// 补齐报告摘要字段。
	result.HumanReviewItems = humanReviewItems(result.Warnings)
	result.GovernanceSummary = governanceSummary(decisions, result.Metrics.PermissionBlocks)
	result.SandboxSummary = sandboxSummary(runs)
	result.Artifacts = reportArtifacts()
	if result.Summary == "" {
		result.Summary = fmt.Sprintf("%d findings, %d warnings", len(result.Findings), len(result.Warnings))
	}

	// 报告文件和 SQLite 使用同一份内容。
	jsonReport, jsonErr := report.BuildJSON(result)
	if jsonErr != nil {
		return review.Result{}, jsonErr
	}
	md := report.BuildMarkdown(result)
	if err := writeReports(a.cfg.OutputDir, jsonReport, []byte(md)); err != nil {
		return review.Result{}, err
	}
	if a.store != nil {
		// 写入完整审计数据。
		if err := a.persist(ctx, taskID, result, decisions, runs, jsonReport, []byte(md)); err != nil {
			return review.Result{}, err
		}
		// 最后标记任务完成。
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

// Close 释放 Agent 持有的存储连接。
func (a *Agent) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

// normalizeConfig 填充默认配置。
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
		// 只允许 code-review 固定脚本。
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
	// codeexec 只接收受控 bash 片段。
	args, _ := json.Marshal(map[string]any{
		"code_blocks": []map[string]string{{
			"language": "bash",
			"code":     goSandboxCode(a.cfg.Runtime, repoPath, command),
		}},
		"execution_id": taskID + "-" + strings.ReplaceAll(command, " ", "-"),
	})
	// 执行前必须经过 PermissionPolicy。
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
		EnvWhitelist:     "PATH,HOME,TMPDIR",
		At:               time.Now(),
	}
	if perm.Action != tool.PermissionActionAllow {
		// 非 allow 不进入执行器。
		run.Status = string(perm.Action)
		return decision, run
	}

	start := time.Now()
	// allow 后才调用 codeexec。
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
		// 用返回文本兜底判断失败。
		run.Status = "failed"
		run.ExitCode = 1
		return decision, run
	}
	run.Status = "ok"
	return decision, run
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

// persist 保存审计和报告数据。
func (a *Agent) persist(ctx context.Context, taskID string, result review.Result, decisions []storage.DecisionRecord, runs []storage.SandboxRunRecord, jsonReport, markdownReport []byte) error {
	// 保存权限决策。
	for _, decision := range decisions {
		if decision.Command == "" && decision.Action == "" {
			continue
		}
		if err := a.store.SaveDecision(ctx, decision); err != nil {
			return err
		}
	}
	if result.Metrics.RedactionCount > 0 {
		// 有脱敏就记录过滤决策。
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
	// 保存沙箱摘要。
	for _, run := range runs {
		if run.Command == "" && run.Status == "" {
			continue
		}
		if err := a.store.SaveSandboxRun(ctx, run); err != nil {
			return err
		}
	}
	// 审查项统一进入 findings 表。
	for _, finding := range persistedReviewItems(result) {
		if err := a.store.SaveFinding(ctx, taskID, finding); err != nil {
			return err
		}
	}
	// 保存聚合指标。
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
	// 保存产物引用。
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
	// 保存最终报告。
	return a.store.SaveReport(ctx, taskID, jsonReport, markdownReport)
}

// persistedReviewItems 返回需要落库的审查项。
func persistedReviewItems(result review.Result) []review.Finding {
	// 用 status 区分 finding、warning 和复核项。
	items := make([]review.Finding, 0, len(result.Findings)+len(result.Warnings)+len(result.HumanReviewItems))
	items = append(items, result.Findings...)
	items = append(items, result.Warnings...)
	items = append(items, result.HumanReviewItems...)
	return review.DedupeFindings(items)
}

// readInput 读取或生成 diff。
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

// readFixtureInput 读取受控样本。
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

// diffFromRepo 从工作区生成 diff。
func diffFromRepo(repoPath string) ([]byte, error) {
	if repoPath == "" {
		return nil, errors.New("repo path is required")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		// Git 工作区直接使用 unified diff。
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
		// 普通目录转换为新增文件 diff。
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

// writeReports 写入报告文件。
func writeReports(dir string, jsonReport, markdownReport []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "review_report.json"), jsonReport, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "review_report.md"), markdownReport, 0o644)
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

// codeExecOutput 提取 codeexec 输出。
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

// sandboxRepoPathForRuntime 返回 runtime 内 repo 路径。
func sandboxRepoPathForRuntime(runtime string, hostRepoPath string) string {
	if runtime == RuntimeContainer {
		return containerRepoMountPath
	}
	return hostRepoPath
}

// goSandboxCode 构造 Go 检查命令。
func goSandboxCode(runtime string, hostRepoPath string, command string) string {
	return "cd " + shellQuote(sandboxRepoPathForRuntime(runtime, hostRepoPath)) + " && " + command
}

// severityCounts 汇总严重级别。
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

// redactionCount 统计脱敏次数。
func redactionCount(findings, warnings []review.Finding) int {
	total := 0
	for _, finding := range append(findings, warnings...) {
		if strings.Contains(finding.Evidence, "[REDACTED]") {
			total++
		}
	}
	return total
}

// humanReviewItems 提取人工复核项。
func humanReviewItems(warnings []review.Finding) []review.Finding {
	var out []review.Finding
	for _, warning := range warnings {
		if warning.Status == "needs_human_review" || warning.Status == "ask" {
			out = append(out, warning)
		}
	}
	return review.DedupeFindings(out)
}

// governanceSummary 生成治理摘要。
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

// sandboxSummary 生成沙箱摘要。
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

// reportArtifacts 声明报告产物。
func reportArtifacts() []review.ArtifactSummary {
	return []review.ArtifactSummary{
		{Name: "review_report.json", Kind: "report", Path: "review_report.json"},
		{Name: "review_report.md", Kind: "report", Path: "review_report.md"},
	}
}

// totalSandboxDuration 汇总沙箱耗时。
func totalSandboxDuration(runs []storage.SandboxRunRecord) int64 {
	var total int64
	for _, run := range runs {
		total += run.DurationMS
	}
	return total
}

// resultWithRunError 将执行错误转为复核项。
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
