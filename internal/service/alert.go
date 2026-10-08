package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// AlertService records every quota rejection, budget breach, and failover
// event, and best-effort forwards it to the configured Notifier — the
// article's "分钟级异常告警...3分钟内通过飞书感知" requirement. Persisting the
// event never depends on notification succeeding: dashboards must show the
// alert even if the webhook is down.
type AlertService struct {
	alerts   domain.AlertRepository
	notifier domain.Notifier
	log      *zap.Logger

	mu       sync.Mutex
	lastSent map[string]time.Time
}

func NewAlertService(alerts domain.AlertRepository, notifier domain.Notifier, log *zap.Logger) *AlertService {
	return &AlertService{alerts: alerts, notifier: notifier, log: log, lastSent: map[string]time.Time{}}
}

// dedupeWindow suppresses repeats of the same alert (type + subject + key):
// under sustained overload every rejected request would otherwise insert a
// row and fire a webhook, turning a rate-limit event into a write storm.
var dedupeWindow = map[domain.AlertType]time.Duration{
	domain.AlertTypeQuota:    time.Minute,
	domain.AlertTypeFailover: time.Minute,
	domain.AlertTypeBudget:   10 * time.Minute,
	domain.AlertTypeSecurity: 10 * time.Minute,
}

func (s *AlertService) suppressed(alertType domain.AlertType, refID int64, keyID *int64, level domain.AlertLevel) bool {
	win, ok := dedupeWindow[alertType]
	if !ok {
		return false
	}
	k := fmt.Sprintf("%s|%s|%d|", alertType, level, refID)
	if keyID != nil {
		k += fmt.Sprint(*keyID)
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, seen := s.lastSent[k]; seen && now.Sub(t) < win {
		return true
	}
	s.lastSent[k] = now
	if len(s.lastSent) > 10000 { // bound memory: drop expired entries
		for kk, t := range s.lastSent {
			if now.Sub(t) > 10*time.Minute {
				delete(s.lastSent, kk)
			}
		}
	}
	return false
}

// Raise records and delivers an alert. keyID (optional) ties it to an API
// key so department users only see alerts about their own keys.
func (s *AlertService) Raise(ctx context.Context, alertType domain.AlertType, refID int64, keyID *int64, message string, level domain.AlertLevel) {
	if s.suppressed(alertType, refID, keyID, level) {
		return
	}
	event := &domain.AlertEvent{Type: alertType, RefID: refID, KeyID: keyID, Message: message, Level: level}
	if err := s.alerts.Create(ctx, event); err != nil {
		s.log.Error("persist alert event failed", zap.Error(err))
		return
	}

	if err := s.notifier.Notify(ctx, *event); err != nil {
		s.log.Warn("alert notification delivery failed", zap.Error(err), zap.Int64("alert_id", event.ID))
		return
	}
	now := time.Now()
	if err := s.alerts.MarkNotified(ctx, event.ID, now); err != nil {
		s.log.Warn("mark alert notified failed", zap.Error(err))
	}
}

func (s *AlertService) List(ctx context.Context, limit int, deptID *int64) ([]*domain.AlertEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.alerts.List(ctx, limit, deptID)
}

func (s *AlertService) Page(ctx context.Context, q domain.AlertQuery) ([]*domain.AlertEvent, int64, error) {
	return s.alerts.Page(ctx, q)
}
