package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
	agentmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// 本文件只负责桥接：把本项目的 ModelReviewProvider 适配到官方 model.Model，
// 再把官方 model.Model 的结构化响应解析回 ModelReviewOutput。

const defaultModelAdapterName = "cr-agent-review-provider"

// reviewProviderModelAdapter 把本项目的 ModelReviewProvider 包成官方 model.Model。
// 这样 fake provider、HTTP provider 和真实 OpenAI-compatible provider 都走同一条 Runner/Model 边界。
type reviewProviderModelAdapter struct {
	name     string
	provider ModelReviewProvider
}

func (m reviewProviderModelAdapter) GenerateContent(ctx context.Context, req *agentmodel.Request) (<-chan *agentmodel.Response, error) {
	if m.provider == nil {
		return nil, errors.New("model review provider is required")
	}
	input, err := modelReviewInputFromRequest(req)
	if err != nil {
		return nil, err
	}
	output, err := m.provider.Review(ctx, sanitizeModelReviewInput(input))
	if err != nil {
		return nil, err
	}
	for i := range output.Findings {
		output.Findings[i] = sanitizeFinding(output.Findings[i])
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal model review output: %w", err)
	}
	ch := make(chan *agentmodel.Response, 1)
	ch <- &agentmodel.Response{
		Object:  agentmodel.ObjectTypeChatCompletion,
		Created: time.Now().Unix(),
		Model:   m.Info().Name,
		Choices: []agentmodel.Choice{{
			Index: 0,
			Message: agentmodel.Message{
				Role:    agentmodel.RoleAssistant,
				Content: string(payload),
			},
		}},
		Done: true,
	}
	close(ch)
	return ch, nil
}

func (m reviewProviderModelAdapter) Info() agentmodel.Info {
	name := strings.TrimSpace(m.name)
	if name == "" {
		name = defaultModelAdapterName
	}
	return agentmodel.Info{Name: name}
}

// officialModelReviewProvider 调用官方 model.Model，再把结构化 JSON 响应还原成审查增量。
type officialModelReviewProvider struct {
	model agentmodel.Model
}

func (p officialModelReviewProvider) Review(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
	if p.model == nil {
		return ModelReviewOutput{}, errors.New("official model is required")
	}
	responses, err := p.model.GenerateContent(ctx, modelReviewInputRequest(input))
	if err != nil {
		return ModelReviewOutput{}, err
	}
	var content string
	for response := range responses {
		if response == nil {
			continue
		}
		if response.Error != nil {
			return ModelReviewOutput{}, fmt.Errorf("official model response error: %s", review.RedactSecrets(response.Error.Message))
		}
		for _, choice := range response.Choices {
			if strings.TrimSpace(choice.Message.Content) != "" {
				content = choice.Message.Content
			}
			if strings.TrimSpace(choice.Delta.Content) != "" {
				content += choice.Delta.Content
			}
		}
	}
	if strings.TrimSpace(content) == "" {
		return ModelReviewOutput{}, nil
	}
	var output ModelReviewOutput
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return ModelReviewOutput{}, fmt.Errorf("decode official model response: %w", err)
	}
	for i := range output.Findings {
		output.Findings[i] = sanitizeFinding(output.Findings[i])
	}
	return output, nil
}

func providerThroughOfficialModel(name string, provider ModelReviewProvider) ModelReviewProvider {
	return officialModelReviewProvider{
		model: reviewProviderModelAdapter{
			name:     name,
			provider: provider,
		},
	}
}

func modelReviewInputRequest(input ModelReviewInput) *agentmodel.Request {
	payload, _ := json.Marshal(sanitizeModelReviewInput(input))
	return agentmodel.NewRequest([]agentmodel.Message{
		agentmodel.NewSystemMessage("Return a JSON object matching ModelReviewOutput with a findings array."),
		agentmodel.NewUserMessage(string(payload)),
	})
}

func modelReviewInputFromRequest(req *agentmodel.Request) (ModelReviewInput, error) {
	if req == nil {
		return ModelReviewInput{}, errors.New("model request is required")
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role != agentmodel.RoleUser || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		var input ModelReviewInput
		if err := json.Unmarshal([]byte(msg.Content), &input); err != nil {
			return ModelReviewInput{}, fmt.Errorf("decode model review input: %w", err)
		}
		return input, nil
	}
	return ModelReviewInput{}, errors.New("model request has no user input payload")
}
