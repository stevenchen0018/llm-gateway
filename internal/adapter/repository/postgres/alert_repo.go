package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type AlertRepo struct{ db *gorm.DB }

func NewAlertRepo(db *gorm.DB) *AlertRepo { return &AlertRepo{db: db} }

func (r *AlertRepo) Create(ctx context.Context, a *domain.AlertEvent) error {
	row := AlertEventRow{
		Type: string(a.Type), RefID: a.RefID, KeyID: a.KeyID, Message: a.Message, Level: string(a.Level),
		NotifiedAt: a.NotifiedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create alert event: %w", err)
	}
	a.ID = row.ID
	a.CreatedAt = row.CreatedAt
	return nil
}

func (r *AlertRepo) List(ctx context.Context, limit int, deptID *int64) ([]*domain.AlertEvent, error) {
	var rows []AlertEventRow
	tx := r.db.WithContext(ctx).Order("created_at desc").Limit(limit)
	if deptID != nil {
		tx = tx.Where("key_id IS NULL OR key_id IN (?)", deptKeyIDs(r.db, *deptID))
	}
	if err := tx.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list alert events: %w", err)
	}
	out := make([]*domain.AlertEvent, len(rows))
	for i, row := range rows {
		out[i] = fromAlertRow(row)
	}
	return out, nil
}

// Page is the filtered, paged alert list; department scope keeps
// platform-wide alerts (no key) plus alerts about the department's keys.
func (r *AlertRepo) Page(ctx context.Context, q domain.AlertQuery) ([]*domain.AlertEvent, int64, error) {
	tx := r.db.WithContext(ctx).Model(&AlertEventRow{})
	if q.DeptID != nil {
		tx = tx.Where("key_id IS NULL OR key_id IN (?)", deptKeyIDs(r.db, *q.DeptID))
	}
	if q.Type != "" {
		tx = tx.Where("type = ?", q.Type)
	}
	if q.Level != "" {
		tx = tx.Where("level = ?", q.Level)
	}
	if kw := strings.TrimSpace(q.Q); kw != "" {
		tx = tx.Where("message ILIKE ?", "%"+escapeLike(kw)+"%")
	}
	if q.Notified != nil {
		if *q.Notified {
			tx = tx.Where("notified_at IS NOT NULL")
		} else {
			tx = tx.Where("notified_at IS NULL")
		}
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count alert events: %w", err)
	}
	var rows []AlertEventRow
	if err := tx.Order("created_at desc, id desc").Offset(q.Offset).Limit(q.Limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("page alert events: %w", err)
	}
	out := make([]*domain.AlertEvent, len(rows))
	for i, row := range rows {
		out[i] = fromAlertRow(row)
	}
	return out, total, nil
}

func fromAlertRow(row AlertEventRow) *domain.AlertEvent {
	return &domain.AlertEvent{
		ID: row.ID, Type: domain.AlertType(row.Type), RefID: row.RefID, KeyID: row.KeyID, Message: row.Message,
		Level: domain.AlertLevel(row.Level), NotifiedAt: row.NotifiedAt, CreatedAt: row.CreatedAt,
	}
}

func (r *AlertRepo) MarkNotified(ctx context.Context, id int64, at time.Time) error {
	if err := r.db.WithContext(ctx).Model(&AlertEventRow{}).Where("id = ?", id).
		Update("notified_at", at).Error; err != nil {
		return fmt.Errorf("mark alert notified: %w", err)
	}
	return nil
}
