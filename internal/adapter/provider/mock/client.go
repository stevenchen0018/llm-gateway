// Package mock implements domain.ProviderClient without making any network
// call. It is used for local development, smoke tests, and unit tests of
// the routing/rate-limit/cost pipeline so they don't depend on real vendor
// credentials or network access. A Provider row with base_url starting
// with "mock://" is routed to this client (see internal/service/routersvc).
package mock

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/tokencount"
)

type Client struct {
	videos sync.Map // video task id -> *mockVideoJob
}

func NewClient() *Client { return &Client{} }

// FailKeyword lets tests/smoke-scripts force a failure deterministically:
// any request whose last message contains this string returns an upstream
// error, which is useful for exercising the routing engine's failover path.
const FailKeyword = "__force_fail__"

func (c *Client) ChatCompletion(ctx context.Context, target domain.CallTarget, req domain.ChatRequest) (domain.ChatResponse, error) {
	var lastUserContent string
	promptTokens := 0
	for _, m := range req.Messages {
		promptTokens += tokencount.Estimate(m.Content)
		if m.Role == "user" {
			lastUserContent = m.Content
		}
	}

	if strings.Contains(lastUserContent, FailKeyword) {
		return domain.ChatResponse{}, fmt.Errorf("%w: mock forced failure for provider %s", domain.ErrUpstream, target.Provider.Code)
	}

	reply := fmt.Sprintf("[mock:%s/%s] echo: %s", target.Provider.Code, target.Model.ModelKey, lastUserContent)
	completionTokens := tokencount.Estimate(reply)

	return domain.ChatResponse{
		ID:      "mock-" + fmt.Sprint(time.Now().UnixNano()),
		Model:   target.Model.ModelKey,
		Created: time.Now().Unix(),
		Choices: []domain.ChatChoice{{
			Index:        0,
			Message:      domain.ChatMessage{Role: "assistant", Content: reply},
			FinishReason: "stop",
		}},
		Usage: domain.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}, nil
}

func (c *Client) Embeddings(ctx context.Context, target domain.CallTarget, req domain.EmbeddingsRequest) (domain.EmbeddingsResponse, error) {
	data := make([]domain.EmbeddingData, len(req.Input))
	promptTokens := 0
	for i, text := range req.Input {
		promptTokens += tokencount.Estimate(text)
		data[i] = domain.EmbeddingData{Index: i, Embedding: []float32{0.1, 0.2, 0.3}}
	}

	return domain.EmbeddingsResponse{
		Model: target.Model.ModelKey,
		Data:  data,
		Usage: domain.Usage{PromptTokens: promptTokens, TotalTokens: promptTokens},
	}, nil
}
