package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type ModelRepo struct{ db *gorm.DB }

func NewModelRepo(db *gorm.DB) *ModelRepo { return &ModelRepo{db: db} }

func toModelRow(m *domain.Model) *ModelRow {
	tags, _ := json.Marshal(m.Tags)
	if m.Tags == nil {
		tags = []byte("[]")
	}
	cat := m.Category
	if cat == "" {
		cat = "text"
	}
	return &ModelRow{
		ID: m.ID, ProviderID: m.ProviderID, VendorID: m.VendorID, ModelKey: m.ModelKey, DisplayName: m.DisplayName,
		Type: string(m.Type), InputPricePer1K: m.InputPricePer1K, OutputPricePer1K: m.OutputPricePer1K,
		TPMLimit: m.TPMLimit, QPSLimit: m.QPSLimit, Status: string(m.Status), CreatedBy: m.CreatedBy,
		Category: cat, ContextLength: m.ContextLength, Tags: string(tags), Description: m.Description,
		ReleasedAt: m.ReleasedAt,
	}
}

func fromModelRow(r *ModelRow) *domain.Model {
	var tags []string
	_ = json.Unmarshal([]byte(r.Tags), &tags)
	if tags == nil {
		tags = []string{}
	}
	return &domain.Model{
		ID: r.ID, ProviderID: r.ProviderID, VendorID: r.VendorID, ModelKey: r.ModelKey, DisplayName: r.DisplayName,
		Type: domain.ModelType(r.Type), InputPricePer1K: r.InputPricePer1K, OutputPricePer1K: r.OutputPricePer1K,
		TPMLimit: r.TPMLimit, QPSLimit: r.QPSLimit, Status: domain.ModelStatus(r.Status), CreatedBy: r.CreatedBy,
		Category: r.Category, ContextLength: r.ContextLength, Tags: tags, Description: r.Description,
		ReleasedAt: r.ReleasedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (r *ModelRepo) Create(ctx context.Context, m *domain.Model) error {
	row := toModelRow(m)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create model: %w", err)
	}
	m.ID = row.ID
	m.CreatedAt, m.UpdatedAt = row.CreatedAt, row.UpdatedAt
	return nil
}

func (r *ModelRepo) Update(ctx context.Context, m *domain.Model) error {
	if err := r.db.WithContext(ctx).Model(&ModelRow{}).Where("id = ?", m.ID).
		Select("display_name", "input_price_per_1k", "output_price_per_1k", "tpm_limit", "qps_limit", "status",
			"category", "context_length", "tags", "description", "released_at", "vendor_id").
		Updates(toModelRow(m)).Error; err != nil {
		return fmt.Errorf("update model: %w", err)
	}
	return nil
}

func (r *ModelRepo) Get(ctx context.Context, id int64) (*domain.Model, error) {
	var row ModelRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get model: %w", err)
	}
	return fromModelRow(&row), nil
}

func (r *ModelRepo) GetWithProvider(ctx context.Context, id int64) (*domain.ModelWithProvider, error) {
	m, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var prow ProviderRow
	if err := r.db.WithContext(ctx).First(&prow, "id = ?", m.ProviderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get model provider: %w", err)
	}
	out := []*domain.ModelWithProvider{{Model: *m, Provider: *fromProviderRow(&prow)}}
	if err := r.attachVendors(ctx, out); err != nil {
		return nil, err
	}
	return out[0], nil
}

// attachVendors fills Vendor and Supply (the vendor→supplier link) for each model.
func (r *ModelRepo) attachVendors(ctx context.Context, models []*domain.ModelWithProvider) error {
	ids := map[int64]bool{}
	for _, m := range models {
		if m.VendorID != nil {
			ids[*m.VendorID] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	var vrows []VendorRow
	if err := r.db.WithContext(ctx).Where("id IN ?", list).Find(&vrows).Error; err != nil {
		return fmt.Errorf("load model vendors: %w", err)
	}
	var srows []VendorSupplierRow
	if err := r.db.WithContext(ctx).Where("vendor_id IN ?", list).Find(&srows).Error; err != nil {
		return fmt.Errorf("load vendor suppliers: %w", err)
	}
	vendors := map[int64]*domain.Vendor{}
	for i := range vrows {
		vendors[vrows[i].ID] = fromVendorRow(&vrows[i])
	}
	links := map[[2]int64]*domain.VendorSupplier{}
	for i := range srows {
		links[[2]int64{srows[i].VendorID, srows[i].ProviderID}] = fromVendorSupplierRow(&srows[i])
	}
	for _, m := range models {
		if m.VendorID == nil {
			continue
		}
		m.Vendor = vendors[*m.VendorID]
		m.Supply = links[[2]int64{*m.VendorID, m.ProviderID}]
	}
	return nil
}

func (r *ModelRepo) attachProviders(ctx context.Context, rows []ModelRow) ([]*domain.ModelWithProvider, error) {
	if len(rows) == 0 {
		return []*domain.ModelWithProvider{}, nil
	}
	ids := make([]int64, 0, len(rows))
	seen := make(map[int64]bool)
	for _, row := range rows {
		if !seen[row.ProviderID] {
			seen[row.ProviderID] = true
			ids = append(ids, row.ProviderID)
		}
	}
	var prows []ProviderRow
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&prows).Error; err != nil {
		return nil, fmt.Errorf("load model providers: %w", err)
	}
	byID := make(map[int64]*domain.Provider, len(prows))
	for i := range prows {
		byID[prows[i].ID] = fromProviderRow(&prows[i])
	}
	out := make([]*domain.ModelWithProvider, len(rows))
	for i := range rows {
		mwp := &domain.ModelWithProvider{Model: *fromModelRow(&rows[i])}
		if p, ok := byID[rows[i].ProviderID]; ok {
			mwp.Provider = *p
		}
		out[i] = mwp
	}
	if err := r.attachVendors(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *ModelRepo) List(ctx context.Context) ([]*domain.ModelWithProvider, error) {
	var rows []ModelRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	return r.attachProviders(ctx, rows)
}

func (r *ModelRepo) FindActiveByName(ctx context.Context, name string) ([]*domain.ModelWithProvider, error) {
	var rows []ModelRow
	err := r.db.WithContext(ctx).
		Where("status = 'active' AND (lower(model_key) = lower(?) OR lower(display_name) = lower(?))", name, name).
		Order("id").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("find models by name: %w", err)
	}
	all, err := r.attachProviders(ctx, rows)
	if err != nil {
		return nil, err
	}
	// a disabled supplier, a disabled vendor, or a disabled vendor→supplier
	// link all take the offering out of routing
	out := all[:0]
	for _, m := range all {
		if m.Provider.Status == domain.ProviderStatusActive && m.Supply.Active() &&
			(m.Vendor == nil || m.Vendor.Status != "disabled") {
			out = append(out, m)
		}
	}
	return out, nil
}
