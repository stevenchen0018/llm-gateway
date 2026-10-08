package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// PlaygroundHandler serves 模型体验: text generation, image understanding
// and image generation against a chosen model.
type PlaygroundHandler struct {
	playground *service.PlaygroundService
}

func NewPlaygroundHandler(p *service.PlaygroundService) *PlaygroundHandler {
	return &PlaygroundHandler{playground: p}
}

func (h *PlaygroundHandler) Chat(c *gin.Context) {
	var req dto.PlaygroundChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	msgs := make([]domain.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = domain.ChatMessage{Role: m.Role, Content: m.Content}
	}
	res, err := h.playground.Chat(c.Request.Context(), req.ModelID, domain.ChatRequest{
		Messages: msgs, Temperature: req.Temperature, TopP: req.TopP, MaxTokens: req.MaxTokens,
	})
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, res)
}

func (h *PlaygroundHandler) Vision(c *gin.Context) {
	var req dto.PlaygroundVisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.playground.Vision(c.Request.Context(), req.ModelID, req.Prompt, req.Image)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, res)
}

func (h *PlaygroundHandler) Image(c *gin.Context) {
	var req dto.PlaygroundImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.playground.Image(c.Request.Context(), req.ModelID, req.Prompt, req.Size)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, res)
}

func (h *PlaygroundHandler) SubmitVideo(c *gin.Context) {
	var req dto.PlaygroundVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	task, err := h.playground.SubmitVideo(c.Request.Context(), req.ModelID, domain.VideoRequest{
		Prompt: req.Prompt, Size: req.Size, Seconds: req.Seconds, Image: req.Image,
	})
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusAccepted, task)
}

func (h *PlaygroundHandler) GetVideo(c *gin.Context) {
	modelID := queryInt64Ptr(c, "model_id")
	if modelID == nil {
		response.AdminError(c, http.StatusBadRequest, "model_id is required")
		return
	}
	task, err := h.playground.GetVideo(c.Request.Context(), *modelID, c.Param("task"))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, task)
}

// VideoContent proxies a finished video the vendor serves only behind its key.
func (h *PlaygroundHandler) VideoContent(c *gin.Context) {
	modelID := queryInt64Ptr(c, "model_id")
	if modelID == nil {
		response.AdminError(c, http.StatusBadRequest, "model_id is required")
		return
	}
	body, ct, err := h.playground.VideoContent(c.Request.Context(), *modelID, c.Param("task"))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	defer body.Close()
	c.Header("Cache-Control", "private, max-age=3600")
	c.DataFromReader(http.StatusOK, -1, ct, body, nil)
}
