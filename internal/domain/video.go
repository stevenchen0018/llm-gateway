package domain

import (
	"context"
	"io"
	"time"
)

// VideoRequest asks a video model for a clip. Image (optional, data: or
// http(s) URL) turns text-to-video into image-to-video.
type VideoRequest struct {
	Prompt  string `json:"prompt"`
	Size    string `json:"size"`    // e.g. 1280x720
	Seconds int    `json:"seconds"` // clip length
	Image   string `json:"image,omitempty"`
}

type VideoStatus string

const (
	VideoQueued    VideoStatus = "queued"
	VideoRunning   VideoStatus = "running"
	VideoSucceeded VideoStatus = "succeeded"
	VideoFailed    VideoStatus = "failed"
)

// VideoTask is an asynchronous generation job: vendors take tens of seconds
// to minutes, so the console submits, then polls.
type VideoTask struct {
	ID       string      `json:"id"`
	Model    string      `json:"model"`
	Status   VideoStatus `json:"status"`
	Progress int         `json:"progress"` // 0-100
	// VideoURL is directly playable (http(s) or data: URL). When the vendor
	// only serves the file behind its API key, ContentProxied is true and the
	// console fetches it through the gateway instead.
	VideoURL       string    `json:"video_url,omitempty"`
	ContentProxied bool      `json:"content_proxied,omitempty"`
	Error          string    `json:"error,omitempty"`
	Size           string    `json:"size"`
	Seconds        int       `json:"seconds"`
	CreatedAt      time.Time `json:"created_at"`
}

// VideoClient is implemented by provider adapters that can generate video.
type VideoClient interface {
	SubmitVideo(ctx context.Context, target CallTarget, req VideoRequest) (VideoTask, error)
	GetVideo(ctx context.Context, target CallTarget, taskID string) (VideoTask, error)
	// VideoContent streams the finished file (for ContentProxied tasks).
	VideoContent(ctx context.Context, target CallTarget, taskID string) (io.ReadCloser, string, error)
}
