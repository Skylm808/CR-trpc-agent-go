package agent

import (
	"context"

	"github.com/Skylm808/CR-trpc-agent-go/internal/llm"
	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// configuredModelProvider selects the optional LLM review boundary for this run.
func (a *Agent) configuredModelProvider(mode string) (llm.Provider, llm.Audit) {
	return llm.ConfiguredProvider(llm.ProviderSelectionConfig{
		ModeFakeModel: ModeFakeModel,
		Mode:          mode,
		Custom:        a.modelProvider,
		HTTP:          a.cfg.ModelHTTP,
		OpenAI:        a.cfg.ModelOpenAI,
	})
}

// runModelReview asks the configured provider for incremental semantic findings.
func (a *Agent) runModelReview(ctx context.Context, taskID string, provider llm.Provider, audit llm.Audit, result review.Result, diff []byte, inputMeta review.InputMetadata) (review.Result, llm.RunSummary) {
	return llm.RunReview(ctx, taskID, provider, audit, result, diff, inputMeta)
}
