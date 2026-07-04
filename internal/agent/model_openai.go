package agent

import (
	"errors"
	"fmt"
	"os"
	"strings"

	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
	officialopenai "trpc.group/trpc-go/trpc-agent-go/model/openai"
)

const (
	modelProviderOpenAI           = "openai"
	modelProviderOpenAICompatible = "openai-compatible"
	modelProviderDeepSeek         = "deepseek"
	defaultOpenAIAPIKeyEnv        = "OPENAI_API_KEY"
	defaultDeepSeekAPIKeyEnv      = "DEEPSEEK_API_KEY"
	defaultDeepSeekModel          = "deepseek-chat"
)

// OpenAIModelProviderConfig controls the opt-in official OpenAI-compatible model provider.
type OpenAIModelProviderConfig struct {
	Enabled   bool
	Provider  string
	Model     string
	APIKeyEnv string
	BaseURL   string
	Variant   string
}

func newOpenAIReviewProvider(cfg OpenAIModelProviderConfig) (ModelReviewProvider, error) {
	model, err := newOpenAIModel(cfg)
	if err != nil {
		return nil, err
	}
	return modelBackedReviewProvider{model: model}, nil
}

func newOpenAIModel(cfg OpenAIModelProviderConfig) (agentmodel.Model, error) {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = modelProviderOpenAI
	}
	modelName := strings.TrimSpace(cfg.Model)
	if modelName == "" && provider == modelProviderDeepSeek {
		modelName = defaultDeepSeekModel
	}
	if modelName == "" {
		return nil, fmt.Errorf("model name is required for %s provider", provider)
	}
	apiKeyEnv := modelAPIKeyEnv(cfg)
	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf("model provider %s requires API key", provider)
	}
	var opts []officialopenai.Option
	opts = append(opts, officialopenai.WithAPIKey(apiKey))
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		opts = append(opts, officialopenai.WithBaseURL(baseURL))
	}
	variant, err := openAIModelVariant(cfg)
	if err != nil {
		return nil, err
	}
	if variant != "" {
		opts = append(opts, officialopenai.WithVariant(variant))
	}
	return officialopenai.New(modelName, opts...), nil
}

func modelAPIKeyEnv(cfg OpenAIModelProviderConfig) string {
	if envName := strings.TrimSpace(cfg.APIKeyEnv); envName != "" {
		return envName
	}
	switch strings.TrimSpace(cfg.Provider) {
	case modelProviderDeepSeek:
		return defaultDeepSeekAPIKeyEnv
	default:
		return defaultOpenAIAPIKeyEnv
	}
}

func openAIModelVariant(cfg OpenAIModelProviderConfig) (officialopenai.Variant, error) {
	variant := strings.TrimSpace(cfg.Variant)
	if variant == "" {
		switch strings.TrimSpace(cfg.Provider) {
		case modelProviderDeepSeek:
			variant = string(officialopenai.VariantDeepSeek)
		case modelProviderOpenAI, modelProviderOpenAICompatible, "":
			variant = string(officialopenai.VariantOpenAI)
		default:
			return "", fmt.Errorf("unsupported OpenAI-compatible provider %q", cfg.Provider)
		}
	}
	switch variant {
	case string(officialopenai.VariantOpenAI):
		return officialopenai.VariantOpenAI, nil
	case string(officialopenai.VariantDeepSeek):
		return officialopenai.VariantDeepSeek, nil
	default:
		return "", errors.New("unsupported model variant")
	}
}
