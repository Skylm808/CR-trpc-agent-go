package agent

import (
	"context"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/reviewmodel"
	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
	officialopenai "trpc.group/trpc-go/trpc-agent-go/model/openai"
)

// This file is the agent-facing compatibility facade for semantic model review.
// Provider implementation, official model adaptation, fake markers, HTTP/OpenAI
// providers, and merge policy live in internal/reviewmodel.

const (
	modelSourceFake = reviewmodel.SourceFake
	modelSourceReal = reviewmodel.SourceReal

	modelProviderAuditFake   = reviewmodel.ProviderAuditFake
	modelProviderAuditCustom = reviewmodel.ProviderAuditCustom
	modelProviderAuditHTTP   = reviewmodel.ProviderAuditHTTP
	modelBackendOfficial     = reviewmodel.BackendOfficial
	modelBackendHTTP         = reviewmodel.BackendHTTP
	modelBackendOpenAI       = reviewmodel.BackendOpenAI

	defaultModelAdapterName       = reviewmodel.DefaultModelAdapterName
	modelProviderOpenAI           = reviewmodel.ProviderOpenAI
	modelProviderOpenAICompatible = reviewmodel.ProviderOpenAICompatible
	modelProviderDeepSeek         = reviewmodel.ProviderDeepSeek
	defaultOpenAIAPIKeyEnv        = reviewmodel.DefaultOpenAIAPIKeyEnv
	defaultDeepSeekAPIKeyEnv      = reviewmodel.DefaultDeepSeekAPIKeyEnv
	defaultDeepSeekModel          = reviewmodel.DefaultDeepSeekModel
)

type ModelReviewProvider = reviewmodel.Provider
type ModelReviewInput = reviewmodel.Input
type ModelReviewOutput = reviewmodel.Output
type HTTPModelProviderConfig = reviewmodel.HTTPConfig
type OpenAIModelProviderConfig = reviewmodel.OpenAIConfig
type httpModelReviewRequest = reviewmodel.HTTPReviewRequest
type modelRunSummary = reviewmodel.RunSummary
type modelAudit = reviewmodel.Audit
type officialModelReviewProvider = reviewmodel.OfficialProvider

type modelProviderFunc func(context.Context, ModelReviewInput) (ModelReviewOutput, error)

func (f modelProviderFunc) Review(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
	return f(ctx, input)
}

type reviewProviderModelAdapter struct {
	name     string
	provider ModelReviewProvider
}

func (m reviewProviderModelAdapter) GenerateContent(ctx context.Context, req *agentmodel.Request) (<-chan *agentmodel.Response, error) {
	return reviewmodel.ProviderModelAdapter{
		Name:     m.name,
		Provider: m.provider,
	}.GenerateContent(ctx, req)
}

func (m reviewProviderModelAdapter) Info() agentmodel.Info {
	return reviewmodel.ProviderModelAdapter{
		Name:     m.name,
		Provider: m.provider,
	}.Info()
}

func (a *Agent) configuredModelProvider(mode string) (ModelReviewProvider, modelAudit) {
	return reviewmodel.ConfiguredProvider(reviewmodel.ProviderSelectionConfig{
		ModeFakeModel: ModeFakeModel,
		Mode:          mode,
		Custom:        a.modelProvider,
		HTTP:          a.cfg.ModelHTTP,
		OpenAI:        a.cfg.ModelOpenAI,
	})
}

func (a *Agent) runModelReview(ctx context.Context, taskID string, provider ModelReviewProvider, audit modelAudit, result review.Result, diff []byte, inputMeta review.InputMetadata) (review.Result, modelRunSummary) {
	return reviewmodel.RunReview(ctx, taskID, provider, audit, result, diff, inputMeta)
}

func providerThroughOfficialModel(name string, provider ModelReviewProvider) ModelReviewProvider {
	return reviewmodel.ProviderThroughOfficialModel(name, provider)
}

func modelProviderName(name string) string {
	return reviewmodel.ProviderName(name)
}

func sanitizedFindingSnapshot(findings, warnings []review.Finding) []review.Finding {
	return reviewmodel.SanitizedFindingSnapshot(findings, warnings)
}

func mergeModelFindings(result review.Result, modelFindings []review.Finding) review.Result {
	return reviewmodel.MergeFindings(result, modelFindings)
}

func normalizeModelFinding(f review.Finding) review.Finding {
	return reviewmodel.NormalizeFinding(f)
}

func normalizeModelSource(source string) string {
	return reviewmodel.NormalizeSource(source)
}

func resultWithModelError(result review.Result, taskID string, err error) review.Result {
	return reviewmodel.ResultWithModelError(result, taskID, err)
}

func countModelSourceFindings(findings []review.Finding) int {
	return reviewmodel.CountModelSourceFindings(findings)
}

func modelReviewInputRequest(input ModelReviewInput) *agentmodel.Request {
	return reviewmodel.InputRequest(input)
}

func modelReviewSystemPrompt() string {
	return reviewmodel.SystemPrompt()
}

func decodeModelReviewOutput(content string) (ModelReviewOutput, error) {
	return reviewmodel.DecodeOutput(content)
}

func modelReviewInputFromRequest(req *agentmodel.Request) (ModelReviewInput, error) {
	return reviewmodel.InputFromRequest(req)
}

func sanitizeModelReviewInput(input ModelReviewInput) ModelReviewInput {
	return reviewmodel.SanitizeInput(input)
}

func newHTTPModelProvider(cfg HTTPModelProviderConfig) (ModelReviewProvider, error) {
	return reviewmodel.NewHTTPProvider(cfg)
}

func newOpenAIReviewProvider(cfg OpenAIModelProviderConfig) (ModelReviewProvider, error) {
	return reviewmodel.NewOpenAIReviewProvider(cfg)
}

func openAIModelAudit(cfg OpenAIModelProviderConfig) modelAudit {
	return reviewmodel.OpenAIModelAudit(cfg)
}

func newOpenAIModel(cfg OpenAIModelProviderConfig) (agentmodel.Model, error) {
	return reviewmodel.NewOpenAIModel(cfg)
}

func openAIModelBaseURL(cfg OpenAIModelProviderConfig) string {
	return reviewmodel.OpenAIModelBaseURL(cfg)
}

func modelAPIKeyEnv(cfg OpenAIModelProviderConfig) string {
	return reviewmodel.ModelAPIKeyEnv(cfg)
}

func modelAPIKey(cfg OpenAIModelProviderConfig, envName string) string {
	return reviewmodel.ModelAPIKey(cfg, envName)
}

func openAIModelVariant(cfg OpenAIModelProviderConfig) (officialopenai.Variant, error) {
	return reviewmodel.OpenAIModelVariant(cfg)
}
