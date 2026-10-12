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
	"context"
	"encoding/json"
	"net/http"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/httpx"
	"github.com/stevenchen/llm-gateway/internal/domain"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: httpx.NewClient()}
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
		return domain.ChatResponse{}, &domain.UpstreamError{Status: http.StatusOK, Message: out.Error.Message}
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
		return domain.EmbeddingsResponse{}, &domain.UpstreamError{Status: http.StatusOK, Message: out.Error.Message}
	}

	return domain.EmbeddingsResponse{Model: target.Model.ModelKey, Data: out.Data, Usage: out.Usage}, nil
}

func (c *Client) post(ctx context.Context, provider domain.Provider, path string, reqBody, out any) error {
	return httpx.Do(ctx, c.httpClient, http.MethodPost, httpx.JoinURL(provider.BaseURL, path), authHeaders(provider), reqBody, out, errorMessage)
}

// ListModels implements domain.ProviderProber with GET /models: it checks
// connectivity and the credential without spending tokens.
func (c *Client) ListModels(ctx context.Context, provider domain.Provider) ([]string, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := httpx.Do(ctx, c.httpClient, http.MethodGet, httpx.JoinURL(provider.BaseURL, "/models"), authHeaders(provider), nil, &out, errorMessage); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// errorMessage reads {"error":{"message":...}} (OpenAI error envelope).
func errorMessage(raw []byte) string {
	var e struct {
		Error *upstreamError `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != nil {
		return e.Error.Message
	}
	return ""
}

func authHeaders(provider domain.Provider) map[string]string {
	switch provider.AuthType {
	case "bearer":
		return map[string]string{"Authorization": "Bearer " + provider.AuthValue}
	case "api_key_header":
		return map[string]string{"Api-Key": provider.AuthValue}
	}
	return nil
}

func applyAuth(req *http.Request, provider domain.Provider) {
	for k, v := range authHeaders(provider) {
		req.Header.Set(k, v)
	}
}
