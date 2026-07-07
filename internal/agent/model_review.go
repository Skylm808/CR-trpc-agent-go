package agent

import (
	"context"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	"github.com/Skylm808/CR-trpc-agent-go/internal/semantics"
	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
	officialopenai "trpc.group/trpc-go/trpc-agent-go/model/openai"
)

// This file is the agent-facing compatibility facade for semantic model review.
// Provider implementation, official model adaptation, fake markers, HTTP/OpenAI
// providers, and merge policy live in internal/semantics.

const (
	modelSourceFake = semantics.SourceFake
	modelSourceReal = semantics.SourceReal

	modelProviderAuditFake   = semantics.ProviderAuditFake
	modelProviderAuditCustom = semantics.ProviderAuditCustom
	modelProviderAuditHTTP   = semantics.ProviderAuditHTTP
	modelBackendOfficial     = semantics.BackendOfficial
	modelBackendHTTP         = semantics.BackendHTTP
	modelBackendOpenAI       = semantics.BackendOpenAI

	defaultModelAdapterName       = semantics.DefaultModelAdapterName
	modelProviderOpenAI           = semantics.ProviderOpenAI
	modelProviderOpenAICompatible = semantics.ProviderOpenAICompatible
	modelProviderDeepSeek         = semantics.ProviderDeepSeek
	defaultOpenAIAPIKeyEnv        = semantics.DefaultOpenAIAPIKeyEnv
	defaultDeepSeekAPIKeyEnv      = semantics.DefaultDeepSeekAPIKeyEnv
	defaultDeepSeekModel          = semantics.DefaultDeepSeekModel
)

type ModelReviewProvider = semantics.Provider
type ModelReviewInput = semantics.Input
type ModelReviewOutput = semantics.Output
type HTTPModelProviderConfig = semantics.HTTPConfig
type OpenAIModelProviderConfig = semantics.OpenAIConfig
type httpModelReviewRequest = semantics.HTTPReviewRequest
type modelRunSummary = semantics.RunSummary
type modelAudit = semantics.Audit
type officialModelReviewProvider = semantics.OfficialProvider

type modelProviderFunc func(context.Context, ModelReviewInput) (ModelReviewOutput, error)

func (f modelProviderFunc) Review(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
	return f(ctx, input)
}

type reviewProviderModelAdapter struct {
	name     string
	provider ModelReviewProvider
}

func (m reviewProviderModelAdapter) GenerateContent(ctx context.Context, req *agentmodel.Request) (<-chan *agentmodel.Response, error) {
	return semantics.ProviderModelAdapter{
		Name:     m.name,
		Provider: m.provider,
	}.GenerateContent(ctx, req)
}

func (m reviewProviderModelAdapter) Info() agentmodel.Info {
	return semantics.ProviderModelAdapter{
		Name:     m.name,
		Provider: m.provider,
	}.Info()
}

func (a *Agent) configuredModelProvider(mode string) (ModelReviewProvider, modelAudit) {
	return semantics.ConfiguredProvider(semantics.ProviderSelectionConfig{
		ModeFakeModel: ModeFakeModel,
		Mode:          mode,
		Custom:        a.modelProvider,
		HTTP:          a.cfg.ModelHTTP,
		OpenAI:        a.cfg.ModelOpenAI,
	})
}

func (a *Agent) runModelReview(ctx context.Context, taskID string, provider ModelReviewProvider, audit modelAudit, result review.Result, diff []byte, inputMeta review.InputMetadata) (review.Result, modelRunSummary) {
	return semantics.RunReview(ctx, taskID, provider, audit, result, diff, inputMeta)
}

func providerThroughOfficialModel(name string, provider ModelReviewProvider) ModelReviewProvider {
	return semantics.ProviderThroughOfficialModel(name, provider)
}

func modelProviderName(name string) string {
	return semantics.ProviderName(name)
}

func sanitizedFindingSnapshot(findings, warnings []review.Finding) []review.Finding {
	return semantics.SanitizedFindingSnapshot(findings, warnings)
}

func mergeModelFindings(result review.Result, modelFindings []review.Finding) review.Result {
	return semantics.MergeFindings(result, modelFindings)
}

func normalizeModelFinding(f review.Finding) review.Finding {
	return semantics.NormalizeFinding(f)
}

func normalizeModelSource(source string) string {
	return semantics.NormalizeSource(source)
}

func resultWithModelError(result review.Result, taskID string, err error) review.Result {
	return semantics.ResultWithModelError(result, taskID, err)
}

func countModelSourceFindings(findings []review.Finding) int {
	return semantics.CountModelSourceFindings(findings)
}

func modelReviewInputRequest(input ModelReviewInput) *agentmodel.Request {
	return semantics.InputRequest(input)
}

func modelReviewSystemPrompt() string {
	return semantics.SystemPrompt()
}

func decodeModelReviewOutput(content string) (ModelReviewOutput, error) {
	return semantics.DecodeOutput(content)
}

func modelReviewInputFromRequest(req *agentmodel.Request) (ModelReviewInput, error) {
	return semantics.InputFromRequest(req)
}

func sanitizeModelReviewInput(input ModelReviewInput) ModelReviewInput {
	return semantics.SanitizeInput(input)
}

func newHTTPModelProvider(cfg HTTPModelProviderConfig) (ModelReviewProvider, error) {
	return semantics.NewHTTPProvider(cfg)
}

func newOpenAIReviewProvider(cfg OpenAIModelProviderConfig) (ModelReviewProvider, error) {
	return semantics.NewOpenAIReviewProvider(cfg)
}

func openAIModelAudit(cfg OpenAIModelProviderConfig) modelAudit {
	return semantics.OpenAIModelAudit(cfg)
}

func newOpenAIModel(cfg OpenAIModelProviderConfig) (agentmodel.Model, error) {
	return semantics.NewOpenAIModel(cfg)
}

func openAIModelBaseURL(cfg OpenAIModelProviderConfig) string {
	return semantics.OpenAIModelBaseURL(cfg)
}

func modelAPIKeyEnv(cfg OpenAIModelProviderConfig) string {
	return semantics.ModelAPIKeyEnv(cfg)
}

func modelAPIKey(cfg OpenAIModelProviderConfig, envName string) string {
	return semantics.ModelAPIKey(cfg, envName)
}

func openAIModelVariant(cfg OpenAIModelProviderConfig) (officialopenai.Variant, error) {
	return semantics.OpenAIModelVariant(cfg)
}
