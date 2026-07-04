package agent

import (
	"testing"

	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
	officialopenai "trpc.group/trpc-go/trpc-agent-go/model/openai"
)

func TestOpenAIModelProviderBuildsOfficialDeepSeekModel(t *testing.T) {
	t.Setenv("CR_AGENT_TEST_DEEPSEEK_KEY", "test-deepseek-key")

	model, err := newOpenAIModel(OpenAIModelProviderConfig{
		Provider:  "deepseek",
		Model:     "deepseek-chat",
		APIKeyEnv: "CR_AGENT_TEST_DEEPSEEK_KEY",
	})
	if err != nil {
		t.Fatalf("newOpenAIModel returned error: %v", err)
	}
	var _ agentmodel.Model = model
	if _, ok := model.(*officialopenai.Model); !ok {
		t.Fatalf("expected official trpc-agent-go/model/openai model, got %T", model)
	}
	if model.Info().Name != "deepseek-chat" {
		t.Fatalf("expected model name deepseek-chat, got %q", model.Info().Name)
	}
}

func TestOpenAIModelProviderRequiresAPIKeyBeforeNetworkCall(t *testing.T) {
	_, err := newOpenAIReviewProvider(OpenAIModelProviderConfig{
		Provider:  "deepseek",
		Model:     "deepseek-chat",
		APIKeyEnv: "CR_AGENT_TEST_MISSING_DEEPSEEK_KEY",
	})
	if err == nil {
		t.Fatal("expected missing API key error")
	}
}
