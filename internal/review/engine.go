package review

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Analysis 是规则执行工作集。
type Analysis struct {
	TaskID   string
	Findings []Finding
	Warnings []Finding
	Diff     ParsedDiff
}

// AnalyzeDiff 解析 diff 并执行规则。
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
				// TODO/FIXME 作为可维护性问题提示。
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
				// 共享代码中直接 panic 通常应改为显式错误处理。
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
					// 测试文件跳过缺失测试提示。
					continue
				}
				// 新函数默认提示补充测试。
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
				// secret 证据在报告和落库前会脱敏。
				if shouldReportSecret(text) {
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

// hunkJoinedText 拼接 hunk 文本。
func hunkJoinedText(hunk Hunk) string {
	var b strings.Builder
	for _, line := range hunk.Lines {
		b.WriteString(line.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// containsAny 判断是否包含任一子串。
func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// hasRuleInFile 判断文件内规则是否已命中。
func hasRuleInFile(findings []Finding, file string, ruleID string) bool {
	for _, finding := range findings {
		if finding.File == file && finding.RuleID == ruleID {
			return true
		}
	}
	return false
}

var (
	secretValuePattern = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|ghp_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|Bearer\s+[A-Za-z0-9\-._~+/=]{8,}|[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}|-----BEGIN [A-Z ]*PRIVATE KEY-----|[a-z][a-z0-9+.-]*://[^/\s:@]+:[^@\s/]+@)`)
	secretNamePattern  = regexp.MustCompile(`(?i)(api[_-]?key|apikey|llm[_-]?key|openai[_-]?(api[_-]?)?key|client[_-]?secret|secret|token|bearer[_-]?token|password|passwd|pwd|github[_-]?token|private[_-]?key)`)
	stringLiteralValue = regexp.MustCompile(`=\s*("([^"]*)"|'([^']*)'|` + "`" + `([^` + "`" + `]*)` + "`" + `)`)
	placeholderSecret  = regexp.MustCompile(`(?i)^(test|example|dummy|placeholder|changeme|change-me|your[-_ ]?token|your[-_ ]?key|xxx+|<.*>)$`)
)

// shouldReportSecret 判断新增行是否包含高置信密钥。
func shouldReportSecret(text string) bool {
	if secretValuePattern.MatchString(text) {
		return true
	}
	if !secretNamePattern.MatchString(text) {
		return false
	}
	value, ok := extractAssignedString(text)
	if !ok {
		return false
	}
	value = strings.TrimSpace(value)
	if len(value) < 12 {
		return false
	}
	return !placeholderSecret.MatchString(value)
}

// extractAssignedString 提取赋值右侧的字符串字面量。
func extractAssignedString(text string) (string, bool) {
	match := stringLiteralValue.FindStringSubmatch(text)
	if len(match) == 0 {
		return "", false
	}
	for _, group := range match[2:] {
		if group != "" {
			return group, true
		}
	}
	return "", false
}

var ErrEmptyInput = errors.New("empty review input")

// BuildReport 是外部入口。
func BuildReport(input string) (Result, error) {
	if strings.TrimSpace(input) == "" {
		return Result{}, ErrEmptyInput
	}
	return AnalyzeDiff(input)
}

// PackageFromPath 从路径推导包名。
func PackageFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
