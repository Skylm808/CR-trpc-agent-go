// Package llm owns model review providers and merge policy.
package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

const (
	SourceFake = "fake_model"
	SourceReal = "model"

	ProviderAuditFake   = "fake"
	ProviderAuditCustom = "custom"
	ProviderAuditHTTP   = "http"
	BackendOfficial     = "trpc-agent-go/model.Model"
	BackendHTTP         = "http"
	BackendOpenAI       = "trpc-agent-go/model/openai"
)

// Provider is the semantic review provider boundary.
type Provider interface {
	Review(context.Context, Input) (Output, error)
}

// Input is the redacted payload sent to semantic review providers.
type Input struct {
	DiffSummary       string                   `json:"diff_summary"`
	InputMetadata     review.InputMetadata     `json:"input_metadata"`
	ExistingFindings  []review.Finding         `json:"existing_findings"`
	SandboxSummary    review.SandboxSummary    `json:"sandbox_summary"`
	GovernanceSummary review.GovernanceSummary `json:"governance_summary"`
}

// Output is the provider's incremental review result.
type Output struct {
	Findings []review.Finding `json:"findings"`
}

// ProviderFunc adapts a function to Provider.
type ProviderFunc func(context.Context, Input) (Output, error)

func (f ProviderFunc) Review(ctx context.Context, input Input) (Output, error) {
	return f(ctx, input)
}

// RunSummary records model review audit metrics.
type RunSummary struct {
	CallCount      int
	FindingCount   int
	DurationMS     int64
	ExceptionCount int
	Provider       string
	Name           string
	Backend        string
}

// Audit records non-sensitive provider identity.
type Audit struct {
	Provider string
	Name     string
	Backend  string
}

// ProviderSelectionConfig selects the provider for fake-model mode.
type ProviderSelectionConfig struct {
	ModeFakeModel string
	Mode          string
	Custom        Provider
	HTTP          HTTPConfig
	OpenAI        OpenAIConfig
}

// ConfiguredProvider chooses the semantic review provider for the active mode.
func ConfiguredProvider(cfg ProviderSelectionConfig) (Provider, Audit) {
	if cfg.Mode != cfg.ModeFakeModel {
		return nil, Audit{}
	}
	if cfg.OpenAI.Enabled {
		audit := OpenAIModelAudit(cfg.OpenAI)
		provider, err := NewOpenAIReviewProvider(cfg.OpenAI)
		if err != nil {
			return ProviderFunc(func(ctx context.Context, input Input) (Output, error) {
				_ = ctx
				_ = input
				return Output{}, err
			}), audit
		}
		return provider, audit
	}
	if cfg.HTTP.Enabled {
		audit := Audit{
			Provider: ProviderAuditHTTP,
			Name:     ProviderName(cfg.HTTP.Model),
			Backend:  BackendHTTP,
		}
		provider, err := NewHTTPProvider(cfg.HTTP)
		if err != nil {
			return ProviderFunc(func(ctx context.Context, input Input) (Output, error) {
				_ = ctx
				_ = input
				return Output{}, err
			}), audit
		}
		return ProviderThroughOfficialModel(ProviderName(cfg.HTTP.Model), provider), audit
	}
	if cfg.Custom != nil {
		return ProviderThroughOfficialModel(DefaultModelAdapterName, cfg.Custom), Audit{
			Provider: ProviderAuditCustom,
			Name:     DefaultModelAdapterName,
			Backend:  BackendOfficial,
		}
	}
	return ProviderThroughOfficialModel(SourceFake, FakeProvider{}), Audit{
		Provider: ProviderAuditFake,
		Name:     SourceFake,
		Backend:  BackendOfficial,
	}
}

func ProviderName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultModelAdapterName
	}
	return name
}

// RunReview calls a semantic provider and merges its incremental findings.
func RunReview(ctx context.Context, taskID string, provider Provider, audit Audit, result review.Result, diff []byte, inputMeta review.InputMetadata) (review.Result, RunSummary) {
	summary := RunSummary{
		CallCount: 1,
		Provider:  audit.Provider,
		Name:      audit.Name,
		Backend:   audit.Backend,
	}
	input := Input{
		DiffSummary:       review.RedactSecrets(string(diff)),
		InputMetadata:     inputMeta,
		ExistingFindings:  SanitizedFindingSnapshot(result.Findings, result.Warnings),
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
		return ResultWithModelError(result, taskID, err), summary
	}
	result = MergeFindings(result, output.Findings)
	summary.FindingCount = CountModelSourceFindings(result.Findings) + CountModelSourceFindings(result.Warnings)
	return result, summary
}

