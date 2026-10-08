// Package service implements the gateway's business logic — the layer
// between HTTP handlers (internal/api) and persistence/cache adapters
// (internal/adapter). Each Service in this package corresponds to one
// capability described in the article.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ModelMarketService implements "模型纳管" (onboarding) and "模型市场"
// (marketplace listing + try-before-you-buy experience) from the article:
// centralized provider/model management plus a one-call experience/eval
// endpoint so business teams can try a model before committing to it.
type ModelMarketService struct {
	providers domain.ProviderRepository
	models    domain.ModelRepository
	clients   *ProviderClientRegistry
	vendors   *VendorService
}

// WithVendors enables vendor validation and automatic vendor→supplier links.
func (s *ModelMarketService) WithVendors(v *VendorService) *ModelMarketService {
	s.vendors = v
	return s
}

func validSupplierType(t string) error {
	switch t {
	case "", domain.SupplierOfficial, domain.SupplierCloud, domain.SupplierReseller, domain.SupplierSelfHosted:
		return nil
	}
	return fmt.Errorf("%w: supplier_type must be official, cloud, reseller or self_hosted", domain.ErrInvalidArgument)
}

// linkVendor validates the model's vendor and records that its supplier
// supplies that vendor.
func (s *ModelMarketService) linkVendor(ctx context.Context, m *domain.Model) error {
	if m.VendorID == nil || s.vendors == nil {
		return nil
	}
	if _, err := s.vendors.Get(ctx, *m.VendorID); err != nil {
		return err
	}
	return s.vendors.EnsureLink(ctx, *m.VendorID, m.ProviderID)
}

func NewModelMarketService(providers domain.ProviderRepository, models domain.ModelRepository, clients *ProviderClientRegistry) *ModelMarketService {
	return &ModelMarketService{providers: providers, models: models, clients: clients}
}

func (s *ModelMarketService) CreateProvider(ctx context.Context, p *domain.Provider) error {
	if p.Code == "" || p.BaseURL == "" {
		return fmt.Errorf("%w: provider code and base_url are required", domain.ErrInvalidArgument)
	}
	if err := validSupplierType(p.SupplierType); err != nil {
		return err
	}
	if p.Status == "" {
		p.Status = domain.ProviderStatusActive
	}
	return s.providers.Create(ctx, p)
}

func (s *ModelMarketService) UpdateProvider(ctx context.Context, p *domain.Provider) error {
	if err := validSupplierType(p.SupplierType); err != nil {
		return err
	}
	return s.providers.Update(ctx, p)
}

func (s *ModelMarketService) ListProviders(ctx context.Context) ([]*domain.Provider, error) {
	return s.providers.List(ctx)
}

func (s *ModelMarketService) GetProvider(ctx context.Context, id int64) (*domain.Provider, error) {
	return s.providers.Get(ctx, id)
}

func (s *ModelMarketService) CreateModel(ctx context.Context, m *domain.Model) error {
	if m.ProviderID == 0 || m.ModelKey == "" {
		return fmt.Errorf("%w: provider_id and model_key are required", domain.ErrInvalidArgument)
	}
	if _, err := s.providers.Get(ctx, m.ProviderID); err != nil {
		return err
	}
	if m.Type == "" {
		m.Type = domain.ModelTypeChat
	}
	if m.Status == "" {
		m.Status = domain.ModelStatusActive
	}
	if err := s.linkVendor(ctx, m); err != nil {
		return err
	}
	return s.models.Create(ctx, m)
}

func (s *ModelMarketService) UpdateModel(ctx context.Context, m *domain.Model) error {
	if err := s.linkVendor(ctx, m); err != nil {
		return err
	}
	return s.models.Update(ctx, m)
}

// ListMarket returns every onboarded model joined with its provider — the
// "AI应用商店" listing surface, 100% of internal/external models in one
// place per the article's coverage goal.
func (s *ModelMarketService) ListMarket(ctx context.Context) ([]*domain.ModelWithProvider, error) {
	return s.models.List(ctx)
}

func (s *ModelMarketService) GetModel(ctx context.Context, id int64) (*domain.ModelWithProvider, error) {
	return s.models.GetWithProvider(ctx, id)
}

// TryModel implements "模型体验": a single, direct (no routing/quota/cost
// bookkeeping) call against one specific model, letting a business user
// evaluate it in minutes instead of the multi-day manual process the
// article cites as the pre-gateway baseline.
func (s *ModelMarketService) TryModel(ctx context.Context, modelID int64, prompt string) (domain.ChatResponse, error) {
	mwp, err := s.models.GetWithProvider(ctx, modelID)
	if err != nil {
		return domain.ChatResponse{}, err
	}

	client := s.clients.For(mwp.Provider)
	target := domain.CallTarget{Provider: mwp.Provider, Model: mwp.Model}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	return client.ChatCompletion(ctx, target, domain.ChatRequest{
		Model:    mwp.ModelKey,
		Messages: []domain.ChatMessage{{Role: "user", Content: prompt}},
	})
}
