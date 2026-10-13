package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type ParamRuleRow struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	Name         string     `gorm:"column:name"`
	Enabled      bool       `gorm:"column:enabled"`
	Source       string     `gorm:"column:source"`
	Protocol     string     `gorm:"column:protocol"`
	ProviderID   *int64     `gorm:"column:provider_id"`
	ModelPattern string     `gorm:"column:model_pattern"`
	Op           string     `gorm:"column:op"`
	Actions      []byte     `gorm:"column:actions;type:jsonb"`
	Priority     int        `gorm:"column:priority"`
	Remark       string     `gorm:"column:remark"`
	HitCount     int64      `gorm:"column:hit_count"`
	LastHitAt    *time.Time `gorm:"column:last_hit_at"`
	CreatedBy    string     `gorm:"column:created_by"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (ParamRuleRow) TableName() string { return "param_rules" }

type ParamRuleRepo struct{ db *gorm.DB }

func NewParamRuleRepo(db *gorm.DB) *ParamRuleRepo { return &ParamRuleRepo{db: db} }

func toParamRuleRow(r *domain.ParamRule) (*ParamRuleRow, error) {
	acts := r.Actions
	if acts == nil {
		acts = []domain.ParamAction{}
	}
	b, err := json.Marshal(acts)
	if err != nil {
		return nil, err
	}
	return &ParamRuleRow{ID: r.ID, Name: r.Name, Enabled: r.Enabled, Source: r.Source, Protocol: r.Protocol, ProviderID: r.ProviderID,
		ModelPattern: r.ModelPattern, Op: r.Op, Actions: b, Priority: r.Priority, Remark: r.Remark, CreatedBy: r.CreatedBy}, nil
}

func fromParamRuleRow(r *ParamRuleRow) *domain.ParamRule {
	out := &domain.ParamRule{ID: r.ID, Name: r.Name, Enabled: r.Enabled, Source: r.Source, Protocol: r.Protocol, ProviderID: r.ProviderID,
		ModelPattern: r.ModelPattern, Op: r.Op, Priority: r.Priority, Remark: r.Remark, HitCount: r.HitCount, LastHitAt: r.LastHitAt,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	_ = json.Unmarshal(r.Actions, &out.Actions)
	if out.Actions == nil {
		out.Actions = []domain.ParamAction{}
	}
	return out
}

func (r *ParamRuleRepo) List(ctx context.Context) ([]*domain.ParamRule, error) {
	var rows []ParamRuleRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list param rules: %w", err)
	}
	out := make([]*domain.ParamRule, len(rows))
	for i := range rows {
		out[i] = fromParamRuleRow(&rows[i])
	}
	return out, nil
}

func (r *ParamRuleRepo) Get(ctx context.Context, id int64) (*domain.ParamRule, error) {
	var row ParamRuleRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get param rule: %w", err)
	}
	return fromParamRuleRow(&row), nil
}

// Create inserts a rule; an auto rule that another node learned at the same
// moment is silently kept (unique index on supplier + model + op).
func (r *ParamRuleRepo) Create(ctx context.Context, p *domain.ParamRule) error {
	row, err := toParamRuleRow(p)
	if err != nil {
		return err
	}
	tx := r.db.WithContext(ctx)
	if p.Source == domain.ParamRuleAuto {
		tx = tx.Clauses(clause.OnConflict{DoNothing: true})
	}
	if err := tx.Create(row).Error; err != nil {
		return fmt.Errorf("create param rule: %w", err)
	}
	p.ID, p.CreatedAt, p.UpdatedAt = row.ID, row.CreatedAt, row.UpdatedAt
	return nil
}

func (r *ParamRuleRepo) Update(ctx context.Context, p *domain.ParamRule) error {
	row, err := toParamRuleRow(p)
	if err != nil {
		return err
	}
	row.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Model(&ParamRuleRow{}).Where("id = ?", p.ID).
		Select("name", "enabled", "protocol", "provider_id", "model_pattern", "op", "actions", "priority", "remark", "updated_at").
		Updates(row).Error; err != nil {
		return fmt.Errorf("update param rule: %w", err)
	}
	return nil
}

func (r *ParamRuleRepo) Delete(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&ParamRuleRow{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete param rule: %w", err)
	}
	return nil
}

func (r *ParamRuleRepo) FindAuto(ctx context.Context, providerID int64, model, op string) (*domain.ParamRule, error) {
	var row ParamRuleRow
	err := r.db.WithContext(ctx).Where("source = ? AND provider_id = ? AND model_pattern = ? AND op = ?", domain.ParamRuleAuto, providerID, model, op).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find auto param rule: %w", err)
	}
	return fromParamRuleRow(&row), nil
}

func (r *ParamRuleRepo) AddHits(ctx context.Context, hits map[int64]int64, at time.Time) error {
	for id, n := range hits {
		if err := r.db.WithContext(ctx).Model(&ParamRuleRow{}).Where("id = ?", id).
			Updates(map[string]any{"hit_count": gorm.Expr("hit_count + ?", n), "last_hit_at": at}).Error; err != nil {
			return fmt.Errorf("add param rule hits: %w", err)
		}
	}
	return nil
}
