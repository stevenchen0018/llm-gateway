package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type VendorRepo struct{ db *gorm.DB }

func NewVendorRepo(db *gorm.DB) *VendorRepo { return &VendorRepo{db: db} }

func fromVendorRow(r *VendorRow) *domain.Vendor {
	return &domain.Vendor{
		ID: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, Website: r.Website,
		RoutingStrategy: domain.VendorRouting(r.RoutingStrategy), Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toVendorRow(v *domain.Vendor) *VendorRow {
	return &VendorRow{ID: v.ID, Code: v.Code, Name: v.Name, Description: v.Description, Website: v.Website,
		RoutingStrategy: string(v.RoutingStrategy), Status: v.Status}
}

func fromVendorSupplierRow(r *VendorSupplierRow) *domain.VendorSupplier {
	return &domain.VendorSupplier{ID: r.ID, VendorID: r.VendorID, ProviderID: r.ProviderID, Priority: r.Priority,
		Weight: r.Weight, Status: r.Status, Remark: r.Remark, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *VendorRepo) Create(ctx context.Context, v *domain.Vendor) error {
	row := toVendorRow(v)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create vendor: %w", err)
	}
	*v = *fromVendorRow(row)
	return nil
}

func (r *VendorRepo) Update(ctx context.Context, v *domain.Vendor) error {
	if err := r.db.WithContext(ctx).Model(&VendorRow{}).Where("id = ?", v.ID).
		Select("name", "description", "website", "routing_strategy", "status").Updates(toVendorRow(v)).Error; err != nil {
		return fmt.Errorf("update vendor: %w", err)
	}
	return nil
}

func (r *VendorRepo) Delete(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&VendorRow{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete vendor: %w", err)
	}
	return nil
}

func (r *VendorRepo) Get(ctx context.Context, id int64) (*domain.Vendor, error) {
	var row VendorRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get vendor: %w", err)
	}
	return fromVendorRow(&row), nil
}

func (r *VendorRepo) List(ctx context.Context) ([]*domain.Vendor, error) {
	var rows []VendorRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list vendors: %w", err)
	}
	out := make([]*domain.Vendor, len(rows))
	for i := range rows {
		out[i] = fromVendorRow(&rows[i])
	}
	return out, nil
}

func (r *VendorRepo) ListSuppliers(ctx context.Context, vendorID *int64) ([]*domain.VendorSupplier, error) {
	var rows []VendorSupplierRow
	tx := r.db.WithContext(ctx).Order("vendor_id, priority, id")
	if vendorID != nil {
		tx = tx.Where("vendor_id = ?", *vendorID)
	}
	if err := tx.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list vendor suppliers: %w", err)
	}
	out := make([]*domain.VendorSupplier, len(rows))
	for i := range rows {
		out[i] = fromVendorSupplierRow(&rows[i])
	}
	return out, nil
}

func (r *VendorRepo) GetSupplier(ctx context.Context, id int64) (*domain.VendorSupplier, error) {
	var row VendorSupplierRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get vendor supplier: %w", err)
	}
	return fromVendorSupplierRow(&row), nil
}

func (r *VendorRepo) CreateSupplier(ctx context.Context, s *domain.VendorSupplier) error {
	row := &VendorSupplierRow{VendorID: s.VendorID, ProviderID: s.ProviderID, Priority: s.Priority, Weight: s.Weight, Status: s.Status, Remark: s.Remark}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create vendor supplier: %w", err)
	}
	*s = *fromVendorSupplierRow(row)
	return nil
}

func (r *VendorRepo) UpdateSupplier(ctx context.Context, s *domain.VendorSupplier) error {
	if err := r.db.WithContext(ctx).Model(&VendorSupplierRow{}).Where("id = ?", s.ID).
		Select("priority", "weight", "status", "remark").
		Updates(&VendorSupplierRow{Priority: s.Priority, Weight: s.Weight, Status: s.Status, Remark: s.Remark}).Error; err != nil {
		return fmt.Errorf("update vendor supplier: %w", err)
	}
	return nil
}

func (r *VendorRepo) DeleteSupplier(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&VendorSupplierRow{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete vendor supplier: %w", err)
	}
	return nil
}
