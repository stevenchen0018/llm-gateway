// Package gateway implements the OpenAI-compatible business API described
// in the article's "统一API入口" capability: POST /v1/chat/completions,
// POST /v1/completions, POST /v1/embeddings, and GET /v1/models. Every
// handler here trusts middleware.APIKeyAuth to have already placed an
// authenticated, active domain.APIKey in the gin context.
package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/pgerr"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

type Handler struct {
	gateway *service.GatewayService
	market  *service.ModelMarketService
}

func NewHandler(gateway *service.GatewayService, market *service.ModelMarketService) *Handler {
	return &Handler{gateway: gateway, market: market}
}

func (h *Handler) ChatCompletions(c *gin.Context) {
	var req domain.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	if req.Model == "" || len(req.Messages) == 0 {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "missing_field", "model and messages are required")
		return
	}

	key := middleware.APIKeyFromContext(c)
	resp, err := h.gateway.ChatCompletion(c.Request.Context(), key, req, c.ClientIP())
	if err != nil {
		writeGatewayError(c, err)
		return
	}
	if req.Stream {
		writeChatStream(c, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// chunk is one "chat.completion.chunk" server-sent event.
type chunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
	Usage   *domain.Usage `json:"usage,omitempty"`
}

type chunkChoice struct {
	Index        int               `json:"index"`
	Delta        map[string]string `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

// streamPiece bounds the content carried by one SSE chunk.
const streamPiece = 64

// writeChatStream answers a stream:true request in the OpenAI SSE format so
// streaming clients (IDE coding assistants, SDKs) work. The completion is
// produced upstream as a whole (after failover, filtering and accounting)
// and then sent as a sequence of chunks, so the first token arrives when the
// full answer is ready rather than incrementally.
func writeChatStream(c *gin.Context, resp domain.ChatResponse) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	send := func(ch chunk) {
		b, _ := json.Marshal(ch)
		_, _ = c.Writer.WriteString("data: " + string(b) + "\n\n")
		c.Writer.Flush()
	}
	base := func() chunk {
		return chunk{ID: resp.ID, Object: "chat.completion.chunk", Created: resp.Created, Model: resp.Model}
	}
	for _, choice := range resp.Choices {
		first := base()
		first.Choices = []chunkChoice{{Index: choice.Index, Delta: map[string]string{"role": "assistant", "content": ""}}}
		send(first)
		runes := []rune(choice.Message.Content)
		for i := 0; i < len(runes); i += streamPiece {
			part := base()
			part.Choices = []chunkChoice{{Index: choice.Index, Delta: map[string]string{"content": string(runes[i:min(i+streamPiece, len(runes))])}}}
			send(part)
		}
		reason := choice.FinishReason
		if reason == "" {
			reason = "stop"
		}
		last := base()
		last.Choices = []chunkChoice{{Index: choice.Index, Delta: map[string]string{}, FinishReason: &reason}}
		send(last)
	}
	usage := base()
	usage.Choices = []chunkChoice{}
	usage.Usage = &resp.Usage
	send(usage)
	_, _ = c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
}

// Completions adapts the legacy single-prompt shape onto ChatCompletion.
func (h *Handler) Completions(c *gin.Context) {
	var req domain.CompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	if req.Model == "" || req.Prompt == "" {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "missing_field", "model and prompt are required")
		return
	}

	key := middleware.APIKeyFromContext(c)
	chatReq := domain.ChatRequest{
		Model:       req.Model,
		Messages:    []domain.ChatMessage{{Role: "user", Content: req.Prompt}},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}

	resp, err := h.gateway.ChatCompletion(c.Request.Context(), key, chatReq, c.ClientIP())
	if err != nil {
		writeGatewayError(c, err)
		return
	}

	text := ""
	if len(resp.Choices) > 0 {
		text = resp.Choices[0].Message.Content
	}
	c.JSON(http.StatusOK, domain.CompletionResponse{
		ID: resp.ID, Model: resp.Model, Created: resp.Created,
		Choices: []domain.CompletionChoice{{Index: 0, Text: text, FinishReason: "stop"}},
		Usage:   resp.Usage,
	})
}

func (h *Handler) Embeddings(c *gin.Context) {
	var req domain.EmbeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	if req.Model == "" || len(req.Input) == 0 {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "missing_field", "model and input are required")
		return
	}

	key := middleware.APIKeyFromContext(c)
	resp, err := h.gateway.Embeddings(c.Request.Context(), key, req, c.ClientIP())
	if err != nil {
		writeGatewayError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

type listedModel struct {
	id, owner string
	embedding bool
}

// allowedModels lists the routable model names this key may call: one entry
// per model name (several suppliers may offer it), limited to the key's
// allowlist; owner is the vendor that makes it.
func (h *Handler) allowedModels(c *gin.Context) ([]listedModel, bool) {
	models, err := h.market.ListMarket(c.Request.Context())
	if err != nil {
		response.GatewayError(c, http.StatusInternalServerError, "internal_error", "", "failed to list models")
		return nil, false
	}
	key := middleware.APIKeyFromContext(c)
	out := make([]listedModel, 0, len(models))
	seen := map[string]bool{}
	for _, m := range models {
		if m.Status != domain.ModelStatusActive || m.Provider.Status != domain.ProviderStatusActive || !m.Supply.Active() ||
			seen[m.DisplayName] || !key.AllowsModel(m.DisplayName) {
			continue
		}
		seen[m.DisplayName] = true
		owner := m.Provider.Code
		if m.Vendor != nil {
			owner = m.Vendor.Code
		}
		out = append(out, listedModel{id: m.DisplayName, owner: owner, embedding: m.Type == domain.ModelTypeEmbedding})
	}
	return out, true
}

// ListModels serves GET /v1/models in the OpenAI format, or in the Anthropic
// format when the client sends the anthropic-version header (Anthropic SDKs
// call the same path).
func (h *Handler) ListModels(c *gin.Context) {
	models, ok := h.allowedModels(c)
	if !ok {
		return
	}
	if c.GetHeader("anthropic-version") != "" {
		data := make([]gin.H, 0, len(models))
		for _, m := range models {
			data = append(data, gin.H{"type": "model", "id": m.id, "display_name": m.id, "created_at": "2025-01-01T00:00:00Z"})
		}
		first, last := "", ""
		if len(models) > 0 {
			first, last = models[0].id, models[len(models)-1].id
		}
		c.JSON(http.StatusOK, gin.H{"data": data, "has_more": false, "first_id": first, "last_id": last})
		return
	}
	type modelEntry struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	data := make([]modelEntry, 0, len(models))
	for _, m := range models {
		data = append(data, modelEntry{ID: m.id, Object: "model", OwnedBy: m.owner})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

func writeGatewayError(c *gin.Context, err error) {
	_ = c.Error(err)
	switch {
	case pgerr.IsSchemaMismatch(err):
		response.GatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "schema_not_ready", "Service database is not initialized; please contact the administrator")
	case errors.Is(err, domain.ErrInvalidArgument):
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_argument", err.Error())
	case errors.Is(err, domain.ErrModelNotAllowed):
		response.GatewayError(c, http.StatusForbidden, "permission_error", "model_not_allowed",
			"This API key is not allowed to call "+strings.TrimPrefix(err.Error(), domain.ErrModelNotAllowed.Error()+": "))
	case errors.Is(err, domain.ErrContentBlocked):
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "content_blocked",
			strings.TrimPrefix(err.Error(), domain.ErrContentBlocked.Error()+": "))
	case errors.Is(err, domain.ErrForbidden):
		response.GatewayError(c, http.StatusForbidden, "permission_error", "department_disabled", "The department that owns this API key is disabled")
	case errors.Is(err, domain.ErrRateLimited):
		response.GatewayError(c, http.StatusTooManyRequests, "rate_limit_error", "rate_limited", "Rate limit exceeded, please retry later")
	case errors.Is(err, domain.ErrBudgetExceeded):
		response.GatewayError(c, http.StatusPaymentRequired, "budget_error", "budget_exceeded", "Budget exceeded for this API key")
	case errors.Is(err, domain.ErrNoHealthyCandidate):
		response.GatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "no_healthy_candidate", "No healthy upstream model available")
	case errors.Is(err, domain.ErrUpstream):
		response.GatewayError(c, http.StatusBadGateway, "upstream_error", "upstream_error", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		response.GatewayError(c, http.StatusNotFound, "invalid_request_error", "not_found", err.Error())
	default:
		response.GatewayError(c, http.StatusInternalServerError, "internal_error", "", "internal server error")
	}
}
