package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type BudgetRepo struct{ db *gorm.DB }

func NewBudgetRepo(db *gorm.DB) *BudgetRepo { return &BudgetRepo{db: db} }

func toBudgetRow(b *domain.Budget) *BudgetRow {
	approval := b.ApprovalStatus
	if approval == "" {
		approval = "approved"
	}
	return &BudgetRow{
		ID: b.ID, KeyID: b.KeyID, Period: string(b.Period), Amount: b.Amount, Consumed: b.Consumed,
		Currency: b.Currency, AlertThresholdPct: b.AlertThresholdPct, Status: string(b.Status),
		ApprovalStatus: approval, ApproverLevel: b.ApproverLevel, Approver: b.Approver,
		Applicant: b.Applicant, Project: b.Project, Reason: b.Reason, RejectReason: b.RejectReason,
		ApprovedAt: b.ApprovedAt,
	}
}

func fromBudgetRow(r *BudgetRow) *domain.Budget {
	return &domain.Budget{
		ID: r.ID, KeyID: r.KeyID, Period: domain.BudgetPeriod(r.Period), Amount: r.Amount, Consumed: r.Consumed,
		Currency: r.Currency, AlertThresholdPct: r.AlertThresholdPct, Status: domain.BudgetStatus(r.Status),
		ApprovalStatus: r.ApprovalStatus, ApproverLevel: r.ApproverLevel, Approver: r.Approver,
		Applicant: r.Applicant, Project: r.Project, Reason: r.Reason, RejectReason: r.RejectReason,
		ApprovedAt: r.ApprovedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (r *BudgetRepo) Create(ctx context.Context, b *domain.Budget) error {
	row := toBudgetRow(b)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create budget: %w", err)
	}
	b.ID = row.ID
	b.CreatedAt, b.UpdatedAt = row.CreatedAt, row.UpdatedAt
	return nil
}

func (r *BudgetRepo) Get(ctx context.Context, id int64) (*domain.Budget, error) {
	var row BudgetRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get budget: %w", err)
	}
	return fromBudgetRow(&row), nil
}

func (r *BudgetRepo) GetByKeyID(ctx context.Context, keyID int64) (*domain.Budget, error) {
	var row BudgetRow
	if err := r.db.WithContext(ctx).Where("key_id = ? AND approval_status <> 'rejected'", keyID).
		Order("id desc").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get budget by key: %w", err)
	}
	return fromBudgetRow(&row), nil
}

func (r *BudgetRepo) List(ctx context.Context) ([]*domain.Budget, error) {
	var rows []BudgetRow
	if err := r.db.WithContext(ctx).Order("id desc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}
	out := make([]*domain.Budget, len(rows))
	for i := range rows {
		out[i] = fromBudgetRow(&rows[i])
	}
	return out, nil
}

// Page is the filtered, paged budget list (newest first).
func (r *BudgetRepo) Page(ctx context.Context, q domain.BudgetQuery) ([]*domain.Budget, int64, error) {
	tx := r.db.WithContext(ctx).Model(&BudgetRow{})
	if q.DeptID != nil {
		tx = tx.Where("key_id IN (?)", deptKeyIDs(r.db, *q.DeptID))
	}
	if q.ApprovalStatus != "" {
		tx = tx.Where("approval_status = ?", q.ApprovalStatus)
	}
	if kw := strings.TrimSpace(q.Q); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		tx = tx.Where("(project ILIKE ? OR applicant ILIKE ? OR reason ILIKE ? OR key_id IN (?))", like, like, like,
			r.db.Table("api_keys").Select("id").Where("name ILIKE ?", like))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count budgets: %w", err)
	}
	var rows []BudgetRow
	if err := tx.Order("id desc").Offset(q.Offset).Limit(q.Limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("page budgets: %w", err)
	}
	out := make([]*domain.Budget, len(rows))
	for i := range rows {
		out[i] = fromBudgetRow(&rows[i])
	}
	return out, total, nil
}

// AddConsumed atomically applies delta to consumed at the database level
// (UPDATE ... SET consumed = consumed + ? RETURNING *) so concurrent
// requests against the same budget never lose an update.
func (r *BudgetRepo) AddConsumed(ctx context.Context, id int64, delta decimal.Decimal) (*domain.Budget, error) {
	var row BudgetRow
	err := r.db.WithContext(ctx).Raw(
		`UPDATE budgets SET consumed = consumed + ?, updated_at = now() WHERE id = ? RETURNING *`,
		delta, id,
	).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("add consumed: %w", err)
	}
	if row.ID == 0 {
		return nil, domain.ErrNotFound
	}
	return fromBudgetRow(&row), nil
}

// Update deliberately excludes `consumed`, which is only ever changed by the
// atomic AddConsumed, so a stale read-modify-write can never clobber spend.
func (r *BudgetRepo) Update(ctx context.Context, b *domain.Budget) error {
	if err := r.db.WithContext(ctx).Model(&BudgetRow{}).Where("id = ?", b.ID).
		Select("period", "amount", "currency", "alert_threshold_pct", "status", "approval_status",
			"approver_level", "approver", "applicant", "project", "reason", "reject_reason", "approved_at").
		Updates(toBudgetRow(b)).Error; err != nil {
		return fmt.Errorf("update budget: %w", err)
	}
	return nil
}
