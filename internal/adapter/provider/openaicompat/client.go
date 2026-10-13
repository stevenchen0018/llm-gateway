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
	params     domain.ParamRewriter
}

// WithParams applies parameter compatibility rules (and their automatic
// recovery) to every request this adapter sends.
func (c *Client) WithParams(p domain.ParamRewriter) *Client {
	c.params = p
	return c
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

	m, err := httpx.ToMap(body)
	if err != nil {
		return domain.ChatResponse{}, err
	}
	// pass through the parameters the gateway does not model (seed, stop,
	// response_format, reasoning_effort, ...); the modelled ones win
	for k, v := range req.Extra {
		if _, set := m[k]; !set {
			var val any
			if json.Unmarshal(v, &val) == nil {
				m[k] = val
			}
		}
	}
	var out chatResponseBody
	if err := c.send(ctx, target, domain.ParamOpChat, "/chat/completions", m, &out); err != nil {
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
	if err := c.post(ctx, target, domain.ParamOpEmbedding, "/embeddings", body, &out); err != nil {
		return domain.EmbeddingsResponse{}, err
	}
	if out.Error != nil {
		return domain.EmbeddingsResponse{}, &domain.UpstreamError{Status: http.StatusOK, Message: out.Error.Message}
	}

	return domain.EmbeddingsResponse{Model: target.Model.ModelKey, Data: out.Data, Usage: out.Usage}, nil
}

// post sends a request struct to a model endpoint (see send).
func (c *Client) post(ctx context.Context, target domain.CallTarget, op, path string, reqBody, out any) error {
	m, err := httpx.ToMap(reqBody)
	if err != nil {
		return err
	}
	return c.send(ctx, target, op, path, m, out)
}

// send posts body after applying the parameter rules of the target
// supplier/model, adapting and retrying once if a parameter is rejected.
func (c *Client) send(ctx context.Context, target domain.CallTarget, op, path string, body map[string]any, out any) error {
	return httpx.DoAdapted(ctx, c.httpClient, http.MethodPost, httpx.JoinURL(target.Provider.BaseURL, path), authHeaders(target.Provider),
		body, out, errorMessage, httpx.Adapt{Params: c.params, Target: target, Op: op})
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
