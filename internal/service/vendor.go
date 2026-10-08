package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// VendorService manages 厂商 (model makers) and which 供应商 (supply
// channels) supply each of them, including the per-supplier scheduling
// settings used when no routing policy matches (the supplier dimension of
// 模型调度).
type VendorService struct {
	vendors   domain.VendorRepository
	providers domain.ProviderRepository
	models    domain.ModelRepository
}

func NewVendorService(vendors domain.VendorRepository, providers domain.ProviderRepository, models domain.ModelRepository) *VendorService {
	return &VendorService{vendors: vendors, providers: providers, models: models}
}

// SupplierView is one vendor→supplier link with the supplier's details and
// how many of the vendor's models it offers.
type SupplierView struct {
	domain.VendorSupplier
	Provider   *domain.Provider `json:"provider,omitempty"`
	ModelCount int              `json:"model_count"`
}

// VendorView is a vendor with its suppliers.
type VendorView struct {
	domain.Vendor
	Suppliers  []SupplierView `json:"suppliers"`
	ModelCount int            `json:"model_count"`
}

var vendorCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

func (s *VendorService) List(ctx context.Context) ([]VendorView, error) {
	vendors, err := s.vendors.List(ctx)
	if err != nil {
		return nil, err
	}
	links, err := s.vendors.ListSuppliers(ctx, nil)
	if err != nil {
		return nil, err
	}
	providers, err := s.providers.List(ctx)
	if err != nil {
		return nil, err
	}
	models, err := s.models.List(ctx)
	if err != nil {
		return nil, err
	}
	pByID := map[int64]*domain.Provider{}
	for _, p := range providers {
		pByID[p.ID] = p
	}
	perVendor := map[int64]int{}
	perLink := map[[2]int64]int{}
	for _, m := range models {
		if m.VendorID != nil {
			perVendor[*m.VendorID]++
			perLink[[2]int64{*m.VendorID, m.ProviderID}]++
		}
	}
	byVendor := map[int64][]SupplierView{}
	for _, l := range links {
		byVendor[l.VendorID] = append(byVendor[l.VendorID], SupplierView{
			VendorSupplier: *l, Provider: pByID[l.ProviderID], ModelCount: perLink[[2]int64{l.VendorID, l.ProviderID}],
		})
	}
	out := make([]VendorView, 0, len(vendors))
	for _, v := range vendors {
		sups := byVendor[v.ID]
		if sups == nil {
			sups = []SupplierView{}
		}
		sort.SliceStable(sups, func(i, j int) bool { return sups[i].Priority < sups[j].Priority })
		out = append(out, VendorView{Vendor: *v, Suppliers: sups, ModelCount: perVendor[v.ID]})
	}
	return out, nil
}

func validRouting(r domain.VendorRouting) bool {
	switch r {
	case domain.VendorRoutingCostFirst, domain.VendorRoutingPriority, domain.VendorRoutingWeighted:
		return true
	}
	return false
}

func (s *VendorService) normalize(v *domain.Vendor) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" {
		return fmt.Errorf("%w: vendor name is required", domain.ErrInvalidArgument)
	}
	if v.RoutingStrategy == "" {
		v.RoutingStrategy = domain.VendorRoutingCostFirst
	}
	if !validRouting(v.RoutingStrategy) {
		return fmt.Errorf("%w: routing_strategy must be cost_first, supplier_priority or supplier_weighted", domain.ErrInvalidArgument)
	}
	if v.Status == "" {
		v.Status = "active"
	}
	if v.Status != "active" && v.Status != "disabled" {
		return fmt.Errorf("%w: status must be active or disabled", domain.ErrInvalidArgument)
	}
	return nil
}

func (s *VendorService) Create(ctx context.Context, v *domain.Vendor) error {
	v.Code = strings.ToLower(strings.TrimSpace(v.Code))
	if !vendorCodeRe.MatchString(v.Code) {
		return fmt.Errorf("%w: code must be 2-32 lowercase letters, digits, - or _", domain.ErrInvalidArgument)
	}
	if err := s.normalize(v); err != nil {
		return err
	}
	return s.vendors.Create(ctx, v)
}

