package postgres

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type AuditRepo struct{ db *gorm.DB }

func NewAuditRepo(db *gorm.DB) *AuditRepo { return &AuditRepo{db: db} }

func fromAuditRow(row AuditLogRow) *domain.AuditLog {
	return &domain.AuditLog{ID: row.ID, KeyID: row.KeyID, Action: domain.AuditAction(row.Action),
		Operator: row.Operator, Detail: row.Detail, CreatedAt: row.CreatedAt}
}

func (r *AuditRepo) Create(ctx context.Context, a *domain.AuditLog) error {
	row := AuditLogRow{KeyID: a.KeyID, Action: string(a.Action), Operator: a.Operator, Detail: a.Detail}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	a.ID = row.ID
	a.CreatedAt = row.CreatedAt
	return nil
}

func (r *AuditRepo) ListByKey(ctx context.Context, keyID int64) ([]*domain.AuditLog, error) {
	var rows []AuditLogRow
	if err := r.db.WithContext(ctx).Where("key_id = ?", keyID).Order("created_at desc, id desc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	out := make([]*domain.AuditLog, len(rows))
	for i, row := range rows {
		out[i] = fromAuditRow(row)
	}
	return out, nil
}

func (r *AuditRepo) List(ctx context.Context, limit int) ([]*domain.AuditLog, error) {
	var rows []AuditLogRow
	if err := r.db.WithContext(ctx).Order("created_at desc, id desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	out := make([]*domain.AuditLog, len(rows))
	for i, row := range rows {
		out[i] = fromAuditRow(row)
	}
	return out, nil
}

// Page is the filtered, paged audit trail. Department scope and the key-name
// search go through api_keys so the database does the filtering.
func (r *AuditRepo) Page(ctx context.Context, q domain.AuditQuery) ([]*domain.AuditLog, int64, error) {
	tx := r.db.WithContext(ctx).Model(&AuditLogRow{})
	if q.DeptID != nil {
		tx = tx.Where("key_id IN (?)", deptKeyIDs(r.db, *q.DeptID))
	}
	if q.KeyID != nil {
		tx = tx.Where("key_id = ?", *q.KeyID)
	}
	if q.Action != "" {
		tx = tx.Where("action = ?", q.Action)
	}
	if kw := strings.TrimSpace(q.Q); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		tx = tx.Where("(operator ILIKE ? OR detail ILIKE ? OR key_id IN (?))", like, like,
			r.db.Table("api_keys").Select("id").Where("name ILIKE ?", like))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}
	var rows []AuditLogRow
	if err := tx.Order("created_at desc, id desc").Offset(q.Offset).Limit(q.Limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("page audit logs: %w", err)
	}
	out := make([]*domain.AuditLog, len(rows))
	for i, row := range rows {
		out[i] = fromAuditRow(row)
	}
	return out, total, nil
}
