package review

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Analysis is the internal working set produced while applying rules.
type Analysis struct {
	TaskID   string
	Findings []Finding
	Warnings []Finding
	Diff     ParsedDiff
}

// AnalyzeDiff parses the diff, runs rules, deduplicates the output, and
// attaches a lightweight telemetry snapshot to the result.
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
	// Secret-shaped literals are treated as high-risk regardless of file
	// type, because the first version is biased toward safety.
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
				// TODO/FIXME markers are not blocking by themselves, but they
				// are useful medium-severity maintainability findings.
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
				// Direct panic paths are flagged because the agent targets Go
				// review, where explicit error handling is preferred in shared code.
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
					// Test files are exempt from the missing-test hint because
					// they are already the test surface.
					continue
				}
				// A new function with no obvious error path is a weak signal that
				// the change might need a focused test, so keep it as a warning.
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
				// Literal secrets are reported as critical findings and the
				// evidence is redacted before storage or reporting.
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

// hunkJoinedText collapses one hunk into a lower-friction search surface for
// simple lifecycle rules that need to see more than the current line.
func hunkJoinedText(hunk Hunk) string {
	var b strings.Builder
	for _, line := range hunk.Lines {
		b.WriteString(line.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// containsAny checks whether the joined hunk text contains any of the
// provided substrings.
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

// BuildReport is the external entry point used by the CLI and tests.
func BuildReport(input string) (Result, error) {
	if strings.TrimSpace(input) == "" {
		return Result{}, ErrEmptyInput
	}
	return AnalyzeDiff(input)
}

// PackageFromPath derives a Go package-like name from a file path.
func PackageFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