func (s *VendorService) Update(ctx context.Context, v *domain.Vendor) error {
	if err := s.normalize(v); err != nil {
		return err
	}
	return s.vendors.Update(ctx, v)
}

func (s *VendorService) Get(ctx context.Context, id int64) (*domain.Vendor, error) {
	return s.vendors.Get(ctx, id)
}

// Delete removes a vendor that no longer has models.
func (s *VendorService) Delete(ctx context.Context, id int64) error {
	models, err := s.models.List(ctx)
	if err != nil {
		return err
	}
	for _, m := range models {
		if m.VendorID != nil && *m.VendorID == id {
			return fmt.Errorf("%w: 该厂商下仍有模型，请先迁移或下架模型", domain.ErrInvalidArgument)
		}
	}
	return s.vendors.Delete(ctx, id)
}

func normalizeLink(l *domain.VendorSupplier) error {
	if l.Priority < 0 || l.Priority > 9999 {
		return fmt.Errorf("%w: priority must be 0-9999", domain.ErrInvalidArgument)
	}
	if l.Weight <= 0 {
		l.Weight = 100
	}
	if l.Weight > 10000 {
		return fmt.Errorf("%w: weight must be 1-10000", domain.ErrInvalidArgument)
	}
	if l.Status == "" {
		l.Status = "active"
	}
	if l.Status != "active" && l.Status != "disabled" {
		return fmt.Errorf("%w: status must be active or disabled", domain.ErrInvalidArgument)
	}
	return nil
}

// AddSupplier records that providerID supplies vendorID.
func (s *VendorService) AddSupplier(ctx context.Context, l *domain.VendorSupplier) error {
	if _, err := s.vendors.Get(ctx, l.VendorID); err != nil {
		return err
	}
	if _, err := s.providers.Get(ctx, l.ProviderID); err != nil {
		return err
	}
	if err := normalizeLink(l); err != nil {
		return err
	}
	existing, err := s.vendors.ListSuppliers(ctx, &l.VendorID)
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.ProviderID == l.ProviderID {
			return fmt.Errorf("%w: 该供应商已在此厂商的供应商列表中", domain.ErrInvalidArgument)
		}
	}
	return s.vendors.CreateSupplier(ctx, l)
}

func (s *VendorService) GetSupplier(ctx context.Context, id int64) (*domain.VendorSupplier, error) {
	return s.vendors.GetSupplier(ctx, id)
}

func (s *VendorService) UpdateSupplier(ctx context.Context, l *domain.VendorSupplier) error {
	if err := normalizeLink(l); err != nil {
		return err
	}
	return s.vendors.UpdateSupplier(ctx, l)
}

// RemoveSupplier deletes a link that no model uses; otherwise disable it.
func (s *VendorService) RemoveSupplier(ctx context.Context, id int64) error {
	l, err := s.vendors.GetSupplier(ctx, id)
	if err != nil {
		return err
	}
	models, err := s.models.List(ctx)
	if err != nil {
		return err
	}
	for _, m := range models {
		if m.VendorID != nil && *m.VendorID == l.VendorID && m.ProviderID == l.ProviderID {
			return fmt.Errorf("%w: 该供应商仍在提供此厂商的模型，请改为停用", domain.ErrInvalidArgument)
		}
	}
	return s.vendors.DeleteSupplier(ctx, id)
}

// EnsureLink makes sure a vendor→supplier link exists (called when a model of
// vendorID is onboarded at providerID), creating an active one if missing.
func (s *VendorService) EnsureLink(ctx context.Context, vendorID, providerID int64) error {
	existing, err := s.vendors.ListSuppliers(ctx, &vendorID)
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.ProviderID == providerID {
			return nil
		}
	}
	return s.vendors.CreateSupplier(ctx, &domain.VendorSupplier{VendorID: vendorID, ProviderID: providerID, Priority: 100, Weight: 100, Status: "active"})
}
