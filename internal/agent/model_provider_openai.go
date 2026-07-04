package agent

import (
	"errors"
	"fmt"
	"os"
	"strings"

	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
	officialopenai "trpc.group/trpc-go/trpc-agent-go/model/openai"
)

// 本文件只放官方 trpc-agent-go/model/openai provider 配置。
// OpenAI、OpenAI-compatible 中转站和 DeepSeek 都从这里进入，不再新增单独厂商 SDK。

const (
	modelProviderOpenAI           = "openai"
	modelProviderOpenAICompatible = "openai-compatible"
	modelProviderDeepSeek         = "deepseek"
	defaultOpenAIAPIKeyEnv        = "OPENAI_API_KEY"
	defaultDeepSeekAPIKeyEnv      = "DEEPSEEK_API_KEY"
	defaultDeepSeekModel          = "deepseek-chat"
)

// OpenAIModelProviderConfig 控制显式开启的官方 OpenAI-compatible provider。
// DeepSeek 也走 trpc-agent-go/model/openai 的 VariantDeepSeek，不引入单独 DeepSeek SDK。
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
	return officialModelReviewProvider{model: model}, nil
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
	if baseURL := openAIModelBaseURL(cfg); baseURL != "" {
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

func openAIModelBaseURL(cfg OpenAIModelProviderConfig) string {
	// base_url 可由 YAML/CLI 固定；为空时兼容官方 examples 常用的 OPENAI_BASE_URL。
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		return baseURL
	}
	switch strings.TrimSpace(cfg.Provider) {
	case modelProviderDeepSeek:
		return ""
	}
	return strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
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
