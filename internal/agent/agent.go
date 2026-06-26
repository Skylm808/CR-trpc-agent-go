// Package agent 编排基于 trpc-agent-go Skill、权限策略、执行器和存储的
// 自动代码评审链路。
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
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage/sqlite"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	containerexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/container"
	localexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
	skillrepo "trpc.group/trpc-go/trpc-agent-go/skill"
	"trpc.group/trpc-go/trpc-agent-go/tool"
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
)

const (
	defaultSkillName        = "code-review"
	defaultSkillCommand     = "scripts/check.sh"
	defaultOutputLimitBytes = 64 * 1024
	defaultTimeout          = 30 * time.Second
)

// Config 描述 Agent 运行一轮审查所需的稳定依赖和安全边界。
type Config struct {
	SkillsRoot       string
	Runtime          string
	SQLitePath       string
	OutputDir        string
	Timeout          time.Duration
	OutputLimitBytes int
}

// Request 描述一次审查输入；DiffFile、RepoPath 至少需要提供一个。
type Request struct {
	DiffFile string
	RepoPath string
	Mode     string
}

// Store 是 Agent 需要的最小持久化接口，便于后续替换不同 SQL 后端。
type Store interface {
	SaveTask(context.Context, sqlite.Task) error
	SaveFinding(context.Context, string, review.Finding) error
	SaveDecision(context.Context, sqlite.DecisionRecord) error
	SaveSandboxRun(context.Context, sqlite.SandboxRunRecord) error
	SaveMetrics(context.Context, sqlite.MetricsRecord) error
	SaveReport(context.Context, string, []byte, []byte) error
	Close() error
}

// Agent 持有 trpc-agent-go 工具和本项目持久化实现。
type Agent struct {
	cfg      Config
	loadTool tool.CallableTool
	runTool  tool.CallableTool
	policy   tool.PermissionPolicy
	store    Store
}

// New 创建一个框架优先的 CR Agent。
func New(cfg Config) (*Agent, error) {
	cfg = normalizeConfig(cfg)
	if cfg.SkillsRoot == "" {
		return nil, errors.New("skills root is required")
	}

	repo, err := skillrepo.NewFSRepository(cfg.SkillsRoot)
	if err != nil {
		return nil, fmt.Errorf("load skill repository: %w", err)
	}
	exec, err := newExecutor(cfg)
	if err != nil {
		return nil, err
	}

	var store Store
	if cfg.SQLitePath != "" {
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
		cfg:      cfg,
		loadTool: toolskill.NewLoadTool(repo),
		runTool:  runTool,
		policy:   defaultPermissionPolicy(),
		store:    store,
	}, nil
}

// Run 执行一次完整审查：采集输入、加载 Skill、权限判定、执行脚本、生成报告并落库。
func (a *Agent) Run(ctx context.Context, req Request) (review.Result, error) {
	start := time.Now()
	diff, inputRef, err := readInput(req)
	if err != nil {
		return review.Result{}, err
	}
	taskID := newTaskID(diff)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = ModeRuleOnly
	}

	if a.store != nil {
		if err := a.store.SaveTask(ctx, sqlite.Task{
			ID: taskID, InputType: "diff", InputRef: inputRef,
			InputDigest: digestBytes(diff), RepoPath: req.RepoPath,
			Status: "running", Mode: mode, CreatedAt: start,
			StartedAt: start,
		}); err != nil {
			return review.Result{}, err
		}
	}

	result, runRecord, decision, err := a.runSkillChecks(ctx, taskID, diff)
	if err != nil {
		result.Metrics.ExceptionCounts = map[string]int{"skill_run": 1}
	}
	result.TaskID = taskID
	result.Created = time.Now()
	result.Metrics.TotalDurationMS = time.Since(start).Milliseconds()
	result.Metrics.ToolCallCount = 2
	result.Metrics.SandboxDurationMS = runRecord.DurationMS
	result.Metrics.FindingCount = len(result.Findings)
	result.Metrics.SeverityCounts = severityCounts(result.Findings, result.Warnings)
	if decision.Action != string(tool.PermissionActionAllow) {
		result.Metrics.PermissionBlocks = 1
	}
	if result.Summary == "" {
		result.Summary = fmt.Sprintf("%d findings, %d warnings", len(result.Findings), len(result.Warnings))
	}

	jsonReport, jsonErr := report.BuildJSON(result)
	if jsonErr != nil {
		return review.Result{}, jsonErr
	}
	md := report.BuildMarkdown(result)
	if err := writeReports(a.cfg.OutputDir, jsonReport, []byte(md)); err != nil {
		return review.Result{}, err
	}
	if a.store != nil {
		if err := a.persist(ctx, taskID, result, decision, runRecord, jsonReport, []byte(md)); err != nil {
			return review.Result{}, err
		}
		if err := a.store.SaveTask(ctx, sqlite.Task{
			ID: taskID, InputType: "diff", InputRef: inputRef,
			InputDigest: digestBytes(diff), RepoPath: req.RepoPath,
			Status: "done", Mode: mode, CreatedAt: start,
			StartedAt: start, FinishedAt: time.Now(),
		}); err != nil {
			return review.Result{}, err
		}
	}
	return result, err
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
		exec, err := containerexec.New()
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
		return tool.AskPermission("unrecognized tool command requires human review"), nil
	})
}

