package domain

import (
	"context"
	"time"
)

type AlertType string

const (
	AlertTypeQuota    AlertType = "quota"    // TPM/QPS rate-limit rejection
	AlertTypeBudget   AlertType = "budget"   // budget threshold crossed
	AlertTypeFailover AlertType = "failover" // routing candidate marked unhealthy
	AlertTypeReport   AlertType = "report"   // weekly usage/cost digest
	AlertTypeSecurity AlertType = "security" // IP whitelist denial, content filter block
)

type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelCritical AlertLevel = "critical"
)

// AlertEvent backs the article's "分钟级异常告警...3分钟内通过飞书感知"
// requirement: every quota rejection, budget breach, or failover is recorded
// here and forwarded through a Notifier.
type AlertEvent struct {
	ID         int64      `json:"id"`
	Type       AlertType  `json:"type"`
	RefID      int64      `json:"ref_id"`           // key/model/budget id depending on Type
	KeyID      *int64     `json:"key_id,omitempty"` // API key concerned, if any (department scoping)
	Message    string     `json:"message"`
	Level      AlertLevel `json:"level"`
	NotifiedAt *time.Time `json:"notified_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type AlertRepository interface {
	Create(ctx context.Context, a *AlertEvent) error
	// List returns the newest alerts; with deptID set, only platform-wide
	// alerts (no key) and alerts about that department's keys.
	List(ctx context.Context, limit int, deptID *int64) ([]*AlertEvent, error)
	Page(ctx context.Context, q AlertQuery) ([]*AlertEvent, int64, error)
	MarkNotified(ctx context.Context, id int64, at time.Time) error
}

// Notifier delivers an AlertEvent to an external channel. WebhookNotifier
// (internal/adapter/notifier) implements this for Feishu-compatible bots;
// other channels can be added behind the same interface.
type Notifier interface {
	Notify(ctx context.Context, event AlertEvent) error
}
