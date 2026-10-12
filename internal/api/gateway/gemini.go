package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/gemini"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/pkg/tokencount"
)

// Gemini API (/v1beta/models/{model}:{method}): clients built on the Google
// Gen AI SDK call the gateway in their native format. {model} is any model
// name the gateway routes (not only Gemini models), and the call goes through
// the same pipeline as an OpenAI-format call.

// CtxModel lets handlers whose model is in the URL report it to the request
// capture middleware.
const CtxModel = middleware.CtxRequestModel

type geminiPart struct {
	Text       string          `json:"text"`
	Thought    bool            `json:"thought"`
	InlineData json.RawMessage `json:"inlineData"`
	FileData   json.RawMessage `json:"fileData"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiContent           `json:"systemInstruction"`
	GenerationConfig  *gemini.GenerationConfig `json:"generationConfig"`
}

func (p geminiContent) text() string {
	parts := make([]string, 0, len(p.Parts))
	for _, x := range p.Parts {
		switch {
		case x.Thought:
		case x.Text != "":
			parts = append(parts, x.Text)
		case len(x.InlineData) > 0 || len(x.FileData) > 0:
			parts = append(parts, "[media]")
		}
	}
	return strings.Join(parts, "\n")
}

func (r geminiRequest) toChat(model string) domain.ChatRequest {
	req := domain.ChatRequest{Model: model}
	if g := r.GenerationConfig; g != nil {
		req.Temperature, req.TopP, req.MaxTokens = g.Temperature, g.TopP, g.MaxOutputTokens
	}
	if r.SystemInstruction != nil {
		if s := r.SystemInstruction.text(); s != "" {
			req.Messages = append(req.Messages, domain.ChatMessage{Role: "system", Content: s})
		}
	}
	for _, c := range r.Contents {
		role := "user"
		if c.Role == "model" {
			role = "assistant"
		}
		req.Messages = append(req.Messages, domain.ChatMessage{Role: role, Content: c.text()})
	}
	return req
}

type geminiCandidate struct {
	Content      gemini.Content `json:"content"`
	FinishReason string         `json:"finishReason,omitempty"`
	Index        int            `json:"index"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate     `json:"candidates"`
	UsageMetadata *gemini.UsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string                `json:"modelVersion"`
	ResponseID    string                `json:"responseId"`
}

// GeminiModels serves POST /v1beta/models/{model}:{method}.
func (h *Handler) GeminiModels(c *gin.Context) {
	model, method, ok := strings.Cut(c.Param("action"), ":")
	if !ok || model == "" {
		response.GatewayError(c, http.StatusNotFound, "invalid_request_error", "not_found", "expected /v1beta/models/{model}:{method}")
		return
	}
	c.Set(CtxModel, model)
	switch method {
	case "generateContent":
		h.geminiGenerate(c, model, false)
	case "streamGenerateContent":
		h.geminiGenerate(c, model, true)
	case "countTokens":
		h.geminiCountTokens(c)
	case "embedContent":
		h.geminiEmbed(c, model, false)
	case "batchEmbedContents":
		h.geminiEmbed(c, model, true)
	default:
		response.GatewayError(c, http.StatusNotImplemented, "invalid_request_error", "unsupported_method", "method "+method+" is not supported by the gateway")
	}
}

func (h *Handler) geminiGenerate(c *gin.Context, model string, stream bool) {
	var in geminiRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	if len(in.Contents) == 0 {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "missing_field", "contents is required")
		return
	}
	req := in.toChat(model)
	resp, err := h.gateway.ChatCompletion(c.Request.Context(), middleware.APIKeyFromContext(c), req, c.ClientIP())
	if err != nil {
		writeGatewayError(c, err)
		return
	}
	text, finish := "", "stop"
	if len(resp.Choices) > 0 {
		text, finish = resp.Choices[0].Message.Content, resp.Choices[0].FinishReason
	}
	usage := &gemini.UsageMetadata{PromptTokenCount: resp.Usage.PromptTokens, CandidatesTokenCount: resp.Usage.CompletionTokens, TotalTokenCount: resp.Usage.TotalTokens}
	full := geminiResponse{
		Candidates:    []geminiCandidate{{Content: gemini.Content{Role: "model", Parts: []gemini.Part{{Text: text}}}, FinishReason: gemini.GeminiFinishReason(finish)}},
		UsageMetadata: usage, ModelVersion: model, ResponseID: resp.ID,
	}
	if !stream {
		c.JSON(http.StatusOK, full)
		return
	}
	writeGeminiStream(c, full, text)
}

