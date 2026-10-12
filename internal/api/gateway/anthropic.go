package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/anthropic"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/pkg/tokencount"
)

// Anthropic Messages API (POST /v1/messages): clients built on the Anthropic
// SDK (or tools such as Claude Code pointed at ANTHROPIC_BASE_URL) call the
// gateway in their native format. The request is converted to the unified
// chat request, so it goes through the same routing, quotas, filtering and
// billing as an OpenAI-format call and can be served by a supplier of any
// protocol.

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      json.RawMessage    `json:"system"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature *float64           `json:"temperature"`
	TopP        *float64           `json:"top_p"`
	Stream      bool               `json:"stream"`
}

type anthropicTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Model        string               `json:"model"`
	Content      []anthropicTextBlock `json:"content"`
	StopReason   string               `json:"stop_reason"`
	StopSequence *string              `json:"stop_sequence"`
	Usage        anthropicUsage       `json:"usage"`
}

// anthropicText flattens a string or an array of content blocks to text.
// Text blocks are kept; tool calls/results are rendered as readable text so
// the conversation history is not lost; images become a placeholder.
func anthropicText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		err := json.Unmarshal(raw, &s)
		return s, err
	}
	var blocks []struct {
		Type    string          `json:"type"`
		Text    string          `json:"text"`
		Name    string          `json:"name"`
		Input   json.RawMessage `json:"input"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("content must be a string or an array of content blocks")
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image", "document":
			parts = append(parts, "["+b.Type+"]")
		case "tool_use":
			parts = append(parts, fmt.Sprintf("[tool_use %s %s]", b.Name, string(b.Input)))
		case "tool_result":
			inner, _ := anthropicText(b.Content)
			parts = append(parts, "[tool_result] "+inner)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func (r anthropicRequest) toChat() (domain.ChatRequest, error) {
	req := domain.ChatRequest{Model: r.Model, Temperature: r.Temperature, TopP: r.TopP, Stream: r.Stream}
	if r.MaxTokens > 0 {
		mt := r.MaxTokens
		req.MaxTokens = &mt
	}
	sys, err := anthropicText(r.System)
	if err != nil {
		return req, fmt.Errorf("system: %w", err)
	}
	if sys != "" {
		req.Messages = append(req.Messages, domain.ChatMessage{Role: "system", Content: sys})
	}
	for i, m := range r.Messages {
		text, err := anthropicText(m.Content)
		if err != nil {
			return req, fmt.Errorf("messages[%d]: %w", i, err)
		}
		req.Messages = append(req.Messages, domain.ChatMessage{Role: m.Role, Content: text})
	}
	return req, nil
}

func bindAnthropic(c *gin.Context) (anthropicRequest, domain.ChatRequest, bool) {
	var in anthropicRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return in, domain.ChatRequest{}, false
	}
	if in.Model == "" || len(in.Messages) == 0 {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "missing_field", "model and messages are required")
		return in, domain.ChatRequest{}, false
	}
	req, err := in.toChat()
	if err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return in, req, false
	}
	return in, req, true
}

// Messages serves POST /v1/messages.
func (h *Handler) Messages(c *gin.Context) {
	in, req, ok := bindAnthropic(c)
	if !ok {
		return
	}
	resp, err := h.gateway.ChatCompletion(c.Request.Context(), middleware.APIKeyFromContext(c), req, c.ClientIP())
	if err != nil {
		writeGatewayError(c, err)
		return
	}
	out := toAnthropic(resp, in.Model)
	if in.Stream {
		writeAnthropicStream(c, out)
		return
	}
	c.JSON(http.StatusOK, out)
}

func toAnthropic(resp domain.ChatResponse, model string) anthropicResponse {
	text, finish := "", "stop"
	if len(resp.Choices) > 0 {
		text, finish = resp.Choices[0].Message.Content, resp.Choices[0].FinishReason
	}
	id := resp.ID
	if !strings.HasPrefix(id, "msg_") {
		id = "msg_" + strings.ReplaceAll(id, "-", "")
	}
	return anthropicResponse{
		ID: id, Type: "message", Role: "assistant", Model: model,
		Content:    []anthropicTextBlock{{Type: "text", Text: text}},
		StopReason: anthropic.StopReason(finish),
		Usage:      anthropicUsage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens},
	}
}

// writeAnthropicStream answers stream:true with the Messages API event
// sequence. Like the OpenAI stream, the answer is produced as a whole first
// and then sent in chunks.
func writeAnthropicStream(c *gin.Context, m anthropicResponse) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	send := func(event string, data any) {
		b, _ := json.Marshal(data)
		_, _ = c.Writer.WriteString("event: " + event + "\ndata: " + string(b) + "\n\n")
		c.Writer.Flush()
	}
	start := m
	start.Content, start.StopReason = []anthropicTextBlock{}, ""
	send("message_start", gin.H{"type": "message_start", "message": gin.H{
		"id": start.ID, "type": "message", "role": "assistant", "model": start.Model, "content": start.Content,
		"stop_reason": nil, "stop_sequence": nil, "usage": anthropicUsage{InputTokens: m.Usage.InputTokens, OutputTokens: 1},
	}})
	send("content_block_start", gin.H{"type": "content_block_start", "index": 0, "content_block": gin.H{"type": "text", "text": ""}})
	send("ping", gin.H{"type": "ping"})
	runes := []rune(m.Content[0].Text)
	for i := 0; i < len(runes); i += streamPiece {
		send("content_block_delta", gin.H{"type": "content_block_delta", "index": 0,
			"delta": gin.H{"type": "text_delta", "text": string(runes[i:min(i+streamPiece, len(runes))])}})
	}
	send("content_block_stop", gin.H{"type": "content_block_stop", "index": 0})
	send("message_delta", gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": m.StopReason, "stop_sequence": nil},
		"usage": gin.H{"output_tokens": m.Usage.OutputTokens}})
	send("message_stop", gin.H{"type": "message_stop"})
}

// CountTokens serves POST /v1/messages/count_tokens with the gateway's
// token estimator (no upstream call, not billed).
func (h *Handler) CountTokens(c *gin.Context) {
	_, req, ok := bindAnthropic(c)
	if !ok {
		return
	}
	n := 0
	for _, m := range req.Messages {
		n += tokencount.Estimate(m.Content)
	}
	c.JSON(http.StatusOK, gin.H{"input_tokens": n})
}
