package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

var _ domain.VideoClient = (*Client)(nil)

// videoJob is the OpenAI video API job shape (POST /videos, GET /videos/{id}),
// which OpenAI-compatible vendors follow. Some vendors return a direct url.
type videoJob struct {
	ID        string `json:"id"`
	Status    string `json:"status"` // queued | in_progress | completed | failed
	Progress  int    `json:"progress"`
	Size      string `json:"size"`
	Seconds   any    `json:"seconds"`
	URL       string `json:"url"`
	VideoURL  string `json:"video_url"`
	CreatedAt int64  `json:"created_at"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (j videoJob) toTask(model string) domain.VideoTask {
	t := domain.VideoTask{ID: j.ID, Model: model, Progress: j.Progress, Size: j.Size, CreatedAt: time.Unix(j.CreatedAt, 0)}
	switch v := j.Seconds.(type) {
	case float64:
		t.Seconds = int(v)
	case string:
		_, _ = fmt.Sscan(v, &t.Seconds)
	}
	switch j.Status {
	case "queued", "pending":
		t.Status = domain.VideoQueued
	case "in_progress", "running", "processing":
		t.Status = domain.VideoRunning
	case "completed", "succeeded":
		t.Status, t.Progress = domain.VideoSucceeded, 100
		if u := firstNonEmpty(j.VideoURL, j.URL); u != "" {
			t.VideoURL = u
		} else {
			t.ContentProxied = true
		}
	default:
		t.Status = domain.VideoFailed
	}
	if j.Error != nil {
		t.Status, t.Error = domain.VideoFailed, j.Error.Message
	}
	return t
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func (c *Client) SubmitVideo(ctx context.Context, target domain.CallTarget, req domain.VideoRequest) (domain.VideoTask, error) {
	body := map[string]any{"model": target.Model.ModelKey, "prompt": req.Prompt}
	if req.Size != "" {
		body["size"] = req.Size
	}
	if req.Seconds > 0 {
		body["seconds"] = fmt.Sprint(req.Seconds)
	}
	if req.Image != "" {
		body["input_reference"] = req.Image
	}
	var out videoJob
	if err := c.post(ctx, target.Provider, "/videos", body, &out); err != nil {
		return domain.VideoTask{}, err
	}
	if out.Error != nil && out.ID == "" {
		return domain.VideoTask{}, fmt.Errorf("%w: %s", domain.ErrUpstream, out.Error.Message)
	}
	return out.toTask(target.Model.ModelKey), nil
}

func (c *Client) GetVideo(ctx context.Context, target domain.CallTarget, taskID string) (domain.VideoTask, error) {
	resp, err := c.get(ctx, target.Provider, "/videos/"+url.PathEscape(taskID))
	if err != nil {
		return domain.VideoTask{}, err
	}
	defer resp.Body.Close()
	var out videoJob
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return domain.VideoTask{}, fmt.Errorf("decode upstream response: %w", err)
	}
	return out.toTask(target.Model.ModelKey), nil
}

func (c *Client) VideoContent(ctx context.Context, target domain.CallTarget, taskID string) (io.ReadCloser, string, error) {
	resp, err := c.get(ctx, target.Provider, "/videos/"+url.PathEscape(taskID)+"/content")
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "video/mp4"
	}
	return resp.Body, ct, nil
}

// get issues an authenticated GET; the caller closes the body on success.
func (c *Client) get(ctx context.Context, provider domain.Provider, path string) (*http.Response, error) {
	u := strings.TrimRight(provider.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	applyAuth(req, provider)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrUpstream, err)
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d: %s", domain.ErrUpstream, resp.StatusCode, string(b))
	}
	return resp, nil
}
