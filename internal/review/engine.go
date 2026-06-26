package review

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Analysis 是规则执行过程中生成的内部工作集。
type Analysis struct {
	TaskID   string
	Findings []Finding
	Warnings []Finding
	Diff     ParsedDiff
}

// AnalyzeDiff 负责解析 diff、执行规则、去重输出，并为结果附加轻量遥测快照。
func AnalyzeDiff(input string) (Result, error) {
	start := time.Now()
	parsed, err := ParseUnifiedDiff(input)
	if err != nil {
		return Result{}, err
	}
	analysis := runRules(parsed)
	findings := DedupeFindings(analysis.Findings)
	warnings := DedupeFindings(analysis.Warnings)
	metrics := Metrics{
		TotalDurationMS: int64(time.Since(start).Milliseconds()),
		FindingCount:    len(findings),
		SeverityCounts:  map[string]int{},
		ExceptionCounts: map[string]int{},
	}
	for _, f := range findings {
		metrics.SeverityCounts[f.Severity]++
	}
	for _, w := range warnings {
		metrics.SeverityCounts[w.Severity]++
	}
	return Result{
		Findings: findings,
		Warnings: warnings,
		Metrics:  metrics,
		Summary:  fmt.Sprintf("%d findings, %d warnings", len(findings), len(warnings)),
	}, nil
}

