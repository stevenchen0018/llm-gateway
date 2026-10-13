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

// maxParamRecoveries bounds how many rejected parameters one call fixes
// (each fix costs a round trip).
const maxParamRecoveries = 3

// Adapt describes the call for parameter compatibility handling.
type Adapt struct {
	Params domain.ParamRewriter // nil = send the body as built
	Target domain.CallTarget
	Op     string // domain.ParamOp*
}

// ToMap turns a request struct into a JSON object map rules can edit
// (numbers stay json.Number so integers are re-encoded exactly).
func ToMap(body any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("request body is not a JSON object: %w", err)
	}
	return m, nil
}

// DoAdapted is Do for model calls: the body is rewritten by the parameter
// rules of the target supplier/model before sending, and when the supplier
// rejects a parameter (400/422) the body is adapted and the call retried; a
// retry that then succeeds is reported back so the fix is learned.
func DoAdapted(ctx context.Context, c *http.Client, method, url string, headers map[string]string, body map[string]any, out any,
	errMessage func([]byte) string, a Adapt) error {
	if a.Params != nil {
		a.Params.Rewrite(ctx, a.Target, a.Op, body)
	}
	var learned []domain.ParamAction
	cause := ""
	for attempt := 0; ; attempt++ {
		err := Do(ctx, c, method, url, headers, body, out, errMessage)
		var ue *domain.UpstreamError
		if err == nil {
			if len(learned) > 0 {
				a.Params.Learned(ctx, a.Target, a.Op, learned, cause)
			}
			return nil
		}
		if a.Params == nil || attempt >= maxParamRecoveries || !errors.As(err, &ue) {
			return err
		}
		fix := a.Params.Recover(ctx, a.Target, a.Op, body, ue)
		if len(fix) == 0 {
			return err
		}
		learned = append(learned, fix...)
		if cause == "" {
			cause = truncate(ue.Message, 200)
		}
	}
}
