// Package report renders the structured review result into JSON and Markdown
// artifacts for users and CI systems.
package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// BuildJSON serializes the review result after deduplicating findings.
func BuildJSON(result review.Result) ([]byte, error) {
	result.Findings = review.DedupeFindings(result.Findings)
	return json.MarshalIndent(result, "", "  ")
}

// BuildMarkdown formats the review result into a readable Markdown summary.
func BuildMarkdown(result review.Result) string {
	findings := review.DedupeFindings(result.Findings)
	var b strings.Builder
	b.WriteString("# Review Report\n\n")
	if result.Summary != "" {
		b.WriteString(result.Summary)
		b.WriteString("\n\n")
	}
	if result.Metrics.FindingCount > 0 || result.Metrics.TotalDurationMS > 0 {
		fmt.Fprintf(&b, "Metrics: findings=%d total_ms=%d sandbox_ms=%d tool_calls=%d permission_blocks=%d redactions=%d\n\n",
			result.Metrics.FindingCount,
			result.Metrics.TotalDurationMS,
			result.Metrics.SandboxDurationMS,
			result.Metrics.ToolCallCount,
			result.Metrics.PermissionBlocks,
			result.Metrics.RedactionCount,
		)
	}
	fmt.Fprintf(&b, "Findings: %d\n\n", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "- [%s] %s:%d %s\n", strings.ToUpper(f.Severity), f.File, f.Line, f.Title)
		if f.Evidence != "" {
			fmt.Fprintf(&b, "  - Evidence: %s\n", f.Evidence)
		}
		if f.Recommendation != "" {
			fmt.Fprintf(&b, "  - Recommendation: %s\n", f.Recommendation)
		}
	}
	return b.String()
}
