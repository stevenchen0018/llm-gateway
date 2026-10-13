// Package gemini implements domain.ProviderClient against the Google Gemini
// API (models/{model}:generateContent, :batchEmbedContents), used for
// suppliers whose protocol is "gemini": Google AI Studio / Gemini API and
// platforms exposing the same wire format.
package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/httpx"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/idgen"
	"github.com/stevenchen/llm-gateway/internal/pkg/tokencount"
)

type Client struct {
	httpClient *http.Client
	params     domain.ParamRewriter
}

func NewClient() *Client { return &Client{httpClient: httpx.NewClient()} }

// WithParams applies parameter compatibility rules (and their automatic
// recovery) to every request this adapter sends.
func (c *Client) WithParams(p domain.ParamRewriter) *Client {
	c.params = p
	return c
}

// send posts a request struct after applying the target's parameter rules.
func (c *Client) send(ctx context.Context, target domain.CallTarget, op, url string, body, out any) error {
	m, err := httpx.ToMap(body)
	if err != nil {
		return err
	}
	return httpx.DoAdapted(ctx, c.httpClient, http.MethodPost, url, headers(target.Provider), m, out, errorMessage,
		httpx.Adapt{Params: c.params, Target: target, Op: op})
}

// Part is one piece of a Gemini Content; Thought marks reasoning output.
type Part struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"`
}

// Content is a Gemini turn (role "user" or "model").
type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

// GenerationConfig holds the sampling parameters of generateContent.
type GenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type generateRequest struct {
	Contents          []Content         `json:"contents"`
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
}

// UsageMetadata is generateContent's token accounting.
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type generateResponse struct {
	Candidates []struct {
		Content      Content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback,omitempty"`
	UsageMetadata UsageMetadata `json:"usageMetadata"`
	ResponseID    string        `json:"responseId"`
}

// ToContents splits unified messages into Gemini's systemInstruction and
// user/model contents, merging consecutive turns of the same role.
func ToContents(msgs []domain.ChatMessage) (system *Content, out []Content) {
	var sys []Part
	for _, m := range msgs {
		role := "user"
		switch m.Role {
		case "system", "developer":
			if m.Content != "" {
				sys = append(sys, Part{Text: m.Content})
			}
			continue
		case "assistant", "model":
			role = "model"
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Parts = append(out[n-1].Parts, Part{Text: m.Content})
			continue
		}
		out = append(out, Content{Role: role, Parts: []Part{{Text: m.Content}}})
	}
	if len(sys) > 0 {
		system = &Content{Parts: sys}
	}
	return system, out
}

// FinishReason maps a Gemini finishReason to the OpenAI finish_reason.
func FinishReason(r string) string {
	switch r {
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "content_filter"
	}
	return "stop"
}

// GeminiFinishReason maps an OpenAI finish_reason to a Gemini finishReason.
func GeminiFinishReason(r string) string {
	switch r {
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	}
	return "STOP"
}

func (c *Client) ChatCompletion(ctx context.Context, target domain.CallTarget, req domain.ChatRequest) (domain.ChatResponse, error) {
	system, contents := ToContents(req.Messages)
	body := generateRequest{Contents: contents, SystemInstruction: system}
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens != nil {
		body.GenerationConfig = &GenerationConfig{Temperature: req.Temperature, TopP: req.TopP, MaxOutputTokens: req.MaxTokens}
	}

	var out generateResponse
	if err := c.send(ctx, target, domain.ParamOpChat, modelURL(target.Provider.BaseURL, target.Model.ModelKey, "generateContent"), body, &out); err != nil {
		return domain.ChatResponse{}, err
	}
	if len(out.Candidates) == 0 {
		reason := "no candidates returned"
		if out.PromptFeedback != nil && out.PromptFeedback.BlockReason != "" {
			reason = "prompt blocked: " + out.PromptFeedback.BlockReason
		}
		return domain.ChatResponse{}, &domain.UpstreamError{Status: http.StatusOK, Message: reason}
	}

	cand := out.Candidates[0]
	var text strings.Builder
	for _, p := range cand.Content.Parts {
		if !p.Thought {
			text.WriteString(p.Text)
		}
	}
	u := out.UsageMetadata
	completion := u.CandidatesTokenCount + u.ThoughtsTokenCount
	id := out.ResponseID
	if id == "" {
		id = idgen.RequestID()
	}
	return domain.ChatResponse{
		ID: id, Model: target.Model.ModelKey, Created: time.Now().Unix(),
		Choices: []domain.ChatChoice{{
			Message:      domain.ChatMessage{Role: "assistant", Content: text.String()},
			FinishReason: FinishReason(cand.FinishReason),
		}},
		Usage: domain.Usage{PromptTokens: u.PromptTokenCount, CompletionTokens: completion, TotalTokens: u.PromptTokenCount + completion},
	}, nil
}

func (c *Client) Embeddings(ctx context.Context, target domain.CallTarget, req domain.EmbeddingsRequest) (domain.EmbeddingsResponse, error) {
	model := "models/" + strings.TrimPrefix(target.Model.ModelKey, "models/")
	type embedReq struct {
		Model   string  `json:"model"`
		Content Content `json:"content"`
	}
	body := struct {
		Requests []embedReq `json:"requests"`
	}{}
	tokens := 0
	for _, text := range req.Input {
		body.Requests = append(body.Requests, embedReq{Model: model, Content: Content{Parts: []Part{{Text: text}}}})
		tokens += tokencount.Estimate(text)
	}

	var out struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := c.send(ctx, target, domain.ParamOpEmbedding, modelURL(target.Provider.BaseURL, target.Model.ModelKey, "batchEmbedContents"), body, &out); err != nil {
		return domain.EmbeddingsResponse{}, err
	}
	data := make([]domain.EmbeddingData, len(out.Embeddings))
	for i, e := range out.Embeddings {
		data[i] = domain.EmbeddingData{Index: i, Embedding: e.Values}
	}
	// the Gemini embeddings API reports no usage: estimate for accounting
	return domain.EmbeddingsResponse{Model: target.Model.ModelKey, Data: data,
		Usage: domain.Usage{PromptTokens: tokens, TotalTokens: tokens}}, nil
}

// ListModels implements domain.ProviderProber with GET /models.
func (c *Client) ListModels(ctx context.Context, provider domain.Provider) ([]string, error) {
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := httpx.Do(ctx, c.httpClient, http.MethodGet, apiBase(provider.BaseURL)+"/models?pageSize=1000",
		headers(provider), nil, &out, errorMessage); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		ids = append(ids, strings.TrimPrefix(m.Name, "models/"))
	}
	return ids, nil
}

// apiBase accepts a base URL with or without the version segment
// (https://generativelanguage.googleapis.com or .../v1beta).
func apiBase(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1beta") || strings.HasSuffix(base, "/v1") || strings.HasSuffix(base, "/v1alpha") {
		return base
	}
	return base + "/v1beta"
}

func modelURL(base, model, method string) string {
	return apiBase(base) + "/models/" + strings.TrimPrefix(model, "models/") + ":" + method
}

// headers authenticates with the protocol's native x-goog-api-key header.
func headers(p domain.Provider) map[string]string {
	return map[string]string{"x-goog-api-key": p.AuthValue}
}

// errorMessage reads {"error":{"code":..,"message":..,"status":..}}.
func errorMessage(raw []byte) string {
	var e struct {
		Error *struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != nil && e.Error.Message != "" {
		if e.Error.Status != "" {
			return e.Error.Status + ": " + e.Error.Message
		}
		return e.Error.Message
	}
	return ""
}
