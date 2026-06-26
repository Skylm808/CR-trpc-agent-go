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
		Summary:   fmt.Sprintf("%d findings, %d warnings", len(findings), len(warnings)),
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
						Severity:      "medium",
						Category:      "maintainability",
						File:          file.Path,
						Line:          line.NewLine,
						Title:         "New code contains a TODO or FIXME marker",
						Evidence:      RedactSecrets(text),
						Recommendation: "Remove the marker or turn it into a tracked issue before merging.",
						Confidence:    "high",
						Source:        "rule",
						RuleID:        "todo-marker",
						Status:        "finding",
					})
				}
				if file.IsTestFile {
					// Test files are exempt from the missing-test hint because
					// they are already the test surface.
					continue
				}
				// Direct panic paths are flagged because the agent targets Go
				// review, where explicit error handling is preferred in shared code.
				if strings.HasPrefix(text, "func ") && strings.Contains(text, "panic(") {
					out.Findings = append(out.Findings, Finding{
						Severity:      "high",
						Category:      "error_handling",
						File:          file.Path,
						Line:          line.NewLine,
						Title:         "New function panics directly",
						Evidence:      RedactSecrets(text),
						Recommendation: "Return an error or handle the failure path explicitly.",
						Confidence:    "high",
						Source:        "rule",
						RuleID:        "panic-direct",
						Status:        "finding",
					})
				}
				// A new function with no obvious error path is a weak signal that
				// the change might need a focused test, so keep it as a warning.
				if strings.HasPrefix(text, "func ") && !strings.Contains(text, "error") {
					out.Warnings = append(out.Warnings, Finding{
						Severity:      "low",
						Category:      "testing",
						File:          file.Path,
						Line:          line.NewLine,
						Title:         "New function may need a focused test",
						Evidence:      RedactSecrets(text),
						Recommendation: "Add a unit test that exercises the new path.",
						Confidence:    "medium",
						Source:        "rule",
						RuleID:        "missing-test-hint",
						Status:        "warning",
					})
				}
				// Literal secrets are reported as critical findings and the
				// evidence is redacted before storage or reporting.
				if strings.Contains(strings.ToLower(text), "password") ||
					strings.Contains(strings.ToLower(text), "token") ||
					strings.Contains(strings.ToLower(text), "secret") ||
					secretToken.MatchString(text) {
					out.Findings = append(out.Findings, Finding{
						Severity:      "critical",
						Category:      "security",
						File:          file.Path,
						Line:          line.NewLine,
						Title:         "Potential secret appears in added code",
						Evidence:      RedactSecrets(text),
						Recommendation: "Replace the literal with a secret manager or environment lookup.",
						Confidence:    "high",
						Source:        "rule",
						RuleID:        "secret-leak",
						Status:        "finding",
					})
				}
			}
		}
	}
	return out
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