// runSkillChecks 通过 skill_load 与 skill_run 执行 code-review Skill。
func (a *Agent) runSkillChecks(ctx context.Context, taskID string, diff []byte) (review.Result, sqlite.SandboxRunRecord, sqlite.DecisionRecord, error) {
	loadArgs := []byte(`{"skill":"code-review"}`)
	if _, err := a.loadTool.Call(ctx, loadArgs); err != nil {
		return review.Result{}, sqlite.SandboxRunRecord{}, sqlite.DecisionRecord{}, err
	}

	runArgs, err := json.Marshal(map[string]any{
		"skill":   defaultSkillName,
		"command": defaultSkillCommand,
		"stdin":   string(diff),
		"timeout": int(a.cfg.Timeout.Seconds()),
	})
	if err != nil {
		return review.Result{}, sqlite.SandboxRunRecord{}, sqlite.DecisionRecord{}, err
	}

	permReq := &tool.PermissionRequest{
		Tool:        a.runTool,
		ToolName:    a.runTool.Declaration().Name,
		Declaration: a.runTool.Declaration(),
		Arguments:   runArgs,
	}
	perm, err := a.policy.CheckToolPermission(ctx, permReq)
	if err != nil {
		return review.Result{}, sqlite.SandboxRunRecord{}, sqlite.DecisionRecord{}, err
	}
	perm, err = tool.NormalizePermissionDecision(perm)
	if err != nil {
		return review.Result{}, sqlite.SandboxRunRecord{}, sqlite.DecisionRecord{}, err
	}
	decision := sqlite.DecisionRecord{
		TaskID: taskID, Command: defaultSkillCommand,
		Action: string(perm.Action), Reason: perm.Reason, At: time.Now(),
	}
	runRecord := sqlite.SandboxRunRecord{
		TaskID: taskID, Command: defaultSkillCommand,
		Runtime: a.cfg.Runtime, TimeoutMS: a.cfg.Timeout.Milliseconds(),
		OutputLimitBytes: a.cfg.OutputLimitBytes,
		EnvWhitelist:     "PATH,HOME,TMPDIR",
		At:               time.Now(),
	}
	if perm.Action != tool.PermissionActionAllow {
		runRecord.Status = string(perm.Action)
		return review.Result{Warnings: []review.Finding{{
			Severity: "low", Category: "governance", Title: "Command requires human review",
			Evidence: perm.Reason, Confidence: "high", Source: "permission",
			RuleID: "permission-non-allow", Status: "needs_human_review",
		}}}, runRecord, decision, nil
	}

	start := time.Now()
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
	if out.ExitCode != 0 || out.TimedOut {
		runRecord.Status = "failed"
	}
	runRecord.ExitCode = out.ExitCode
	runRecord.StdoutDigest = digestString(out.Stdout)
	runRecord.StderrDigest = digestString(out.Stderr)
	if runRecord.DurationMS == 0 {
		runRecord.DurationMS = out.DurationMS
	}

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

type skillRunOutput struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMS int64  `json:"duration_ms"`
}

// parseSkillFindings 解析 Skill 脚本 stdout 中的结构化 findings。
func parseSkillFindings(stdout string) (review.Result, error) {
	var payload struct {
		Findings []review.Finding `json:"findings"`
		Warnings []review.Finding `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		return review.Result{}, err
	}
	for i := range payload.Findings {
		payload.Findings[i] = sanitizeFinding(payload.Findings[i])
	}
	for i := range payload.Warnings {
		payload.Warnings[i] = sanitizeFinding(payload.Warnings[i])
	}
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
func (a *Agent) persist(ctx context.Context, taskID string, result review.Result, decision sqlite.DecisionRecord, run sqlite.SandboxRunRecord, jsonReport, markdownReport []byte) error {
	if err := a.store.SaveDecision(ctx, decision); err != nil {
		return err
	}
	if err := a.store.SaveSandboxRun(ctx, run); err != nil {
		return err
	}
	for _, finding := range result.Findings {
		if err := a.store.SaveFinding(ctx, taskID, finding); err != nil {
			return err
		}
	}
	if err := a.store.SaveMetrics(ctx, sqlite.MetricsRecord{
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
	return a.store.SaveReport(ctx, taskID, jsonReport, markdownReport)
}

// readInput 读取 diff 文件或从 repo path 生成统一 diff。
func readInput(req Request) ([]byte, string, error) {
	if req.DiffFile != "" {
		b, err := os.ReadFile(req.DiffFile)
		return b, req.DiffFile, err
	}
	if req.RepoPath != "" {
		b, err := diffFromRepo(req.RepoPath)
		return b, req.RepoPath, err
	}
	return nil, "", errors.New("diff file or repo path is required")
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
