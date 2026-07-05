// Package report 生成 JSON 和 Markdown 报告。
package report

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// BuildJSON 生成 JSON 报告。
func BuildJSON(result review.Result) ([]byte, error) {
	result.Findings = review.DedupeFindings(result.Findings)
	result.HumanReviewItems = humanReviewItems(result)
	return json.MarshalIndent(result, "", "  ")
}

// BuildMarkdown 生成 Markdown 报告。
func BuildMarkdown(result review.Result) string {
	findings := review.DedupeFindings(result.Findings)
	var b strings.Builder
	b.WriteString("# Review Report\n\n")
	if result.Summary != "" {
		b.WriteString(result.Summary)
		b.WriteString("\n\n")
	}
	writeConclusion(&b, result.Conclusion)
	if result.Metrics.FindingCount > 0 || result.Metrics.TotalDurationMS > 0 {
		fmt.Fprintf(&b, "Metrics: findings=%d total_ms=%d sandbox_ms=%d model_ms=%d tool_calls=%d model_calls=%d model_findings=%d model_exceptions=%d permission_blocks=%d redactions=%d\n\n",
			result.Metrics.FindingCount,
			result.Metrics.TotalDurationMS,
			result.Metrics.SandboxDurationMS,
			result.Metrics.ModelDurationMS,
			result.Metrics.ToolCallCount,
			result.Metrics.ModelCallCount,
			result.Metrics.ModelFindingCount,
			result.Metrics.ModelExceptionCount,
			result.Metrics.PermissionBlocks,
			result.Metrics.RedactionCount,
		)
	}
	if result.Metrics.ModelProvider != "" || result.Metrics.ModelName != "" || result.Metrics.ModelBackend != "" {
		fmt.Fprintf(&b, "Model: provider=%s name=%s backend=%s\n\n",
			result.Metrics.ModelProvider,
			result.Metrics.ModelName,
			result.Metrics.ModelBackend,
		)
	}
	if len(result.Metrics.SeverityCounts) > 0 {
		b.WriteString("Severity Counts:\n")
		for severity, count := range result.Metrics.SeverityCounts {
			fmt.Fprintf(&b, "- %s: %d\n", severity, count)
		}
		b.WriteString("\n")
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
	writeHumanReview(&b, humanReviewItems(result))
	writeGovernance(&b, result.GovernanceSummary)
	writeSandbox(&b, result.SandboxSummary)
	writeArtifacts(&b, result.Artifacts)
	return b.String()
}

// humanReviewItems 汇总人工复核项。
func humanReviewItems(result review.Result) []review.Finding {
	items := append([]review.Finding(nil), result.HumanReviewItems...)
	for _, warning := range result.Warnings {
		if warning.Status == "needs_human_review" || warning.Status == "ask" {
			items = append(items, warning)
		}
	}
	return review.DedupeFindings(items)
}

// writeConclusion 渲染最终结论。
func writeConclusion(b *strings.Builder, conclusion review.Conclusion) {
	if conclusion.Status == "" {
		return
	}
	b.WriteString("## Conclusion\n\n")
	fmt.Fprintf(b, "- Status: %s\n", conclusion.Status)
	if conclusion.Reason != "" {
		fmt.Fprintf(b, "- Reason: %s\n", conclusion.Reason)
	}
	if conclusion.Summary != "" {
		fmt.Fprintf(b, "- Summary: %s\n", conclusion.Summary)
	}
	b.WriteString("\n")
}

// writeHumanReview 渲染人工复核项。
func writeHumanReview(b *strings.Builder, items []review.Finding) {
	if len(items) == 0 {
		return
	}
	b.WriteString("\n## Human Review\n\n")
	for _, item := range items {
		fmt.Fprintf(b, "- [%s] %s\n", strings.ToUpper(item.Severity), item.Title)
		if item.Recommendation != "" {
			fmt.Fprintf(b, "  - Recommendation: %s\n", item.Recommendation)
		}
	}
}

// writeGovernance 渲染治理摘要。
func writeGovernance(b *strings.Builder, summary review.GovernanceSummary) {
	if len(summary.PermissionDecisions) == 0 && len(summary.FilterDecisions) == 0 && summary.PermissionBlocks == 0 {
		return
	}
	b.WriteString("\n## Governance\n\n")
	if summary.PermissionBlocks > 0 {
		fmt.Fprintf(b, "- Permission blocks: %d\n", summary.PermissionBlocks)
	}
	for _, decision := range summary.PermissionDecisions {
		fmt.Fprintf(b, "- Permission %s: %s", decision.Action, decision.Command)
		if decision.Reason != "" {
			fmt.Fprintf(b, " (%s)", decision.Reason)
		}
		b.WriteString("\n")
	}
	for _, decision := range summary.FilterDecisions {
		fmt.Fprintf(b, "- Filter %s: %s", decision.Action, decision.Target)
		if decision.Reason != "" {
			fmt.Fprintf(b, " (%s)", decision.Reason)
		}
		b.WriteString("\n")
	}
}

// writeSandbox 渲染沙箱摘要。
func writeSandbox(b *strings.Builder, summary review.SandboxSummary) {
	if len(summary.Runs) == 0 {
		return
	}
	b.WriteString("\n## Sandbox\n\n")
	for _, run := range summary.Runs {
		fmt.Fprintf(b, "- %s via %s: %s, timeout_ms=%d, output_limit_bytes=%d, duration_ms=%d\n",
			run.Command, run.Runtime, run.Status, run.TimeoutMS, run.OutputLimitBytes, run.DurationMS)
	}
}

// writeArtifacts 渲染产物摘要。
func writeArtifacts(b *strings.Builder, artifacts []review.ArtifactSummary) {
	if len(artifacts) == 0 {
		return
	}
	b.WriteString("\n## Artifacts\n\n")
	for _, artifact := range artifacts {
		fmt.Fprintf(b, "- %s (%s)", artifact.Name, artifact.Kind)
		if artifact.Path != "" {
			fmt.Fprintf(b, ": %s", artifact.Path)
		}
		b.WriteString("\n")
	}
}
