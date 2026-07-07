package review

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/rules"
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
	result := rules.Run(toRulesDiff(diff), rules.Options{Redact: RedactSecrets})
	return Analysis{
		Findings: fromRuleFindings(result.Findings),
		Warnings: fromRuleFindings(result.Warnings),
		Diff:     diff,
	}
}

func toRulesDiff(diff ParsedDiff) rules.ParsedDiff {
	out := rules.ParsedDiff{
		Files: make([]rules.ParsedFile, 0, len(diff.Files)),
	}
	for _, file := range diff.Files {
		converted := rules.ParsedFile{
			Path:       file.Path,
			IsTestFile: file.IsTestFile,
			Hunks:      make([]rules.Hunk, 0, len(file.Hunks)),
		}
		for _, hunk := range file.Hunks {
			convertedHunk := rules.Hunk{
				Lines: make([]rules.Line, 0, len(hunk.Lines)),
			}
			for _, line := range hunk.Lines {
				convertedHunk.Lines = append(convertedHunk.Lines, rules.Line{
					NewLine: line.NewLine,
					Kind:    line.Kind,
					Text:    line.Text,
				})
			}
			converted.Hunks = append(converted.Hunks, convertedHunk)
		}
		out.Files = append(out.Files, converted)
	}
	return out
}

func fromRuleFindings(findings []rules.Finding) []Finding {
	out := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		out = append(out, Finding{
			Severity:       finding.Severity,
			Category:       finding.Category,
			File:           finding.File,
			Line:           finding.Line,
			Title:          finding.Title,
			Evidence:       finding.Evidence,
			Recommendation: finding.Recommendation,
			Confidence:     finding.Confidence,
			Source:         finding.Source,
			RuleID:         finding.RuleID,
			Status:         finding.Status,
		})
	}
	return out
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
