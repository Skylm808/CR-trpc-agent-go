package agent

import (
	"context"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/storage"
)

// persist 保存审计和报告数据。
func (a *Agent) persist(ctx context.Context, taskID string, result review.Result, decisions []storage.DecisionRecord, runs []storage.SandboxRunRecord, jsonReport, markdownReport []byte) error {
	// 保存权限决策。
	for _, decision := range decisions {
		if decision.Command == "" && decision.Action == "" {
			continue
		}
		if err := a.store.SaveDecision(ctx, decision); err != nil {
			return err
		}
	}
	if result.Metrics.RedactionCount > 0 {
		// 有脱敏就记录过滤决策。
		if err := a.store.SaveFilterDecision(ctx, storage.FilterDecisionRecord{
			TaskID: taskID,
			Target: "finding.evidence",
			Action: "redact",
			Reason: "secret pattern",
			At:     time.Now(),
		}); err != nil {
			return err
		}
	}
	// 保存沙箱摘要。
	for _, run := range runs {
		if run.Command == "" && run.Status == "" {
			continue
		}
		if err := a.store.SaveSandboxRun(ctx, run); err != nil {
			return err
		}
	}
	// 审查项统一进入 findings 表。
	for _, finding := range persistedReviewItems(result) {
		if err := a.store.SaveFinding(ctx, taskID, finding); err != nil {
			return err
		}
	}
	// 保存聚合指标。
	if err := a.store.SaveMetrics(ctx, storage.MetricsRecord{
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
	// 保存产物引用。
	for _, artifact := range result.Artifacts {
		digest := artifact.Digest
		if artifact.Name == "review_report.json" {
			digest = digestBytes(jsonReport)
		}
		if artifact.Name == "review_report.md" {
			digest = digestBytes(markdownReport)
		}
		if err := a.store.SaveArtifact(ctx, storage.ArtifactRecord{
			TaskID: taskID,
			Name:   artifact.Name,
			Kind:   artifact.Kind,
			Path:   artifact.Path,
			Digest: digest,
			At:     time.Now(),
		}); err != nil {
			return err
		}
	}
	// 保存最终报告。
	return a.store.SaveReport(ctx, taskID, jsonReport, markdownReport)
}

// persistedReviewItems 返回需要落库的审查项。
func persistedReviewItems(result review.Result) []review.Finding {
	// 用 status 区分 finding、warning 和复核项。
	items := make([]review.Finding, 0, len(result.Findings)+len(result.Warnings)+len(result.HumanReviewItems))
	items = append(items, result.Findings...)
	items = append(items, result.Warnings...)
	items = append(items, result.HumanReviewItems...)
	return review.DedupeFindings(items)
}
