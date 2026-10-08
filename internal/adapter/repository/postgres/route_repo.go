package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type RouteRepo struct{ db *gorm.DB }

func NewRouteRepo(db *gorm.DB) *RouteRepo { return &RouteRepo{db: db} }

func toRouteRow(r *domain.ModelRoute) *ModelRouteRow {
	st := r.SourceType
	if st == "" {
		st = "custom"
	}
	return &ModelRouteRow{
		ID: r.ID, Alias: r.Alias, CandidateModelID: r.CandidateModelID,
		Priority: r.Priority, Weight: r.Weight, Strategy: string(r.Strategy), Enabled: r.Enabled,
		Name: r.Name, AppID: r.AppID, APIKeyID: r.APIKeyID, SourceType: st,
		SourceVendorID: r.SourceVendorID, Remark: r.Remark,
	}
}

func fromRouteRow(row *ModelRouteRow) *domain.ModelRoute {
	return &domain.ModelRoute{
		ID: row.ID, Alias: row.Alias, CandidateModelID: row.CandidateModelID,
		Priority: row.Priority, Weight: row.Weight, Strategy: domain.RouteStrategy(row.Strategy),
		Enabled: row.Enabled, Name: row.Name, AppID: row.AppID, APIKeyID: row.APIKeyID,
		SourceType: row.SourceType, SourceVendorID: row.SourceVendorID, Remark: row.Remark,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (r *RouteRepo) Create(ctx context.Context, route *domain.ModelRoute) error {
	row := toRouteRow(route)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create route: %w", err)
	}
	route.ID = row.ID
	route.CreatedAt, route.UpdatedAt = row.CreatedAt, row.UpdatedAt
	return nil
}

func (r *RouteRepo) Update(ctx context.Context, route *domain.ModelRoute) error {
	if err := r.db.WithContext(ctx).Model(&ModelRouteRow{}).Where("id = ?", route.ID).
		Select("candidate_model_id", "priority", "weight", "strategy", "enabled", "name", "app_id",
			"source_type", "source_vendor_id", "remark", "alias").
		Updates(toRouteRow(route)).Error; err != nil {
		return fmt.Errorf("update route: %w", err)
	}
	return nil
}

func (r *RouteRepo) Delete(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&ModelRouteRow{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete route: %w", err)
	}
	return nil
}

func (r *RouteRepo) Get(ctx context.Context, id int64) (*domain.ModelRoute, error) {
	var row ModelRouteRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get route: %w", err)
	}
	return fromRouteRow(&row), nil
}

func (r *RouteRepo) ListByAlias(ctx context.Context, alias string) ([]*domain.ModelRoute, error) {
	return r.listWhere(ctx, "lower(alias) = lower(?) AND enabled = ?", alias, true)
}

func (r *RouteRepo) ListForKey(ctx context.Context, alias string, keyID int64) ([]*domain.ModelRoute, error) {
	return r.listWhere(ctx, "lower(alias) = lower(?) AND enabled = ? AND (api_key_id IS NULL OR api_key_id = ?)", alias, true, keyID)
}

func (r *RouteRepo) listWhere(ctx context.Context, query string, args ...any) ([]*domain.ModelRoute, error) {
	var rows []ModelRouteRow
	if err := r.db.WithContext(ctx).Where(query, args...).Order("priority, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	out := make([]*domain.ModelRoute, len(rows))
	for i := range rows {
		out[i] = fromRouteRow(&rows[i])
	}
	return out, nil
}

func (r *RouteRepo) List(ctx context.Context) ([]*domain.ModelRoute, error) {
	var rows []ModelRouteRow
	if err := r.db.WithContext(ctx).Order("alias, priority, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	out := make([]*domain.ModelRoute, len(rows))
	for i := range rows {
		out[i] = fromRouteRow(&rows[i])
	}
	return out, nil
}