// SanitizedFindingSnapshot returns redacted, deduped existing findings.
func SanitizedFindingSnapshot(findings, warnings []review.Finding) []review.Finding {
	out := make([]review.Finding, 0, len(findings)+len(warnings))
	for _, finding := range append(append([]review.Finding(nil), findings...), warnings...) {
		out = append(out, SanitizeFinding(finding))
	}
	return review.DedupeFindings(out)
}

// MergeFindings merges provider findings without duplicating rule findings.
func MergeFindings(result review.Result, modelFindings []review.Finding) review.Result {
	existing := map[string]struct{}{}
	for _, finding := range append(append([]review.Finding(nil), result.Findings...), result.Warnings...) {
		existing[finding.DedupeKey()] = struct{}{}
	}
	for _, finding := range modelFindings {
		finding = NormalizeFinding(finding)
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

// NormalizeFinding applies defaults and source normalization.
func NormalizeFinding(f review.Finding) review.Finding {
	f = SanitizeFinding(f)
	f.Source = NormalizeSource(f.Source)
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

// SanitizeFinding redacts evidence before provider output enters reports/storage.
func SanitizeFinding(f review.Finding) review.Finding {
	f.Evidence = review.RedactSecrets(f.Evidence)
	if f.Status == "" {
		f.Status = "finding"
	}
	return f
}

func NormalizeSource(source string) string {
	source = strings.TrimSpace(source)
	switch source {
	case SourceFake:
		return SourceFake
	default:
		return SourceReal
	}
}

// ResultWithModelError converts provider failure to human review evidence.
func ResultWithModelError(result review.Result, taskID string, err error) review.Result {
	if result.Metrics.ExceptionCounts == nil {
		result.Metrics.ExceptionCounts = map[string]int{}
	}
	result.Metrics.ExceptionCounts["model_provider"]++
	result.Warnings = append(result.Warnings, review.Finding{
		Severity:       "low",
		Category:       "model",
		Title:          "Model review provider failed",
		Evidence:       review.RedactSecrets(fmt.Sprintf("%s: %v", taskID, err)),
		Recommendation: "Ask a human reviewer to inspect semantic and cross-file risks.",
		Confidence:     "high",
		Source:         SourceReal,
		RuleID:         "model-provider-failed",
		Status:         "needs_human_review",
	})
	return result
}

// CountModelSourceFindings counts fake and real model findings.
func CountModelSourceFindings(findings []review.Finding) int {
	count := 0
	for _, finding := range findings {
		if finding.Source == SourceReal || finding.Source == SourceFake {
			count++
		}
	}
	return count
}

// FakeProvider gives fake-model mode a deterministic no-network provider.
type FakeProvider struct{}

func (FakeProvider) Review(ctx context.Context, input Input) (Output, error) {
	_ = ctx
	if !strings.Contains(input.DiffSummary, "CR_AGENT_FAKE_MODEL_") {
		return Output{}, nil
	}
	parsed, err := review.ParseUnifiedDiff(input.DiffSummary)
	if err != nil {
		return Output{}, err
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
				signal := fakeSignalForLine(line.Text)
				findings = append(findings, review.Finding{
					Severity:       signal.Severity,
					Category:       signal.Category,
					File:           file.Path,
					Line:           line.NewLine,
					Title:          signal.Title,
					Evidence:       strings.TrimSpace(line.Text),
					Recommendation: signal.Recommendation,
					Confidence:     confidence,
					Source:         SourceFake,
					RuleID:         signal.RuleID,
				})
			}
		}
	}
	return Output{Findings: findings}, nil
}

type fakeSignal struct {
	RuleID         string
	Severity       string
	Category       string
	Title          string
	Recommendation string
}

func fakeSignalForLine(text string) fakeSignal {
	for marker, signal := range map[string]fakeSignal{
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
	return fakeSignal{
		RuleID:         "fake-model-semantic-risk",
		Severity:       "medium",
		Category:       "logic",
		Title:          "Fake model semantic review signal",
		Recommendation: "Inspect the semantic risk before merging.",
	}
}
