// Package review 包含第一版 CR Agent 使用的确定性 diff 解析器、规则引擎和共享数据结构。
package review

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Result 是一次审查运行的标准化输出。
type Result struct {
	TaskID            string            `json:"task_id"`
	Findings          []Finding         `json:"findings"`
	Warnings          []Finding         `json:"warnings,omitempty"`
	HumanReviewItems  []Finding         `json:"human_review_items"`
	Metrics           Metrics           `json:"metrics,omitempty"`
	GovernanceSummary GovernanceSummary `json:"governance_summary"`
	SandboxSummary    SandboxSummary    `json:"sandbox_summary"`
	Artifacts         []ArtifactSummary `json:"artifacts"`
	Summary           string            `json:"summary,omitempty"`
	Created           time.Time         `json:"created_at,omitempty"`
}

// Metrics 保存可安全持久化并在报告中展示的粗粒度审查遥测。
type Metrics struct {
	TotalDurationMS   int64          `json:"total_duration_ms,omitempty"`
	SandboxDurationMS int64          `json:"sandbox_duration_ms,omitempty"`
	ToolCallCount     int            `json:"tool_call_count,omitempty"`
	PermissionBlocks  int            `json:"permission_block_count,omitempty"`
	FindingCount      int            `json:"finding_count,omitempty"`
	SeverityCounts    map[string]int `json:"severity_counts,omitempty"`
	ExceptionCounts   map[string]int `json:"exception_counts,omitempty"`
	RedactionCount    int            `json:"redaction_count,omitempty"`
}

// GovernanceSummary 汇总一次审查中的权限与过滤策略决策。
type GovernanceSummary struct {
	PermissionDecisions []PermissionDecisionSummary `json:"permission_decisions,omitempty"`
	FilterDecisions     []FilterDecisionSummary     `json:"filter_decisions,omitempty"`
	PermissionBlocks    int                         `json:"permission_blocks,omitempty"`
}

// PermissionDecisionSummary 是报告层可展示的权限决策摘要。
type PermissionDecisionSummary struct {
	Command string `json:"command"`
	Action  string `json:"action"`
	Reason  string `json:"reason,omitempty"`
}

// FilterDecisionSummary 是报告层可展示的过滤/脱敏决策摘要。
type FilterDecisionSummary struct {
	Target string `json:"target"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// SandboxSummary 汇总沙箱执行尝试和失败状态。
type SandboxSummary struct {
	Runs []SandboxRunSummary `json:"runs,omitempty"`
}

// SandboxRunSummary 是报告层可展示的单次沙箱执行摘要。
type SandboxRunSummary struct {
	Command          string `json:"command"`
	Runtime          string `json:"runtime"`
	Status           string `json:"status"`
	TimeoutMS        int64  `json:"timeout_ms"`
	OutputLimitBytes int    `json:"output_limit_bytes"`
	EnvWhitelist     string `json:"env_whitelist,omitempty"`
	ExitCode         int    `json:"exit_code,omitempty"`
	StdoutDigest     string `json:"stdout_digest,omitempty"`
	StderrDigest     string `json:"stderr_digest,omitempty"`
	DurationMS       int64  `json:"duration_ms"`
}

// ArtifactSummary 描述报告中引用的持久化产物。
type ArtifactSummary struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Digest string `json:"digest,omitempty"`
}

// Finding 是规则引擎输出的结构化审查问题。
type Finding struct {
	Severity       string `json:"severity"`
	Category       string `json:"category"`
	File           string `json:"file"`
	Line           int    `json:"line"`
	Title          string `json:"title"`
	Evidence       string `json:"evidence,omitempty"`
	Recommendation string `json:"recommendation,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Source         string `json:"source"`
	RuleID         string `json:"rule_id"`
	Status         string `json:"status,omitempty"`
}

// DedupeKey 返回一个稳定 key，用于合并指向同一文件、行、类别和规则的重复 finding。
func (f Finding) DedupeKey() string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		strings.ToLower(strings.TrimSpace(f.File)),
		fmt.Sprintf("%d", f.Line),
		strings.ToLower(strings.TrimSpace(f.Category)),
		strings.ToLower(strings.TrimSpace(f.RuleID)),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

// ParsedDiff 是统一 diff 的标准化表示。
type ParsedDiff struct {
	Files []ParsedFile `json:"files"`
}

// ParsedFile 描述 diff 中的一份变更文件。
type ParsedFile struct {
	Path        string `json:"path"`
	Language    string `json:"language"`
	PackageName string `json:"package_name,omitempty"`
	IsTestFile  bool   `json:"is_test_file"`
	ChangeType  string `json:"change_type,omitempty"`
	Hunks       []Hunk `json:"hunks"`
}

// Hunk 表示一个 diff hunk，包含行级上下文和候选行号。
type Hunk struct {
	File           string   `json:"file"`
	OldStart       int      `json:"old_start"`
	OldLines       int      `json:"old_lines"`
	NewStart       int      `json:"new_start"`
	NewLines       int      `json:"new_lines"`
	Context        []string `json:"context,omitempty"`
	CandidateLines []int    `json:"candidate_lines,omitempty"`
	Lines          []Line   `json:"lines,omitempty"`
}

// Line 保存 hunk 中的一行以及旧行号和新行号。
type Line struct {
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
}

// RedactSecrets 在写入 findings、报告或存储之前，替换常见的敏感信息形态值。
func RedactSecrets(input string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password)\b\s*[:=]\s*([^\s,;]+)`),
		regexp.MustCompile(`sk-[A-Za-z0-9]{12,}`),
		regexp.MustCompile(`(?i)\bearer\s+[A-Za-z0-9\-._~+/=]+`),
	}
	out := input
	for _, re := range patterns {
		out = re.ReplaceAllStringFunc(out, func(s string) string {
			return re.ReplaceAllString(s, "$1=[REDACTED]")
		})
	}
	return out
}

// DedupeFindings 保留每个唯一 finding key 的第一次出现，并返回稳定排序后的切片。
func DedupeFindings(findings []Finding) []Finding {
	seen := map[string]struct{}{}
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		key := f.DedupeKey()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

// MustJSON 将值格式化为 pretty JSON，并刻意忽略错误，因为它只用于内部遥测快照。
func MustJSON(v any) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return b
}
