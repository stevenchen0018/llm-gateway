package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ChatMessage is the OpenAI-compatible message shape used both on the
// gateway's public API and internally when calling provider adapters.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// UnmarshalJSON accepts content as a string, null, or an array of content
// parts ([{"type":"text","text":...}, {"type":"image_url",...}]) as sent by
// IDE coding assistants; text parts are joined, other parts are noted.
func (m *ChatMessage) UnmarshalJSON(b []byte) error {
	var raw struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	m.Role, m.Content = raw.Role, ""
	if len(raw.Content) == 0 || string(raw.Content) == "null" {
		return nil
	}
	if raw.Content[0] == '"' {
		return json.Unmarshal(raw.Content, &m.Content)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw.Content, &parts); err != nil {
		return fmt.Errorf("message content must be a string or an array of content parts")
	}
	texts := make([]string, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.Type == "text" || p.Text != "":
			texts = append(texts, p.Text)
		case p.Type == "image_url":
			texts = append(texts, "[image]")
		}
	}
	m.Content = strings.Join(texts, "\n")
	return nil
}

// ChatRequest is the gateway's unified representation of a chat completion
// call, independent of which upstream vendor ultimately serves it.
type ChatRequest struct {
	// Model is the business-facing alias (resolved to a real candidate by
	// the routing engine before an adapter ever sees this struct filled
	// in with the vendor's real model_key).
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	// MaxTokens is the output limit whatever the client called it
	// (max_tokens, or OpenAI's newer max_completion_tokens); the name sent to
	// a supplier is decided per supplier/model by the parameter rules.
	MaxTokens *int `json:"max_tokens,omitempty"`
	Stream    bool `json:"stream,omitempty"`
	// Extra holds other OpenAI-protocol parameters the gateway does not model
	// (seed, stop, response_format, reasoning_effort, ...). They are passed
	// through unchanged to OpenAI-protocol suppliers, so new upstream
	// parameters work without a gateway release; parameter rules can rename
	// or drop them per supplier.
	Extra map[string]json.RawMessage `json:"-"`
}

// chatKnownFields are modelled by ChatRequest; chatDroppedFields are not
// passed through because the gateway cannot honour them end to end (tool
// calls and multiple choices are not carried back in the unified response).
var (
	chatKnownFields   = map[string]bool{"model": true, "messages": true, "temperature": true, "top_p": true, "max_tokens": true, "max_completion_tokens": true, "stream": true}
	chatDroppedFields = map[string]bool{"stream_options": true, "tools": true, "tool_choice": true, "functions": true, "function_call": true, "parallel_tool_calls": true, "n": true}
)

func (r *ChatRequest) UnmarshalJSON(b []byte) error {
	type plain ChatRequest
	var in struct {
		plain
		MaxCompletionTokens *int `json:"max_completion_tokens"`
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*r = ChatRequest(in.plain)
	if r.MaxTokens == nil {
		r.MaxTokens = in.MaxCompletionTokens
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	for k, v := range all {
		if chatKnownFields[k] || chatDroppedFields[k] {
			continue
		}
		if r.Extra == nil {
			r.Extra = map[string]json.RawMessage{}
		}
		r.Extra[k] = v
	}
	return nil
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Created int64        `json:"created"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

// CompletionRequest/Response back the legacy-style POST /v1/completions
// endpoint, translated internally to a single-message ChatRequest.
type CompletionRequest struct {
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
}

type CompletionChoice struct {
	Index        int    `json:"index"`
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

type CompletionResponse struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Created int64              `json:"created"`
	Choices []CompletionChoice `json:"choices"`
	Usage   Usage              `json:"usage"`
}

type EmbeddingsRequest struct {
	Model string     `json:"model"`
	Input StringList `json:"input"`
}

// StringList accepts either a JSON string or an array of strings, matching
// the OpenAI embeddings contract for `input`.
type StringList []string

func (l *StringList) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*l = StringList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("input must be a string or an array of strings")
	}
	*l = many
	return nil
}

type EmbeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type EmbeddingsResponse struct {
	Model string          `json:"model"`
	Data  []EmbeddingData `json:"data"`
	Usage Usage           `json:"usage"`
}

// CallTarget carries everything a ProviderClient needs to actually reach a
// resolved candidate: the vendor's real model_key plus its Provider
// connection info.
type CallTarget struct {
	Provider Provider
	Model    Model
}

// ProviderClient is the single interface every vendor adapter
// (internal/adapter/provider/*) implements. The gateway core never branches
// on vendor identity outside this boundary.
type ProviderClient interface {
	ChatCompletion(ctx context.Context, target CallTarget, req ChatRequest) (ChatResponse, error)
	Embeddings(ctx context.Context, target CallTarget, req EmbeddingsRequest) (EmbeddingsResponse, error)
}

// ProviderProber is implemented by adapters that can check a supplier's
// connectivity and credential without spending tokens (by listing models).
type ProviderProber interface {
	ListModels(ctx context.Context, provider Provider) ([]string, error)
}

// ImageResult is the outcome of an image generation call: each entry is an
// http(s) URL or a data: URL.
type ImageResult struct {
	Model  string   `json:"model"`
	Images []string `json:"images"`
}

// MultimodalClient is implemented by provider adapters that can serve image
// understanding and image generation (模型体验). It is deliberately separate
// from ProviderClient so text-only adapters need not implement it.
type MultimodalClient interface {
	Vision(ctx context.Context, target CallTarget, prompt, imageDataURL string) (ChatResponse, error)
	GenerateImage(ctx context.Context, target CallTarget, prompt, size string) (ImageResult, error)
}