// writeGeminiStream answers streamGenerateContent: server-sent events with
// ?alt=sse (what the SDKs request), otherwise a streamed JSON array. The last
// chunk carries finishReason and usage, like the real API.
func writeGeminiStream(c *gin.Context, full geminiResponse, text string) {
	runes := []rune(text)
	var chunks []geminiResponse
	for i := 0; i < len(runes) || i == 0; i += streamPiece {
		ch := geminiResponse{ModelVersion: full.ModelVersion, ResponseID: full.ResponseID, Candidates: []geminiCandidate{{
			Content: gemini.Content{Role: "model", Parts: []gemini.Part{{Text: string(runes[i:min(i+streamPiece, len(runes))])}}},
		}}}
		chunks = append(chunks, ch)
		if len(runes) == 0 {
			break
		}
	}
	last := &chunks[len(chunks)-1]
	last.Candidates[0].FinishReason = full.Candidates[0].FinishReason
	last.UsageMetadata = full.UsageMetadata

	sse := c.Query("alt") == "sse"
	if sse {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
	} else {
		c.Header("Content-Type", "application/json")
	}
	c.Status(http.StatusOK)
	for i, ch := range chunks {
		b, _ := json.Marshal(ch)
		switch {
		case sse:
			_, _ = c.Writer.WriteString("data: " + string(b) + "\r\n\r\n")
		case i == 0:
			_, _ = c.Writer.WriteString("[" + string(b))
		default:
			_, _ = c.Writer.WriteString(",\r\n" + string(b))
		}
		c.Writer.Flush()
	}
	if !sse {
		_, _ = c.Writer.WriteString("]")
	}
}

func (h *Handler) geminiCountTokens(c *gin.Context) {
	var in struct {
		geminiRequest
		GenerateContentRequest *geminiRequest `json:"generateContentRequest"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	src := in.geminiRequest
	if in.GenerateContentRequest != nil {
		src = *in.GenerateContentRequest
	}
	n := 0
	for _, m := range src.toChat("").Messages {
		n += tokencount.Estimate(m.Content)
	}
	c.JSON(http.StatusOK, gin.H{"totalTokens": n})
}

func (h *Handler) geminiEmbed(c *gin.Context, model string, batch bool) {
	var in struct {
		Content  geminiContent `json:"content"`
		Requests []struct {
			Content geminiContent `json:"content"`
		} `json:"requests"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "invalid_body", err.Error())
		return
	}
	var inputs domain.StringList
	if batch {
		for _, r := range in.Requests {
			inputs = append(inputs, r.Content.text())
		}
	} else {
		inputs = domain.StringList{in.Content.text()}
	}
	if len(inputs) == 0 {
		response.GatewayError(c, http.StatusBadRequest, "invalid_request_error", "missing_field", "content is required")
		return
	}
	resp, err := h.gateway.Embeddings(c.Request.Context(), middleware.APIKeyFromContext(c),
		domain.EmbeddingsRequest{Model: model, Input: inputs}, c.ClientIP())
	if err != nil {
		writeGatewayError(c, err)
		return
	}
	values := func(i int) gin.H {
		if i < len(resp.Data) {
			return gin.H{"values": resp.Data[i].Embedding}
		}
		return gin.H{"values": []float32{}}
	}
	if !batch {
		c.JSON(http.StatusOK, gin.H{"embedding": values(0)})
		return
	}
	out := make([]gin.H, len(inputs))
	for i := range inputs {
		out[i] = values(i)
	}
	c.JSON(http.StatusOK, gin.H{"embeddings": out})
}

// GeminiListModels serves GET /v1beta/models in the Gemini format.
func (h *Handler) GeminiListModels(c *gin.Context) {
	names, ok := h.allowedModels(c)
	if !ok {
		return
	}
	out := make([]gin.H, 0, len(names))
	for _, n := range names {
		methods := []string{"generateContent", "streamGenerateContent", "countTokens"}
		if n.embedding {
			methods = []string{"embedContent", "batchEmbedContents"}
		}
		out = append(out, gin.H{"name": "models/" + n.id, "displayName": n.id, "supportedGenerationMethods": methods})
	}
	c.JSON(http.StatusOK, gin.H{"models": out})
}