func runRules(diff ParsedDiff) Analysis {
	var out Analysis
	out.Diff = diff
	// 形似 secret 的字面量无论文件类型都视为高风险，因为第一版更偏向安全。
	secretToken := regexp.MustCompile(`sk-[A-Za-z0-9]{12,}`)
	for _, file := range diff.Files {
		for _, hunk := range file.Hunks {
			hunkText := hunkJoinedText(hunk)
			for _, line := range hunk.Lines {
				if line.Kind != "add" {
					continue
				}
				text := strings.TrimSpace(line.Text)
				if file.Path == "" {
					continue
				}
				// TODO/FIXME 标记本身不阻断，但作为中等严重级别的可维护性问题很有价值。
				if strings.Contains(text, "TODO(") || strings.Contains(text, "FIXME") {
					out.Findings = append(out.Findings, Finding{
						Severity:       "medium",
						Category:       "maintainability",
						File:           file.Path,
						Line:           line.NewLine,
						Title:          "New code contains a TODO or FIXME marker",
						Evidence:       RedactSecrets(text),
						Recommendation: "Remove the marker or turn it into a tracked issue before merging.",
						Confidence:     "high",
						Source:         "rule",
						RuleID:         "todo-marker",
						Status:         "finding",
					})
				}
				// 直接 panic 的路径会被标记，因为这个 Agent 面向 Go 代码审查，共享代码更偏好显式错误处理。
				if strings.Contains(text, "panic(") && !hasRuleInFile(out.Findings, file.Path, "panic-direct") {
					out.Findings = append(out.Findings, Finding{
						Severity:       "high",
						Category:       "error_handling",
						File:           file.Path,
						Line:           line.NewLine,
						Title:          "New function panics directly",
						Evidence:       RedactSecrets(text),
						Recommendation: "Return an error or handle the failure path explicitly.",
						Confidence:     "high",
						Source:         "rule",
						RuleID:         "panic-direct",
						Status:         "finding",
					})
				}
				if file.IsTestFile {
					// 测试文件本身就是测试面，因此跳过缺失测试提示。
					continue
				}
				// 新函数如果没有明显错误路径，通常意味着可能需要专门测试，因此保留为 warning。
				if strings.HasPrefix(text, "func ") && !strings.Contains(text, "error") {
					out.Warnings = append(out.Warnings, Finding{
						Severity:       "low",
						Category:       "testing",
						File:           file.Path,
						Line:           line.NewLine,
						Title:          "New function may need a focused test",
						Evidence:       RedactSecrets(text),
						Recommendation: "Add a unit test that exercises the new path.",
						Confidence:     "medium",
						Source:         "rule",
						RuleID:         "missing-test-hint",
						Status:         "warning",
					})
				}
				if strings.Contains(text, "go func") || strings.HasPrefix(text, "go ") {
					if !containsAny(hunkText, "WaitGroup", "ctx.Done", "errgroup", "done", "sync.") {
						out.Findings = append(out.Findings, Finding{
							Severity:       "high",
							Category:       "concurrency",
							File:           file.Path,
							Line:           line.NewLine,
							Title:          "New goroutine has no visible lifecycle guard",
							Evidence:       RedactSecrets(text),
							Recommendation: "Bind the goroutine to a context, wait group, or explicit completion signal.",
							Confidence:     "high",
							Source:         "rule",
							RuleID:         "goroutine-leak",
							Status:         "finding",
						})
					}
				}
				if strings.Contains(text, "context.WithCancel") ||
					strings.Contains(text, "context.WithTimeout") ||
					strings.Contains(text, "context.WithDeadline") {
					if !containsAny(hunkText, "defer cancel()", "ctx.Done", "cancel()") {
						out.Findings = append(out.Findings, Finding{
							Severity:       "high",
							Category:       "lifecycle",
							File:           file.Path,
							Line:           line.NewLine,
							Title:          "Derived context is not canceled",
							Evidence:       RedactSecrets(text),
							Recommendation: "Store the cancel function and defer cancel() in the same scope.",
							Confidence:     "high",
							Source:         "rule",
							RuleID:         "context-leak",
							Status:         "finding",
						})
					}
				}
				if strings.Contains(text, "os.Open") || strings.Contains(text, "os.OpenFile") || strings.Contains(text, "os.Create") {
					if !containsAny(hunkText, "defer", "Close()") {
						out.Findings = append(out.Findings, Finding{
							Severity:       "high",
							Category:       "resource",
							File:           file.Path,
							Line:           line.NewLine,
							Title:          "Opened resource has no close path",
							Evidence:       RedactSecrets(text),
							Recommendation: "Defer Close() immediately after the resource is opened.",
							Confidence:     "high",
							Source:         "rule",
							RuleID:         "resource-leak",
							Status:         "finding",
						})
					}
				}
				if strings.Contains(text, "sql.Open") || strings.Contains(text, ".BeginTx") || strings.Contains(text, ".Begin(") {
					if !containsAny(hunkText, "Rollback()", "Close()") {
						out.Findings = append(out.Findings, Finding{
							Severity:       "high",
							Category:       "database",
							File:           file.Path,
							Line:           line.NewLine,
							Title:          "Database handle or transaction has no cleanup path",
							Evidence:       RedactSecrets(text),
							Recommendation: "Defer Close() for handles and Rollback() for transactions in the same scope.",
							Confidence:     "high",
							Source:         "rule",
							RuleID:         "db-lifecycle",
							Status:         "finding",
						})
					}
				}
				// 字面量 secret 会按 critical 级别报告，并在落库或出报告前脱敏 evidence。
				if strings.Contains(strings.ToLower(text), "password") ||
					strings.Contains(strings.ToLower(text), "token") ||
					strings.Contains(strings.ToLower(text), "secret") ||
					secretToken.MatchString(text) {
					out.Findings = append(out.Findings, Finding{
						Severity:       "critical",
						Category:       "security",
						File:           file.Path,
						Line:           line.NewLine,
						Title:          "Potential secret appears in added code",
						Evidence:       RedactSecrets(text),
						Recommendation: "Replace the literal with a secret manager or environment lookup.",
						Confidence:     "high",
						Source:         "rule",
						RuleID:         "secret-leak",
						Status:         "finding",
					})
				}
			}
		}
	}
	return out
}

// hunkJoinedText 将一个 hunk 压缩成更容易搜索的文本，供需要跨行判断的简单生命周期规则使用。
func hunkJoinedText(hunk Hunk) string {
	var b strings.Builder
	for _, line := range hunk.Lines {
		b.WriteString(line.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// containsAny 检查拼接后的 hunk 文本是否包含任一给定子串。
func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// hasRuleInFile 判断同一文件是否已经报告过同一规则，用于少量高噪声规则降噪。
func hasRuleInFile(findings []Finding, file string, ruleID string) bool {
	for _, finding := range findings {
		if finding.File == file && finding.RuleID == ruleID {
			return true
		}
	}
	return false
}

var ErrEmptyInput = errors.New("empty review input")

// BuildReport 是 CLI 和测试使用的外部入口。
func BuildReport(input string) (Result, error) {
	if strings.TrimSpace(input) == "" {
		return Result{}, ErrEmptyInput
	}
	return AnalyzeDiff(input)
}

// PackageFromPath 根据文件路径推导出类似 Go package 的名称。
func PackageFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
