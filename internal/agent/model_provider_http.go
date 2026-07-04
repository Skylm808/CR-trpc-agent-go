package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Skylm808/CR-trpc-agent-go/internal/review"
)

// 本文件只放 generic HTTP provider。它用于接临时网关或测试服务；
// 官方 OpenAI-compatible / DeepSeek 路线见 model_provider_openai.go。

const defaultModelHTTPTimeout = 30 * time.Second

// HTTPModelProviderConfig 控制显式开启的 generic HTTP model provider。
type HTTPModelProviderConfig struct {
	Enabled   bool
	Endpoint  string
	APIKeyEnv string
	Model     string
	Timeout   time.Duration
	Client    *http.Client
}

type httpModelProvider struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

type httpModelReviewRequest struct {
	Model string           `json:"model,omitempty"`
	Input ModelReviewInput `json:"input"`
}

func newHTTPModelProvider(cfg HTTPModelProviderConfig) (ModelReviewProvider, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("model http endpoint is required when model provider is enabled")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultModelHTTPTimeout
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	} else if client.Timeout == 0 {
		clone := *client
		clone.Timeout = timeout
		client = &clone
	}
	apiKey := ""
	if envName := strings.TrimSpace(cfg.APIKeyEnv); envName != "" {
		apiKey = os.Getenv(envName)
	}
	return &httpModelProvider{
		endpoint: endpoint,
		apiKey:   apiKey,
		model:    strings.TrimSpace(cfg.Model),
		client:   client,
	}, nil
}

func (p *httpModelProvider) Review(ctx context.Context, input ModelReviewInput) (ModelReviewOutput, error) {
	payload := httpModelReviewRequest{
		Model: p.model,
		Input: sanitizeModelReviewInput(input),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ModelReviewOutput{}, fmt.Errorf("marshal model request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return ModelReviewOutput{}, fmt.Errorf("create model request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return ModelReviewOutput{}, fmt.Errorf("call model provider: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, int64(defaultOutputLimitBytes)))
	if err != nil {
		return ModelReviewOutput{}, fmt.Errorf("read model response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ModelReviewOutput{}, fmt.Errorf("model provider returned %d: %s", resp.StatusCode, review.RedactSecrets(string(responseBody)))
	}
	var output ModelReviewOutput
	if err := json.Unmarshal(responseBody, &output); err != nil {
		return ModelReviewOutput{}, fmt.Errorf("decode model response: %w", err)
	}
	for i := range output.Findings {
		output.Findings[i] = sanitizeFinding(output.Findings[i])
	}
	return output, nil
}

func sanitizeModelReviewInput(input ModelReviewInput) ModelReviewInput {
	input.DiffSummary = review.RedactSecrets(input.DiffSummary)
	input.ExistingFindings = sanitizedFindingSnapshot(input.ExistingFindings, nil)
	return input
}
