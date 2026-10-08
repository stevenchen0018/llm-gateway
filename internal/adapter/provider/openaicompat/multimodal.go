package openaicompat

import (
	"context"
	"fmt"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

var _ domain.MultimodalClient = (*Client)(nil)

type visionMessage struct {
	Role    string       `json:"role"`
	Content []visionPart `json:"content"`
}

type visionPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionImageURL struct {
	URL string `json:"url"`
}

// Vision sends an OpenAI-style multimodal chat request (text + image_url part).
func (c *Client) Vision(ctx context.Context, target domain.CallTarget, prompt, imageDataURL string) (domain.ChatResponse, error) {
	body := struct {
		Model    string          `json:"model"`
		Messages []visionMessage `json:"messages"`
	}{
		Model: target.Model.ModelKey,
		Messages: []visionMessage{{Role: "user", Content: []visionPart{
			{Type: "text", Text: prompt},
			{Type: "image_url", ImageURL: &visionImageURL{URL: imageDataURL}},
		}}},
	}
	var out chatResponseBody
	if err := c.post(ctx, target.Provider, "/chat/completions", body, &out); err != nil {
		return domain.ChatResponse{}, err
	}
	if out.Error != nil {
		return domain.ChatResponse{}, fmt.Errorf("%w: %s", domain.ErrUpstream, out.Error.Message)
	}
	return domain.ChatResponse{ID: out.ID, Model: target.Model.ModelKey, Created: out.Created, Choices: out.Choices, Usage: out.Usage}, nil
}

// GenerateImage calls the OpenAI-compatible /images/generations endpoint.
func (c *Client) GenerateImage(ctx context.Context, target domain.CallTarget, prompt, size string) (domain.ImageResult, error) {
	body := map[string]any{"model": target.Model.ModelKey, "prompt": prompt, "size": size, "n": 1}
	var out struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Error *upstreamError `json:"error,omitempty"`
	}
	if err := c.post(ctx, target.Provider, "/images/generations", body, &out); err != nil {
		return domain.ImageResult{}, err
	}
	if out.Error != nil {
		return domain.ImageResult{}, fmt.Errorf("%w: %s", domain.ErrUpstream, out.Error.Message)
	}
	res := domain.ImageResult{Model: target.Model.ModelKey}
	for _, d := range out.Data {
		switch {
		case d.URL != "":
			res.Images = append(res.Images, d.URL)
		case d.B64JSON != "":
			res.Images = append(res.Images, "data:image/png;base64,"+d.B64JSON)
		}
	}
	if len(res.Images) == 0 {
		return res, fmt.Errorf("%w: vendor returned no image", domain.ErrUpstream)
	}
	return res, nil
}
