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

const defaultOfficialModelName = "cr-agent-review-provider"

// modelReviewProviderModel adapts the local review provider boundary to the official model.Model interface.
type modelReviewProviderModel struct {
	name     string
	provider ModelReviewProvider
}

func (m modelReviewProviderModel) GenerateContent(ctx context.Context, req *agentmodel.Request) (<-chan *agentmodel.Response, error) {
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

func (m modelReviewProviderModel) Info() agentmodel.Info {
	name := strings.TrimSpace(m.name)
	if name == "" {
		name = defaultOfficialModelName
	}
	return agentmodel.Info{Name: name}
}

// modelBackedReviewProvider calls the official model.Model route and maps its structured JSON response back.
type modelBackedReviewProvider struct {
	model agentmodel.Model
}

func (p modelBackedReviewProvider) Review(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
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

func officialModelBackedProvider(name string, provider ModelReviewProvider) ModelReviewProvider {
	return modelBackedReviewProvider{
		model: modelReviewProviderModel{
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
