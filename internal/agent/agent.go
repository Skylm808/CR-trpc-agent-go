// Package agent 编排基于 trpc-agent-go 的代码评审链路。
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/report"
	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"trpc.group/trpc-go/trpc-agent-go/artifact"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	skillrepo "trpc.group/trpc-go/trpc-agent-go/skill"
	telemetrytrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
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
	goSandboxCacheDir       = "/tmp/cr-agent-gocache"
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
	// ArtifactService 接入官方 artifact service。
	ArtifactService artifact.Service
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
	// exec 是底层执行器，供 workspaceexec 使用。
	exec codeexecutor.CodeExecutor
	// policy 审批工具调用。
	policy tool.PermissionPolicy
	// store 持久化审计数据。
	store storage.Store
	// artifactService 保存官方产物。
	artifactService artifact.Service
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
		cfg:             cfg,
		loadTool:        toolskill.NewLoadTool(repo),
		runTool:         runTool,
		checkTool:       toolcodeexec.NewTool(exec, toolcodeexec.WithName("execute_code"), toolcodeexec.WithLanguages("bash")),
		exec:            exec,
		policy:          defaultPermissionPolicy(),
		store:           store,
		artifactService: cfg.ArtifactService,
	}, nil
}

// Run 执行一次完整审查。
func (a *Agent) Run(ctx context.Context, req Request) (result review.Result, err error) {
	ctx, span := telemetrytrace.Tracer.Start(ctx, "cr-agent.review")
	defer func() {
		if err != nil {
			recordReviewErrorTelemetry(span, err)
		}
		span.End()
	}()

	start := time.Now()
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = ModeRuleOnly
	}
	recordReviewStartTelemetry(span, a.cfg, req, mode)

	// 统一把输入收敛成 diff。
	diff, inputRef, err := readInput(a.cfg, req)
	if err != nil {
		return review.Result{}, err
	}
	// taskID 便于报告和数据库关联。
	taskID := newTaskID(diff)
	span.SetAttributes(attribute.String("cr_agent.task_id", taskID))

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
	result.Conclusion = conclusion(result)

	// 报告文件和 SQLite 使用同一份内容。
	jsonReport, jsonErr := report.BuildJSON(result)
	if jsonErr != nil {
		return review.Result{}, jsonErr
	}
	md := report.BuildMarkdown(result)
	diagnosticsReport, err := buildDiagnostics(result)
	if err != nil {
		return review.Result{}, err
	}
	if err := writeReports(a.cfg.OutputDir, jsonReport, []byte(md), diagnosticsReport); err != nil {
		return review.Result{}, err
	}
	if a.artifactService != nil {
		if err := a.saveArtifacts(ctx, taskID, result, jsonReport, []byte(md), diagnosticsReport); err != nil {
			return review.Result{}, err
		}
	}
	if a.store != nil {
		// 写入完整审计数据。
		if err := a.persist(ctx, taskID, result, decisions, runs, jsonReport, []byte(md), diagnosticsReport); err != nil {
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
	recordReviewResultTelemetry(span, result)
	return result, nil
}

// Close 释放 Agent 持有的存储连接。
func (a *Agent) Close() error {
	if a == nil || a.store == nil {
		return nil
	}
	return a.store.Close()
}

// saveArtifacts 使用官方 artifact service 持久化报告和诊断产物。
func (a *Agent) saveArtifacts(ctx context.Context, taskID string, result review.Result, jsonReport, markdownReport, diagnosticsReport []byte) error {
	sessionInfo := artifact.SessionInfo{
		AppName:   "cr-agent",
		UserID:    "local",
		SessionID: taskID,
	}
	for _, art := range result.Artifacts {
		var payload []byte
		var mime string
		switch art.Name {
		case "review_report.json":
			payload = jsonReport
			mime = "application/json"
		case "review_report.md":
			payload = markdownReport
			mime = "text/markdown"
		case "review_diagnostics.json":
			payload = diagnosticsReport
			mime = "application/json"
		default:
			continue
		}
		if _, err := a.artifactService.SaveArtifact(ctx, sessionInfo, art.Path, &artifact.Artifact{
			Data:     payload,
			MimeType: mime,
			Name:     art.Name,
		}); err != nil {
			return err
		}
	}
	return nil
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

// recordReviewStartTelemetry 记录审查入口边界。
func recordReviewStartTelemetry(span oteltrace.Span, cfg Config, req Request, mode string) {
	span.SetAttributes(
		attribute.String("cr_agent.runtime", cfg.Runtime),
		attribute.String("cr_agent.mode", mode),
		attribute.String("cr_agent.input_type", requestInputKind(req)),
		attribute.Bool("cr_agent.staticcheck_enabled", cfg.EnableStaticcheck),
	)
}

// recordReviewResultTelemetry 记录审查结果摘要。
func recordReviewResultTelemetry(span oteltrace.Span, result review.Result) {
	span.SetAttributes(
		attribute.Int("cr_agent.finding_count", len(result.Findings)),
		attribute.Int("cr_agent.warning_count", len(result.Warnings)),
		attribute.Int("cr_agent.human_review_count", len(result.HumanReviewItems)),
		attribute.Int("cr_agent.artifact_count", len(result.Artifacts)),
		attribute.Int("cr_agent.permission_block_count", result.Metrics.PermissionBlocks),
		attribute.Int("cr_agent.tool_call_count", result.Metrics.ToolCallCount),
		attribute.Int("cr_agent.sandbox_run_count", len(result.SandboxSummary.Runs)),
		attribute.Int("cr_agent.redaction_count", result.Metrics.RedactionCount),
		attribute.Int("cr_agent.exception_count", exceptionCount(result.Metrics.ExceptionCounts)),
		attribute.Int64("cr_agent.total_duration_ms", result.Metrics.TotalDurationMS),
		attribute.Int64("cr_agent.sandbox_duration_ms", result.Metrics.SandboxDurationMS),
	)
}

// recordReviewErrorTelemetry 记录失败状态但不写入敏感错误正文。
func recordReviewErrorTelemetry(span oteltrace.Span, err error) {
	span.SetStatus(codes.Error, "review failed")
	span.SetAttributes(attribute.String("cr_agent.error_type", fmt.Sprintf("%T", err)))
}

// requestInputKind 返回审查输入类型。
func requestInputKind(req Request) string {
	switch {
	case strings.TrimSpace(req.DiffFile) != "":
		return "diff_file"
	case strings.TrimSpace(req.RepoPath) != "":
		return "repo_path"
	case strings.TrimSpace(req.Fixture) != "":
		return "fixture"
	default:
		return "unknown"
	}
}

func exceptionCount(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}
