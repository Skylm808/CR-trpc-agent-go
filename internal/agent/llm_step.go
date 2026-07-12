package agent

import (
	"context"

	"github.com/Skylm808/CR-trpc-agent-go/internal/llm"
	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// configuredModelProvider 为本次运行选择可选 LLM 审查边界。
func (a *Agent) configuredModelProvider(enabled bool) (llm.Provider, llm.Audit) {
	return llm.ConfiguredProvider(llm.ProviderSelectionConfig{
		Enabled: enabled,
		Custom:  a.modelProvider,
		HTTP:    a.cfg.ModelHTTP,
		OpenAI:  a.cfg.ModelOpenAI,
	})
}

// runModelReview 向配置的 Provider 请求增量语义 findings。
func (a *Agent) runModelReview(ctx context.Context, taskID string, provider llm.Provider, audit llm.Audit, result review.Result, diff []byte, inputMeta review.InputMetadata) (review.Result, llm.RunSummary) {
	return llm.RunReview(ctx, taskID, provider, audit, result, diff, inputMeta)
}
