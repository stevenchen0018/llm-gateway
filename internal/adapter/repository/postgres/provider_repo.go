package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type ProviderRepo struct{ db *gorm.DB }

func NewProviderRepo(db *gorm.DB) *ProviderRepo { return &ProviderRepo{db: db} }

func toProviderRow(p *domain.Provider) *ProviderRow {
	rate := p.DiscountRate
	if rate.IsZero() {
		rate = decimal.NewFromInt(1)
	}
	return &ProviderRow{
		ID: p.ID, Code: p.Code, Name: p.Name, BaseURL: p.BaseURL,
		AuthType: p.AuthType, AuthValue: p.AuthValue, Status: string(p.Status),
		DiscountRate: rate, Description: p.Description, SupplierType: supplierType(p.SupplierType), Contact: p.Contact,
	}
}

func fromProviderRow(r *ProviderRow) *domain.Provider {
	return &domain.Provider{
		ID: r.ID, Code: r.Code, Name: r.Name, BaseURL: r.BaseURL,
		AuthType: r.AuthType, AuthValue: r.AuthValue, Status: domain.ProviderStatus(r.Status),
		DiscountRate: r.DiscountRate, Description: r.Description, SupplierType: r.SupplierType, Contact: r.Contact,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (r *ProviderRepo) Create(ctx context.Context, p *domain.Provider) error {
	row := toProviderRow(p)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create provider: %w", err)
	}
	p.ID = row.ID
	p.DiscountRate = row.DiscountRate
	p.CreatedAt, p.UpdatedAt = row.CreatedAt, row.UpdatedAt
	return nil
}

func (r *ProviderRepo) Update(ctx context.Context, p *domain.Provider) error {
	if err := r.db.WithContext(ctx).Model(&ProviderRow{}).Where("id = ?", p.ID).
		Select("name", "base_url", "auth_type", "auth_value", "status", "discount_rate", "description", "supplier_type", "contact").
		Updates(toProviderRow(p)).Error; err != nil {
		return fmt.Errorf("update provider: %w", err)
	}
	return nil
}

func (r *ProviderRepo) Get(ctx context.Context, id int64) (*domain.Provider, error) {
	var row ProviderRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get provider: %w", err)
	}
	return fromProviderRow(&row), nil
}

func (r *ProviderRepo) GetByCode(ctx context.Context, code string) (*domain.Provider, error) {
	var row ProviderRow
	if err := r.db.WithContext(ctx).First(&row, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get provider by code: %w", err)
	}
	return fromProviderRow(&row), nil
}

func (r *ProviderRepo) List(ctx context.Context) ([]*domain.Provider, error) {
	var rows []ProviderRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	out := make([]*domain.Provider, len(rows))
	for i := range rows {
		out[i] = fromProviderRow(&rows[i])
	}
	return out, nil
}

func supplierType(t string) string {
	if t == "" {
		return domain.SupplierCloud
	}
	return t
}
