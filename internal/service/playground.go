package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// PlaygroundService powers 模型体验 (text generation, image understanding,
// image generation): a direct, one-off call to a chosen model with no
// routing, quota or billing, so teams can evaluate models before applying
// for a key.
type PlaygroundService struct {
	models  domain.ModelRepository
	clients *ProviderClientRegistry
}

func NewPlaygroundService(models domain.ModelRepository, clients *ProviderClientRegistry) *PlaygroundService {
	return &PlaygroundService{models: models, clients: clients}
}

type PlaygroundChatResult struct {
	domain.ChatResponse
	LatencyMS int64 `json:"latency_ms"`
}

func (s *PlaygroundService) target(ctx context.Context, modelID int64) (domain.CallTarget, error) {
	m, err := s.models.GetWithProvider(ctx, modelID)
	if err != nil {
		return domain.CallTarget{}, err
	}
	return domain.CallTarget{Provider: m.Provider, Model: m.Model}, nil
}

func (s *PlaygroundService) Chat(ctx context.Context, modelID int64, req domain.ChatRequest) (*PlaygroundChatResult, error) {
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("%w: messages are required", domain.ErrInvalidArgument)
	}
	t, err := s.target(ctx, modelID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := s.clients.For(t.Provider).ChatCompletion(ctx, t, req)
	if err != nil {
		return nil, err
	}
	return &PlaygroundChatResult{ChatResponse: resp, LatencyMS: time.Since(start).Milliseconds()}, nil
}

func (s *PlaygroundService) Vision(ctx context.Context, modelID int64, prompt, image string) (*PlaygroundChatResult, error) {
	if prompt == "" || image == "" {
		return nil, fmt.Errorf("%w: prompt and image are required", domain.ErrInvalidArgument)
	}
	t, err := s.target(ctx, modelID)
	if err != nil {
		return nil, err
	}
	mm, ok := s.clients.For(t.Provider).(domain.MultimodalClient)
	if !ok {
		return nil, fmt.Errorf("%w: provider %s does not support image understanding", domain.ErrInvalidArgument, t.Provider.Code)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := mm.Vision(ctx, t, prompt, image)
	if err != nil {
		return nil, err
	}
	return &PlaygroundChatResult{ChatResponse: resp, LatencyMS: time.Since(start).Milliseconds()}, nil
}

type PlaygroundImageResult struct {
	domain.ImageResult
	LatencyMS int64 `json:"latency_ms"`
}

func (s *PlaygroundService) Image(ctx context.Context, modelID int64, prompt, size string) (*PlaygroundImageResult, error) {
	if prompt == "" {
		return nil, fmt.Errorf("%w: prompt is required", domain.ErrInvalidArgument)
	}
	if size == "" {
		size = "1024x1024"
	}
	t, err := s.target(ctx, modelID)
	if err != nil {
		return nil, err
	}
	mm, ok := s.clients.For(t.Provider).(domain.MultimodalClient)
	if !ok {
		return nil, fmt.Errorf("%w: provider %s does not support image generation", domain.ErrInvalidArgument, t.Provider.Code)
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	start := time.Now()
	res, err := mm.GenerateImage(ctx, t, prompt, size)
	if err != nil {
		return nil, err
	}
	return &PlaygroundImageResult{ImageResult: res, LatencyMS: time.Since(start).Milliseconds()}, nil
}

func (s *PlaygroundService) videoTarget(ctx context.Context, modelID int64) (domain.CallTarget, domain.VideoClient, error) {
	t, err := s.target(ctx, modelID)
	if err != nil {
		return t, nil, err
	}
	vc, ok := s.clients.For(t.Provider).(domain.VideoClient)
	if !ok {
		return t, nil, fmt.Errorf("%w: provider %s does not support video generation", domain.ErrInvalidArgument, t.Provider.Code)
	}
	return t, vc, nil
}

var videoSizes = map[string]bool{"1280x720": true, "720x1280": true, "960x960": true, "1920x1080": true}

// SubmitVideo starts an asynchronous video generation job (视频生成).
func (s *PlaygroundService) SubmitVideo(ctx context.Context, modelID int64, req domain.VideoRequest) (domain.VideoTask, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return domain.VideoTask{}, fmt.Errorf("%w: prompt is required", domain.ErrInvalidArgument)
	}
	if req.Size == "" {
		req.Size = "1280x720"
	}
	if !videoSizes[req.Size] {
		return domain.VideoTask{}, fmt.Errorf("%w: unsupported size %q", domain.ErrInvalidArgument, req.Size)
	}
	if req.Seconds == 0 {
		req.Seconds = 5
	}
	if req.Seconds < 1 || req.Seconds > 20 {
		return domain.VideoTask{}, fmt.Errorf("%w: seconds must be between 1 and 20", domain.ErrInvalidArgument)
	}
	t, vc, err := s.videoTarget(ctx, modelID)
	if err != nil {
		return domain.VideoTask{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return vc.SubmitVideo(ctx, t, req)
}

func (s *PlaygroundService) GetVideo(ctx context.Context, modelID int64, taskID string) (domain.VideoTask, error) {
	t, vc, err := s.videoTarget(ctx, modelID)
	if err != nil {
		return domain.VideoTask{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return vc.GetVideo(ctx, t, taskID)
}

// VideoContent streams a finished video that the vendor only serves behind
// its credentials; the caller must close the reader.
func (s *PlaygroundService) VideoContent(ctx context.Context, modelID int64, taskID string) (io.ReadCloser, string, error) {
	t, vc, err := s.videoTarget(ctx, modelID)
	if err != nil {
		return nil, "", err
	}
	return vc.VideoContent(ctx, t, taskID)
}
