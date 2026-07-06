package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// 本文件只放“模型审查业务边界”：输入/输出结构、默认 fake provider、
// provider 选择、模型结果合并、去重、降级和失败兜底。
// 具体外部 provider 实现在 model_provider_*.go，避免主流程混进网络细节。

const (
	modelSourceFake = "fake_model"
	modelSourceReal = "model"

	modelProviderAuditFake   = "fake"
	modelProviderAuditCustom = "custom"
	modelProviderAuditHTTP   = "http"
	modelBackendOfficial     = "trpc-agent-go/model.Model"
	modelBackendHTTP         = "http"
	modelBackendOpenAI       = "trpc-agent-go/model/openai"
)

// ModelReviewProvider 是语义审查 provider 的最小边界。
// 输入只能使用脱敏后的 diff/metadata/已有 finding；输出必须复用 review.Finding。
type ModelReviewProvider interface {
	Review(context.Context, ModelReviewInput) (ModelReviewOutput, error)
}

// ModelReviewInput 是进入模型前的脱敏 prompt payload。
type ModelReviewInput struct {
	DiffSummary       string                   `json:"diff_summary"`
	InputMetadata     review.InputMetadata     `json:"input_metadata"`
	ExistingFindings  []review.Finding         `json:"existing_findings"`
	SandboxSummary    review.SandboxSummary    `json:"sandbox_summary"`
	GovernanceSummary review.GovernanceSummary `json:"governance_summary"`
}

// ModelReviewOutput 是 provider 返回的增量审查结果。
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
	Provider       string
	Name           string
	Backend        string
}

type modelAudit struct {
	Provider string
	Name     string
	Backend  string
}

// fakeModelProvider 给测试和 fake-model 模式提供无网络、无 API Key 的确定性模型路径。
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
				signal := fakeModelSignalForLine(line.Text)
				findings = append(findings, review.Finding{
					Severity:       signal.Severity,
					Category:       signal.Category,
					File:           file.Path,
					Line:           line.NewLine,
					Title:          signal.Title,
					Evidence:       strings.TrimSpace(line.Text),
					Recommendation: signal.Recommendation,
					Confidence:     confidence,
					Source:         modelSourceFake,
					RuleID:         signal.RuleID,
				})
			}
		}
	}
	return ModelReviewOutput{Findings: findings}, nil
}

type fakeModelSignal struct {
	RuleID         string
	Severity       string
	Category       string
	Title          string
	Recommendation string
}

func fakeModelSignalForLine(text string) fakeModelSignal {
	for marker, signal := range map[string]fakeModelSignal{
		"CR_AGENT_FAKE_MODEL_AUTHZ_BYPASS": {
			RuleID:         "fake-model-authz-bypass",
			Severity:       "high",
			Category:       "authorization",
			Title:          "Semantic authorization bypass risk",
			Recommendation: "Require explicit authorization checks for the newly allowed branch.",
		},
		"CR_AGENT_FAKE_MODEL_NIL_BOUNDARY": {
			RuleID:         "fake-model-nil-boundary",
			Severity:       "medium",
			Category:       "boundary",
			Title:          "Nil or zero-value boundary changes behavior",
			Recommendation: "Add explicit handling and tests for nil or zero-value input.",
		},
		"CR_AGENT_FAKE_MODEL_STATE_INCONSISTENCY": {
			RuleID:         "fake-model-state-inconsistency",
			Severity:       "medium",
			Category:       "state",
			Title:          "Cross-function state transition is inconsistent",
			Recommendation: "Keep state transitions and persisted status values aligned.",
		},
		"CR_AGENT_FAKE_MODEL_TRANSACTION_SEMANTIC": {
			RuleID:         "fake-model-transaction-semantic",
			Severity:       "high",
			Category:       "database",
			Title:          "Transaction semantics can commit a failed operation",
			Recommendation: "Rollback on semantic failure paths before returning success or committing.",
		},
		"CR_AGENT_FAKE_MODEL_ERROR_SWALLOW": {
			RuleID:         "fake-model-error-swallow",
			Severity:       "high",
			Category:       "error_handling",
			Title:          "Error is swallowed and reported as success",
			Recommendation: "Propagate or handle the error instead of returning a successful result.",
		},
	} {
		if strings.Contains(text, marker) {
			return signal
		}
	}
	return fakeModelSignal{
		RuleID:         "fake-model-semantic-risk",
		Severity:       "medium",
		Category:       "logic",
		Title:          "Fake model semantic review signal",
		Recommendation: "Inspect the semantic risk before merging.",
	}
}

func (a *Agent) configuredModelProvider(mode string) (ModelReviewProvider, modelAudit) {
	if mode != ModeFakeModel {
		return nil, modelAudit{}
	}
	if a.cfg.ModelOpenAI.Enabled {
		audit := openAIModelAudit(a.cfg.ModelOpenAI)
		provider, err := newOpenAIReviewProvider(a.cfg.ModelOpenAI)
		if err != nil {
			return modelProviderFunc(func(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
				_ = ctx
				_ = input
				return ModelReviewOutput{}, err
			}), audit
		}
		return provider, audit
	}
	if a.cfg.ModelHTTP.Enabled {
		audit := modelAudit{
			Provider: modelProviderAuditHTTP,
			Name:     modelProviderName(a.cfg.ModelHTTP.Model),
			Backend:  modelBackendHTTP,
		}
		provider, err := newHTTPModelProvider(a.cfg.ModelHTTP)
		if err != nil {
			return modelProviderFunc(func(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
				_ = ctx
				_ = input
				return ModelReviewOutput{}, err
			}), audit
		}
		return providerThroughOfficialModel(modelProviderName(a.cfg.ModelHTTP.Model), provider), audit
	}
	if a.modelProvider != nil {
		return providerThroughOfficialModel(defaultModelAdapterName, a.modelProvider), modelAudit{
			Provider: modelProviderAuditCustom,
			Name:     defaultModelAdapterName,
			Backend:  modelBackendOfficial,
		}
	}
	return providerThroughOfficialModel(modelSourceFake, fakeModelProvider{}), modelAudit{
		Provider: modelProviderAuditFake,
		Name:     modelSourceFake,
		Backend:  modelBackendOfficial,
	}
}

func modelProviderName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultModelAdapterName
	}
	return name
}

func (a *Agent) runModelReview(ctx context.Context, taskID string, provider ModelReviewProvider, audit modelAudit, result review.Result, diff []byte, inputMeta review.InputMetadata) (review.Result, modelRunSummary) {
	summary := modelRunSummary{
		CallCount: 1,
		Provider:  audit.Provider,
		Name:      audit.Name,
		Backend:   audit.Backend,
	}
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
