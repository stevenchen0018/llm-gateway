// Package httpx is the small HTTP layer shared by the supplier protocol
// adapters (openaicompat, anthropic, gemini): JSON in, JSON out, and every
// failure reported as a *domain.UpstreamError carrying the HTTP status.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// maxErrorBody bounds how much of an upstream error body is kept in messages.
const maxErrorBody = 600

// NewClient returns the HTTP client adapters use for model calls.
func NewClient() *http.Client { return &http.Client{Timeout: 120 * time.Second} }

// JoinURL appends path to base, avoiding double slashes.
func JoinURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// Do sends method url with an optional JSON body and decodes a 2xx JSON
// response into out (when non-nil). errMessage extracts a readable message
// from an error body in the supplier's own error format.
func Do(ctx context.Context, c *http.Client, method, url string, headers map[string]string, body, out any, errMessage func([]byte) string) error {
	var rd io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rd = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return &domain.UpstreamError{Message: "build request: " + err.Error()}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &domain.UpstreamError{Message: "timeout: " + err.Error()}
		}
		return &domain.UpstreamError{Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return &domain.UpstreamError{Status: resp.StatusCode, Message: "read response: " + err.Error()}
	}
	if resp.StatusCode >= 400 {
		msg := ""
		if errMessage != nil {
			msg = errMessage(raw)
		}
		if msg == "" {
			msg = truncate(strings.TrimSpace(string(raw)), maxErrorBody)
		}
		return &domain.UpstreamError{Status: resp.StatusCode, Message: msg}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &domain.UpstreamError{Status: resp.StatusCode, Message: "invalid response: " + truncate(err.Error(), 200)}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
