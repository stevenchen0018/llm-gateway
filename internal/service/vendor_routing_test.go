package service

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// One vendor (DeepSeek) supplied by three suppliers.
func supplierFixture(strategy domain.VendorRouting) (*RoutingService, *fakeModelRepo) {
	vid := int64(50)
	vendor := &domain.Vendor{ID: vid, Code: "deepseek", Name: "DeepSeek", RoutingStrategy: strategy, Status: "active"}
	sup := func(id int64, code string, rate float64) *domain.Provider {
		return &domain.Provider{ID: id, Code: code, Status: domain.ProviderStatusActive, DiscountRate: decimal.NewFromFloat(rate)}
	}
	model := func(id, provider int64, price float64) *domain.Model {
		return &domain.Model{ID: id, ProviderID: provider, VendorID: &vid, ModelKey: "deepseek-r1", DisplayName: "deepseek-r1",
			Status: domain.ModelStatusActive, InputPricePer1K: decimal.NewFromFloat(price), OutputPricePer1K: decimal.NewFromFloat(price)}
	}
	repo := &fakeModelRepo{
		providers: map[int64]*domain.Provider{1: sup(1, "deepseek-official", 1), 2: sup(2, "aliyun", 0.9), 3: sup(3, "huawei", 0.5)},
		models:    map[int64]*domain.Model{11: model(11, 1, 0.01), 12: model(12, 2, 0.01), 13: model(13, 3, 0.01)},
		vendors:   map[int64]*domain.Vendor{vid: vendor},
		links: map[[2]int64]*domain.VendorSupplier{
			{vid, 1}: {VendorID: vid, ProviderID: 1, Priority: 1, Weight: 10, Status: "active"},
			{vid, 2}: {VendorID: vid, ProviderID: 2, Priority: 2, Weight: 10, Status: "active"},
			{vid, 3}: {VendorID: vid, ProviderID: 3, Priority: 3, Weight: 10, Status: "active"},
		},
	}
	return NewRoutingService(&fakeRouteRepo{byAlias: map[string][]*domain.ModelRoute{}}, repo), repo
}

func order(tr *Trace) []int64 {
	out := make([]int64, len(tr.Candidates))
	for i, c := range tr.Candidates {
		out[i] = c.Model.ID
	}
	return out
}

func TestDirectRoutingFollowsVendorSupplierStrategy(t *testing.T) {
	// cost first: huawei has the deepest discount
	svc, _ := supplierFixture(domain.VendorRoutingCostFirst)
	tr, err := svc.Explain(context.Background(), 1, "deepseek-r1")
	if err != nil {
		t.Fatal(err)
	}
	if got := order(tr); got[0] != 13 || tr.Strategy != domain.RouteStrategyCostFirst {
		t.Fatalf("cost_first: got %v (%s)", got, tr.Strategy)
	}

	// supplier priority: the official API is priority 1
	svc, _ = supplierFixture(domain.VendorRoutingPriority)
	tr, _ = svc.Explain(context.Background(), 1, "deepseek-r1")
	if got := order(tr); got[0] != 11 || got[1] != 12 || got[2] != 13 || tr.Strategy != domain.RouteStrategySupplierPriority {
		t.Fatalf("supplier_priority: got %v (%s)", got, tr.Strategy)
	}
}

func TestSupplierWeightedSpreadsByWeight(t *testing.T) {
	svc, repo := supplierFixture(domain.VendorRoutingWeighted)
	repo.links[[2]int64{50, 1}].Weight = 80
	repo.links[[2]int64{50, 2}].Weight = 15
	repo.links[[2]int64{50, 3}].Weight = 5
	first := map[int64]int{}
	for range 2000 {
		tr, err := svc.Explain(context.Background(), 1, "deepseek-r1")
		if err != nil {
			t.Fatal(err)
		}
		first[tr.Candidates[0].Model.ID]++
	}
	if first[11] < 1400 || first[11] > 1800 || first[13] > 250 {
		t.Fatalf("first-choice shares off for weights 80/15/5: %v", first)
	}
}

func TestDisabledSupplierLinkIsNeverRouted(t *testing.T) {
	svc, repo := supplierFixture(domain.VendorRoutingPriority)
	repo.links[[2]int64{50, 1}].Status = "disabled"
	tr, err := svc.Explain(context.Background(), 1, "deepseek-r1")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range tr.Candidates {
		if c.Model.ProviderID == 1 {
			t.Fatal("a supplier disabled for this vendor must not be a candidate")
		}
	}
	// ...and a policy pointing at it is skipped too
	routes := svc.routes.(*fakeRouteRepo)
	routes.byAlias["deepseek-r1"] = []*domain.ModelRoute{
		{ID: 1, Alias: "deepseek-r1", CandidateModelID: 11, Priority: 0, Strategy: domain.RouteStrategyPriority, Enabled: true},
		{ID: 2, Alias: "deepseek-r1", CandidateModelID: 12, Priority: 1, Strategy: domain.RouteStrategyPriority, Enabled: true},
	}
	tr, err = svc.Explain(context.Background(), 1, "deepseek-r1")
	if err != nil || len(tr.Candidates) != 1 || tr.Candidates[0].Model.ID != 12 {
		t.Fatalf("policy target on a disabled supplier must be skipped: %v %v", order(tr), err)
	}
}

func TestPinSupplierOrVendor(t *testing.T) {
	svc, _ := supplierFixture(domain.VendorRoutingCostFirst)
	tr, err := svc.Explain(context.Background(), 1, "aliyun/deepseek-r1")
	if err != nil || tr.PinKind != "supplier" || len(tr.Matched) != 1 || tr.Matched[0].Provider.Code != "aliyun" {
		t.Fatalf("supplier pin: %+v %v", tr, err)
	}
	tr, err = svc.Explain(context.Background(), 1, "deepseek/deepseek-r1")
	if err != nil || tr.PinKind != "vendor" || len(tr.Matched) != 3 {
		t.Fatalf("vendor pin should keep every supplier of the vendor: kind=%s matched=%d %v", tr.PinKind, len(tr.Matched), err)
	}
}

func TestPolicySupplierPriorityStrategy(t *testing.T) {
	svc, _ := supplierFixture(domain.VendorRoutingCostFirst)
	svc.routes.(*fakeRouteRepo).byAlias["deepseek-r1"] = []*domain.ModelRoute{
		{ID: 1, Alias: "deepseek-r1", CandidateModelID: 13, Priority: 0, Strategy: domain.RouteStrategySupplierPriority, Enabled: true},
		{ID: 2, Alias: "deepseek-r1", CandidateModelID: 11, Priority: 1, Strategy: domain.RouteStrategySupplierPriority, Enabled: true},
	}
	tr, err := svc.Explain(context.Background(), 1, "deepseek-r1")
	if err != nil {
		t.Fatal(err)
	}
	if got := order(tr); got[0] != 11 {
		t.Fatalf("supplier_priority policy should put the priority-1 supplier first, got %v", got)
	}
	if err := svc.CreateRoute(context.Background(), &domain.ModelRoute{Alias: "x", CandidateModelID: 11, Strategy: "fastest"}); err == nil {
		t.Fatal("unknown strategies must be rejected")
	}
}
