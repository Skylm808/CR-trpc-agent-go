package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

const (
	modelSourceFake = "fake_model"
	modelSourceReal = "model"
)

// ModelReviewProvider is the narrow boundary for semantic review providers.
type ModelReviewProvider interface {
	Review(context.Context, ModelReviewInput) (ModelReviewOutput, error)
}

// ModelReviewInput is the sanitized prompt payload for a model review.
type ModelReviewInput struct {
	DiffSummary       string                   `json:"diff_summary"`
	InputMetadata     review.InputMetadata     `json:"input_metadata"`
	ExistingFindings  []review.Finding         `json:"existing_findings"`
	SandboxSummary    review.SandboxSummary    `json:"sandbox_summary"`
	GovernanceSummary review.GovernanceSummary `json:"governance_summary"`
}

// ModelReviewOutput is the provider's incremental review result.
type ModelReviewOutput struct {
	Findings []review.Finding `json:"findings"`
}

type modelProviderFunc func(context.Context, ModelReviewInput) (ModelReviewOutput, error)

func (f modelProviderFunc) Review(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
	return f(ctx, input)
}

type modelRunSummary struct {
	CallCount      int
	FindingCount   int
	DurationMS     int64
	ExceptionCount int
}

// fakeModelProvider gives tests and fake-model mode a deterministic model path.
type fakeModelProvider struct{}

func (fakeModelProvider) Review(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
	_ = ctx
	if !strings.Contains(input.DiffSummary, "CR_AGENT_FAKE_MODEL_") {
		return ModelReviewOutput{}, nil
	}
	parsed, err := review.ParseUnifiedDiff(input.DiffSummary)
	if err != nil {
		return ModelReviewOutput{}, err
	}
	var findings []review.Finding
	for _, file := range parsed.Files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if line.Kind != "add" {
					continue
				}
				confidence := ""
				switch {
				case strings.Contains(line.Text, "CR_AGENT_FAKE_MODEL_HIGH"):
					confidence = "high"
				case strings.Contains(line.Text, "CR_AGENT_FAKE_MODEL_LOW"):
					confidence = "low"
				default:
					continue
				}
				findings = append(findings, review.Finding{
					Severity:       "medium",
					Category:       "logic",
					File:           file.Path,
					Line:           line.NewLine,
					Title:          "Fake model semantic review signal",
					Evidence:       strings.TrimSpace(line.Text),
					Recommendation: "Inspect the semantic risk before merging.",
					Confidence:     confidence,
					Source:         modelSourceFake,
					RuleID:         "fake-model-semantic-risk",
				})
			}
		}
	}
	return ModelReviewOutput{Findings: findings}, nil
}

func (a *Agent) configuredModelProvider(mode string) ModelReviewProvider {
	if mode != ModeFakeModel {
		return nil
	}
	if a.cfg.ModelHTTP.Enabled {
		provider, err := newHTTPModelProvider(a.cfg.ModelHTTP)
		if err != nil {
			return modelProviderFunc(func(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
				_ = ctx
				_ = input
				return ModelReviewOutput{}, err
			})
		}
		return provider
	}
	if a.modelProvider != nil {
		return a.modelProvider
	}
	return fakeModelProvider{}
}

func (a *Agent) runModelReview(ctx context.Context, taskID string, provider ModelReviewProvider, result review.Result, diff []byte, inputMeta review.InputMetadata) (review.Result, modelRunSummary) {
	summary := modelRunSummary{CallCount: 1}
	input := ModelReviewInput{
		DiffSummary:       review.RedactSecrets(string(diff)),
		InputMetadata:     inputMeta,
		ExistingFindings:  sanitizedFindingSnapshot(result.Findings, result.Warnings),
		SandboxSummary:    result.SandboxSummary,
		GovernanceSummary: result.GovernanceSummary,
	}
	start := time.Now()
	output, err := provider.Review(ctx, input)
	summary.DurationMS = time.Since(start).Milliseconds()
	if summary.DurationMS == 0 {
		summary.DurationMS = 1
	}
	if err != nil {
		summary.ExceptionCount = 1
		return resultWithModelError(result, taskID, err), summary
	}
	result = mergeModelFindings(result, output.Findings)
	summary.FindingCount = countModelSourceFindings(result.Findings) + countModelSourceFindings(result.Warnings)
	return result, summary
}

func sanitizedFindingSnapshot(findings, warnings []review.Finding) []review.Finding {
	out := make([]review.Finding, 0, len(findings)+len(warnings))
	for _, finding := range append(append([]review.Finding(nil), findings...), warnings...) {
		out = append(out, sanitizeFinding(finding))
	}
	return review.DedupeFindings(out)
}

func mergeModelFindings(result review.Result, modelFindings []review.Finding) review.Result {
	existing := map[string]struct{}{}
	for _, finding := range append(append([]review.Finding(nil), result.Findings...), result.Warnings...) {
		existing[finding.DedupeKey()] = struct{}{}
	}
	for _, finding := range modelFindings {
		finding = normalizeModelFinding(finding)
		if _, ok := existing[finding.DedupeKey()]; ok {
			continue
		}
		existing[finding.DedupeKey()] = struct{}{}
		if strings.EqualFold(strings.TrimSpace(finding.Confidence), "high") {
			finding.Status = "finding"
			result.Findings = append(result.Findings, finding)
			continue
		}
		finding.Status = "needs_human_review"
		result.Warnings = append(result.Warnings, finding)
	}
	result.Findings = review.DedupeFindings(result.Findings)
	result.Warnings = review.DedupeFindings(result.Warnings)
	return result
}

func normalizeModelFinding(f review.Finding) review.Finding {
	f = sanitizeFinding(f)
	f.Source = normalizeModelSource(f.Source)
	if f.Confidence == "" {
		f.Confidence = "low"
	}
	if f.RuleID == "" {
		f.RuleID = "model-review"
	}
	if f.Category == "" {
		f.Category = "model"
	}
	if f.Severity == "" {
		f.Severity = "low"
	}
	if f.Title == "" {
		f.Title = "Model review signal"
	}
	return f
}

func normalizeModelSource(source string) string {
	source = strings.TrimSpace(source)
	switch source {
	case modelSourceFake:
		return modelSourceFake
	default:
		return modelSourceReal
	}
}

func resultWithModelError(result review.Result, taskID string, err error) review.Result {
	if result.Metrics.ExceptionCounts == nil {
		result.Metrics.ExceptionCounts = map[string]int{}
	}
	incrementException(result.Metrics.ExceptionCounts, "model_provider")
	result.Warnings = append(result.Warnings, review.Finding{
		Severity:       "low",
		Category:       "model",
		File:           "",
		Line:           0,
		Title:          "Model review provider failed",
		Evidence:       review.RedactSecrets(fmt.Sprintf("%s: %v", taskID, err)),
		Recommendation: "Ask a human reviewer to inspect semantic and cross-file risks.",
		Confidence:     "high",
		Source:         modelSourceReal,
		RuleID:         "model-provider-failed",
		Status:         "needs_human_review",
	})
	return result
}

func countModelSourceFindings(findings []review.Finding) int {
	count := 0
	for _, finding := range findings {
		if finding.Source == modelSourceReal || finding.Source == modelSourceFake {
			count++
		}
	}
	return count
}
