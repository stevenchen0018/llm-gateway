package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/mock"
	"github.com/stevenchen/llm-gateway/internal/domain"
)

// --- in-memory fakes for domain ports, used only by this test file --------

type fakeModelRepo struct {
	models    map[int64]*domain.Model
	providers map[int64]*domain.Provider
	vendors   map[int64]*domain.Vendor
	links     map[[2]int64]*domain.VendorSupplier
}

// withVendor mirrors ModelRepo.attachVendors.
func (f *fakeModelRepo) withVendor(m *domain.Model) *domain.ModelWithProvider {
	mwp := &domain.ModelWithProvider{Model: *m, Provider: *f.providers[m.ProviderID]}
	if m.VendorID != nil {
		mwp.Vendor = f.vendors[*m.VendorID]
		mwp.Supply = f.links[[2]int64{*m.VendorID, m.ProviderID}]
	}
	return mwp
}

func (f *fakeModelRepo) Create(ctx context.Context, m *domain.Model) error { return nil }
func (f *fakeModelRepo) Update(ctx context.Context, m *domain.Model) error { return nil }
func (f *fakeModelRepo) Get(ctx context.Context, id int64) (*domain.Model, error) {
	m, ok := f.models[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return m, nil
}
func (f *fakeModelRepo) GetWithProvider(ctx context.Context, id int64) (*domain.ModelWithProvider, error) {
	m, ok := f.models[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return f.withVendor(m), nil
}
func (f *fakeModelRepo) List(ctx context.Context) ([]*domain.ModelWithProvider, error) {
	return nil, nil
}
func (f *fakeModelRepo) FindActiveByName(ctx context.Context, name string) ([]*domain.ModelWithProvider, error) {
	var out []*domain.ModelWithProvider
	for _, m := range f.models {
		if m.Status == domain.ModelStatusActive && (m.ModelKey == name || m.DisplayName == name) {
			if mwp := f.withVendor(m); mwp.Supply.Active() {
				out = append(out, mwp)
			}
		}
	}
	return out, nil
}

type fakeRouteRepo struct {
	byAlias map[string][]*domain.ModelRoute
}

func (f *fakeRouteRepo) Create(ctx context.Context, r *domain.ModelRoute) error { return nil }
func (f *fakeRouteRepo) Update(ctx context.Context, r *domain.ModelRoute) error { return nil }
func (f *fakeRouteRepo) Delete(ctx context.Context, id int64) error             { return nil }
func (f *fakeRouteRepo) ListByAlias(ctx context.Context, alias string) ([]*domain.ModelRoute, error) {
	return f.byAlias[alias], nil
}
func (f *fakeRouteRepo) Get(ctx context.Context, id int64) (*domain.ModelRoute, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeRouteRepo) ListForKey(ctx context.Context, alias string, keyID int64) ([]*domain.ModelRoute, error) {
	var out []*domain.ModelRoute
	for _, r := range f.byAlias[alias] {
		if r.APIKeyID == nil || *r.APIKeyID == keyID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRouteRepo) List(ctx context.Context) ([]*domain.ModelRoute, error) { return nil, nil }

type fakeRateLimiter struct {
	denyQPS bool
	denyTPM bool
}

func (f *fakeRateLimiter) AllowQPS(ctx context.Context, scope domain.RateLimitScope) (bool, error) {
	return !f.denyQPS, nil
}
func (f *fakeRateLimiter) ReserveTPM(ctx context.Context, scope domain.RateLimitScope, est int) (string, bool, error) {
	if f.denyTPM {
		return "", false, nil
	}
	return "resv", true, nil
}
func (f *fakeRateLimiter) SettleTPM(ctx context.Context, reservationID string, actual int) error {
	return nil
}
func (f *fakeRateLimiter) ReleaseTPM(ctx context.Context, reservationID string) error { return nil }

type fakeHealthStore struct {
	mu        sync.Mutex
	unhealthy map[int64]bool
}

func (f *fakeHealthStore) RecordResult(ctx context.Context, modelID int64, success bool) error {
	return nil
}
func (f *fakeHealthStore) IsHealthy(ctx context.Context, modelID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.unhealthy[modelID], nil
}

type fakeUsageRepo struct {
	mu      sync.Mutex
	records []domain.UsageRecord
}

func (f *fakeUsageRepo) Create(ctx context.Context, u *domain.UsageRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, *u)
	return nil
}
func (f *fakeUsageRepo) Summarize(ctx context.Context, q domain.UsageQuery) ([]domain.UsageSummary, error) {
	return nil, nil
}
func (f *fakeUsageRepo) List(ctx context.Context, q domain.UsageLogQuery) ([]domain.UsageRecord, int64, error) {
	return nil, 0, nil
}
func (f *fakeUsageRepo) len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

type fakeBudgetRepo struct {
	budgets map[int64]*domain.Budget
}

func (f *fakeBudgetRepo) Create(ctx context.Context, b *domain.Budget) error { return nil }
func (f *fakeBudgetRepo) Get(ctx context.Context, id int64) (*domain.Budget, error) {
	b, ok := f.budgets[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return b, nil
}
func (f *fakeBudgetRepo) GetByKeyID(ctx context.Context, keyID int64) (*domain.Budget, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeBudgetRepo) AddConsumed(ctx context.Context, id int64, delta decimal.Decimal) (*domain.Budget, error) {
	b, ok := f.budgets[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	b.Consumed = b.Consumed.Add(delta)
	return b, nil
}
func (f *fakeBudgetRepo) List(ctx context.Context) ([]*domain.Budget, error) { return nil, nil }
func (f *fakeBudgetRepo) Page(ctx context.Context, q domain.BudgetQuery) ([]*domain.Budget, int64, error) {
	return nil, 0, nil
}
func (f *fakeBudgetRepo) Update(ctx context.Context, b *domain.Budget) error { return nil }

type fakeAlertRepo struct{}

func (fakeAlertRepo) Create(ctx context.Context, a *domain.AlertEvent) error { return nil }
func (fakeAlertRepo) List(ctx context.Context, limit int, deptID *int64) ([]*domain.AlertEvent, error) {
	return nil, nil
}
func (fakeAlertRepo) Page(ctx context.Context, q domain.AlertQuery) ([]*domain.AlertEvent, int64, error) {
	return nil, 0, nil
}
func (fakeAlertRepo) MarkNotified(ctx context.Context, id int64, at time.Time) error { return nil }

type fakeMetricsRepo struct{}

func (fakeMetricsRepo) Upsert(ctx context.Context, d []domain.MetricDelta) error { return nil }
func (fakeMetricsRepo) TimeSeries(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy, step int) ([]domain.Series, error) {
	return nil, nil
}
func (fakeMetricsRepo) Aggregate(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy) ([]domain.AggRow, error) {
	return nil, nil
}
func (fakeMetricsRepo) Peaks(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy) ([]domain.PeakRow, error) {
	return nil, nil
}

type fakeNotifier struct{}

func (fakeNotifier) Notify(ctx context.Context, event domain.AlertEvent) error { return nil }

// failingModelClient wraps a real ProviderClient but forces an upstream
// error for one specific model ID, regardless of request content — used to
// exercise the failover path deterministically (the mock package's own
// FailKeyword can't do this alone, since every candidate for an alias
// receives the identical request).
type failingModelClient struct {
	inner       domain.ProviderClient
	failModelID int64
}

func (f *failingModelClient) ChatCompletion(ctx context.Context, target domain.CallTarget, req domain.ChatRequest) (domain.ChatResponse, error) {
	if target.Model.ID == f.failModelID {
		return domain.ChatResponse{}, errors.New("simulated upstream failure")
	}
	return f.inner.ChatCompletion(ctx, target, req)
}

func (f *failingModelClient) Embeddings(ctx context.Context, target domain.CallTarget, req domain.EmbeddingsRequest) (domain.EmbeddingsResponse, error) {
	if target.Model.ID == f.failModelID {
		return domain.EmbeddingsResponse{}, errors.New("simulated upstream failure")
	}
	return f.inner.Embeddings(ctx, target, req)
}

// --- test harness -----------------------------------------------------------

func newTestGateway(t *testing.T, limiter domain.RateLimiter, health domain.HealthStore, client domain.ProviderClient) (*GatewayService, *fakeUsageRepo) {
	t.Helper()

	provider := &domain.Provider{ID: 1, Code: "mock-provider", BaseURL: "mock://test"}
	modelPrimary := &domain.Model{ID: 10, ProviderID: 1, ModelKey: "primary-model", Status: domain.ModelStatusActive}
	modelBackup := &domain.Model{ID: 11, ProviderID: 1, ModelKey: "backup-model", Status: domain.ModelStatusActive}

	modelRepo := &fakeModelRepo{
		models:    map[int64]*domain.Model{10: modelPrimary, 11: modelBackup},
		providers: map[int64]*domain.Provider{1: provider},
	}
	routeRepo := &fakeRouteRepo{
		byAlias: map[string][]*domain.ModelRoute{
			"business-alias": {
				{Alias: "business-alias", CandidateModelID: 10, Priority: 0, Strategy: domain.RouteStrategyPriority},
				{Alias: "business-alias", CandidateModelID: 11, Priority: 1, Strategy: domain.RouteStrategyPriority},
			},
		},
	}

	routingSvc := NewRoutingService(routeRepo, modelRepo)
	registry := NewProviderClientRegistry(client, client)
	usageRepo := &fakeUsageRepo{}
	budgetRepo := &fakeBudgetRepo{budgets: map[int64]*domain.Budget{}}
	log := zap.NewNop()

	alertSvc := NewAlertService(fakeAlertRepo{}, fakeNotifier{}, log)
	budgetSvc := NewBudgetService(budgetRepo, alertSvc, 10000)
	usageWorker := NewAsyncUsageWorker(100, usageRepo, fakeMetricsRepo{}, budgetSvc, log)
	usageWorker.Start(context.Background(), 1)

	gw := NewGatewayService(routingSvc, limiter, health, registry, usageWorker, alertSvc, budgetSvc, log, 0)
	return gw, usageRepo
}

func TestGatewayChatCompletionSuccess(t *testing.T) {
	gw, _ := newTestGateway(t, &fakeRateLimiter{}, &fakeHealthStore{unhealthy: map[int64]bool{}}, mock.NewClient())

	key := domain.APIKey{ID: 1, Status: domain.APIKeyStatusActive}
	req := domain.ChatRequest{Model: "business-alias", Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}}

	resp, err := gw.ChatCompletion(context.Background(), key, req, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
}

func TestGatewayChatCompletionFailoverToBackup(t *testing.T) {
	client := &failingModelClient{inner: mock.NewClient(), failModelID: 10} // primary model always fails
	gw, usage := newTestGateway(t, &fakeRateLimiter{}, &fakeHealthStore{unhealthy: map[int64]bool{}}, client)

	key := domain.APIKey{ID: 1, Status: domain.APIKeyStatusActive}
	req := domain.ChatRequest{
		Model:    "business-alias",
		Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}},
	}

	resp, err := gw.ChatCompletion(context.Background(), key, req, "127.0.0.1")
	if err != nil {
		t.Fatalf("expected failover to backup to succeed, got error: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected a response from the backup candidate")
	}

	// Give the async worker a moment to persist both the failed attempt and
	// the successful one.
	waitFor(t, func() bool { return usage.len() >= 2 })
}

func TestGatewayChatCompletionAllRateLimited(t *testing.T) {
	gw, _ := newTestGateway(t, &fakeRateLimiter{denyQPS: true}, &fakeHealthStore{unhealthy: map[int64]bool{}}, mock.NewClient())

	key := domain.APIKey{ID: 1, Status: domain.APIKeyStatusActive}
	req := domain.ChatRequest{Model: "business-alias", Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}}

	_, err := gw.ChatCompletion(context.Background(), key, req, "127.0.0.1")
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestGatewayChatCompletionUnknownAlias(t *testing.T) {
	gw, _ := newTestGateway(t, &fakeRateLimiter{}, &fakeHealthStore{unhealthy: map[int64]bool{}}, mock.NewClient())

	key := domain.APIKey{ID: 1, Status: domain.APIKeyStatusActive}
	req := domain.ChatRequest{Model: "does-not-exist", Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}}

	_, err := gw.ChatCompletion(context.Background(), key, req, "127.0.0.1")
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}
