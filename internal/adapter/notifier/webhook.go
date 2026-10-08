// Package notifier implements domain.Notifier over outbound webhooks. The
// default implementation targets Feishu's "custom bot" webhook format,
// matching the article's "3分钟内通过飞书感知" alerting channel; any other
// webhook-based channel (Slack, DingTalk, a generic incident endpoint) can
// reuse the same struct by pointing URL at a different endpoint that
// accepts a similarly-shaped JSON body, or by adding a sibling
// implementation behind the same domain.Notifier interface.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type WebhookNotifier struct {
	url    string
	client *http.Client
}

func NewWebhookNotifier(url string) *WebhookNotifier {
	return &WebhookNotifier{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

type feishuTextMessage struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Notify posts event to the configured webhook. If no URL is configured
// (e.g. in local dev), it is a silent no-op rather than an error — alerting
// delivery failures should never fail the request path that triggered them.
func (n *WebhookNotifier) Notify(ctx context.Context, event domain.AlertEvent) error {
	if n.url == "" {
		return nil
	}

	msg := feishuTextMessage{MsgType: "text"}
	msg.Content.Text = fmt.Sprintf("[%s][%s] %s (ref_id=%d)", event.Level, event.Type, event.Message, event.RefID)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
