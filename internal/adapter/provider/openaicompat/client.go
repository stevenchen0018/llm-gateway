// Package openaicompat implements domain.ProviderClient against any vendor
// exposing an OpenAI-compatible HTTP API (OpenAI itself, DeepSeek, Qwen/
// Tongyi's compatible-mode endpoint, most self-hosted vLLM/SGLang
// deployments, and many other OpenAI-like commercial clouds). It is the
// gateway's default adapter; vendor-specific adapters for providers with
// genuinely different wire formats (e.g. Baidu ERNIE's native API) can be
// added as sibling packages implementing the same domain.ProviderClient
// interface without touching gateway core code.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 120 * time.Second}}
}

type chatRequestBody struct {
	Model       string               `json:"model"`
	Messages    []domain.ChatMessage `json:"messages"`
	Temperature *float64             `json:"temperature,omitempty"`
	TopP        *float64             `json:"top_p,omitempty"`
	MaxTokens   *int                 `json:"max_tokens,omitempty"`
}

type chatResponseBody struct {
	ID      string              `json:"id"`
	Model   string              `json:"model"`
	Created int64               `json:"created"`
	Choices []domain.ChatChoice `json:"choices"`
	Usage   domain.Usage        `json:"usage"`
	Error   *upstreamError      `json:"error,omitempty"`
}

type embeddingsRequestBody struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingsResponseBody struct {
	Model string                 `json:"model"`
	Data  []domain.EmbeddingData `json:"data"`
	Usage domain.Usage           `json:"usage"`
	Error *upstreamError         `json:"error,omitempty"`
}

type upstreamError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (c *Client) ChatCompletion(ctx context.Context, target domain.CallTarget, req domain.ChatRequest) (domain.ChatResponse, error) {
	body := chatRequestBody{
		Model: target.Model.ModelKey, Messages: req.Messages,
		Temperature: req.Temperature, TopP: req.TopP, MaxTokens: req.MaxTokens,
	}

	var out chatResponseBody
	if err := c.post(ctx, target.Provider, "/chat/completions", body, &out); err != nil {
		return domain.ChatResponse{}, err
	}
	if out.Error != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: %s", domain.ErrUpstream, out.Error.Message)
	}

	return domain.ChatResponse{
		ID: out.ID, Model: target.Model.ModelKey, Created: out.Created,
		Choices: out.Choices, Usage: out.Usage,
	}, nil
}

func (c *Client) Embeddings(ctx context.Context, target domain.CallTarget, req domain.EmbeddingsRequest) (domain.EmbeddingsResponse, error) {
	body := embeddingsRequestBody{Model: target.Model.ModelKey, Input: req.Input}

	var out embeddingsResponseBody
	if err := c.post(ctx, target.Provider, "/embeddings", body, &out); err != nil {
		return domain.EmbeddingsResponse{}, err
	}
	if out.Error != nil {
		return domain.EmbeddingsResponse{}, fmt.Errorf("%w: %s", domain.ErrUpstream, out.Error.Message)
	}

	return domain.EmbeddingsResponse{Model: target.Model.ModelKey, Data: out.Data, Usage: out.Usage}, nil
}

func (c *Client) post(ctx context.Context, provider domain.Provider, path string, reqBody, out any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(provider.BaseURL, "/") + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuth(httpReq, provider)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrUpstream, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read upstream response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("%w: status %d: %s", domain.ErrUpstream, resp.StatusCode, string(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode upstream response: %w", err)
	}
	return nil
}

func applyAuth(req *http.Request, provider domain.Provider) {
	switch provider.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+provider.AuthValue)
	case "api_key_header":
		req.Header.Set("Api-Key", provider.AuthValue)
	}
}
