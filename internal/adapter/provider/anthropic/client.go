// Package anthropic implements domain.ProviderClient against the Anthropic
// Messages API (POST /v1/messages), used for suppliers whose protocol is
// "anthropic": Anthropic's own API and the many resellers / cloud platforms
// exposing the same wire format. The gateway's unified (OpenAI-shaped)
// request is converted here; callers never see the difference.
package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/httpx"
	"github.com/stevenchen/llm-gateway/internal/domain"
)

// APIVersion is the anthropic-version header sent with every request.
const APIVersion = "2023-06-01"

// defaultMaxTokens fills max_tokens, which the Messages API requires.
const defaultMaxTokens = 4096

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client { return &Client{httpClient: httpx.NewClient()} }

// Message is one Messages API turn; content is sent as plain text.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage is the Messages API usage object.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type messagesResponse struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

// ToMessages splits unified messages into the Messages API's top-level
// system prompt and user/assistant turns. Consecutive turns of the same role
// are merged, since the API expects them to alternate.
func ToMessages(msgs []domain.ChatMessage) (system string, out []Message) {
	var sys []string
	for _, m := range msgs {
		role := m.Role
		switch role {
		case "system", "developer":
			if m.Content != "" {
				sys = append(sys, m.Content)
			}
			continue
		case "assistant":
		default:
			role = "user"
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content += "\n\n" + m.Content
			continue
		}
		out = append(out, Message{Role: role, Content: m.Content})
	}
	return strings.Join(sys, "\n\n"), out
}

// FinishReason maps an Anthropic stop_reason to the OpenAI finish_reason.
func FinishReason(stop string) string {
	switch stop {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return "content_filter"
	}
	return "stop"
}

// StopReason maps an OpenAI finish_reason to an Anthropic stop_reason.
func StopReason(finish string) string {
	switch finish {
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "refusal"
	}
	return "end_turn"
}

func (c *Client) ChatCompletion(ctx context.Context, target domain.CallTarget, req domain.ChatRequest) (domain.ChatResponse, error) {
	system, msgs := ToMessages(req.Messages)
	if len(msgs) == 0 {
		msgs = []Message{{Role: "user", Content: "."}}
	}
	body := messagesRequest{
		Model: target.Model.ModelKey, MaxTokens: defaultMaxTokens, System: system, Messages: msgs,
		Temperature: req.Temperature, TopP: req.TopP,
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		body.MaxTokens = *req.MaxTokens
	}

	var out messagesResponse
	if err := httpx.Do(ctx, c.httpClient, http.MethodPost, endpoint(target.Provider.BaseURL, "messages"),
		headers(target.Provider), body, &out, errorMessage); err != nil {
		return domain.ChatResponse{}, err
	}

	var text strings.Builder
	for _, b := range out.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	prompt := out.Usage.InputTokens + out.Usage.CacheCreationInputTokens + out.Usage.CacheReadInputTokens
	return domain.ChatResponse{
		ID: out.ID, Model: target.Model.ModelKey, Created: time.Now().Unix(),
		Choices: []domain.ChatChoice{{
			Message:      domain.ChatMessage{Role: "assistant", Content: text.String()},
			FinishReason: FinishReason(out.StopReason),
		}},
		Usage: domain.Usage{PromptTokens: prompt, CompletionTokens: out.Usage.OutputTokens, TotalTokens: prompt + out.Usage.OutputTokens},
	}, nil
}

// Embeddings is not offered by the Messages API.
func (c *Client) Embeddings(context.Context, domain.CallTarget, domain.EmbeddingsRequest) (domain.EmbeddingsResponse, error) {
	return domain.EmbeddingsResponse{}, &domain.UpstreamError{Status: http.StatusNotImplemented,
		Message: "unsupported: the Anthropic protocol has no embeddings endpoint"}
}

// ListModels implements domain.ProviderProber with GET /v1/models.
func (c *Client) ListModels(ctx context.Context, provider domain.Provider) ([]string, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := httpx.Do(ctx, c.httpClient, http.MethodGet, endpoint(provider.BaseURL, "models?limit=1000"),
		headers(provider), nil, &out, errorMessage); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// endpoint accepts a base URL with or without the /v1 suffix
// (https://api.anthropic.com or https://api.anthropic.com/v1).
func endpoint(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base + "/" + path
}

// headers authenticates with the protocol's native x-api-key header (the
// supplier's auth_type only applies to the OpenAI protocol).
func headers(p domain.Provider) map[string]string {
	return map[string]string{"anthropic-version": APIVersion, "x-api-key": p.AuthValue}
}

// errorMessage reads {"type":"error","error":{"type":...,"message":...}}.
func errorMessage(raw []byte) string {
	var e struct {
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != nil && e.Error.Message != "" {
		return e.Error.Type + ": " + e.Error.Message
	}
	return ""
}
