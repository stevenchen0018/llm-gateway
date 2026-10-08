package service

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

func i64(v int64) *int64 { return &v }

func policyFixture() (*RoutingService, map[string]*domain.Model) {
	pa := &domain.Provider{ID: 1, Code: "aliyun", Status: domain.ProviderStatusActive, DiscountRate: decimal.NewFromFloat(1)}
	pb := &domain.Provider{ID: 2, Code: "huawei", Status: domain.ProviderStatusActive, DiscountRate: decimal.NewFromFloat(0.5)} // 50% discount
	models := map[string]*domain.Model{
		"ali-r1":    {ID: 1, ProviderID: 1, ModelKey: "deepseek-r1", DisplayName: "deepseek-r1", Status: domain.ModelStatusActive, InputPricePer1K: decimal.NewFromFloat(0.004), OutputPricePer1K: decimal.NewFromFloat(0.016)},
		"huawei-r1": {ID: 2, ProviderID: 2, ModelKey: "deepseek-r1", DisplayName: "deepseek-r1", Status: domain.ModelStatusActive, InputPricePer1K: decimal.NewFromFloat(0.006), OutputPricePer1K: decimal.NewFromFloat(0.018)},
		"qwen":      {ID: 3, ProviderID: 1, ModelKey: "qwen-max", DisplayName: "qwen-max", Status: domain.ModelStatusActive},
	}
	modelRepo := &fakeModelRepo{
		models:    map[int64]*domain.Model{1: models["ali-r1"], 2: models["huawei-r1"], 3: models["qwen"]},
		providers: map[int64]*domain.Provider{1: pa, 2: pb},
	}
	routeRepo := &fakeRouteRepo{byAlias: map[string][]*domain.ModelRoute{
		"deepseek-r1": {
			{ID: 10, Alias: "deepseek-r1", CandidateModelID: 3, Priority: 0, Strategy: domain.RouteStrategyPriority, Enabled: true}, // global → qwen
			{ID: 11, Alias: "deepseek-r1", CandidateModelID: 1, Priority: 0, Strategy: domain.RouteStrategyPriority, Enabled: true, APIKeyID: i64(7)},
		},
	}}
	return NewRoutingService(routeRepo, modelRepo), models
}

func TestExplainKeyScopedPolicyBeatsGlobal(t *testing.T) {
	svc, _ := policyFixture()

	tr, err := svc.Explain(context.Background(), 7, "deepseek-r1")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Scope != ScopeKey || len(tr.Candidates) != 1 || tr.Candidates[0].Model.ID != 1 {
		t.Fatalf("key 7 should hit its own policy → model 1, got scope=%s candidates=%d", tr.Scope, len(tr.Candidates))
	}

	tr, err = svc.Explain(context.Background(), 8, "deepseek-r1")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Scope != ScopeGlobal || tr.Candidates[0].Model.ID != 3 {
		t.Fatalf("other keys should fall back to the global policy → model 3, got scope=%s", tr.Scope)
	}
	if len(tr.Matched) != 2 {
		t.Fatalf("厂商匹配 should find both vendors serving deepseek-r1, got %d", len(tr.Matched))
	}
}

func TestExplainDirectPrefersCheapestAfterDiscount(t *testing.T) {
	svc, _ := policyFixture()

	// no policy for "qwen-max"'s siblings here: use a name with no route → direct
	tr, err := svc.Explain(context.Background(), 1, "qwen-max")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Scope != ScopeDirect || len(tr.Candidates) != 1 {
		t.Fatalf("expected direct resolution, got %s", tr.Scope)
	}

	// a "vendor/model" request pins the vendor
	tr, err = svc.Explain(context.Background(), 8, "huawei/deepseek-r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Matched) != 1 || tr.Matched[0].Provider.Code != "huawei" {
		t.Fatalf("vendor prefix should pin huawei, got %+v", tr.Matched)
	}
}

func TestDirectOrderingUsesDiscountedPrice(t *testing.T) {
	_, models := policyFixture()
	aliProv := domain.Provider{ID: 1, DiscountRate: decimal.NewFromFloat(1)}
	hwProv := domain.Provider{ID: 2, DiscountRate: decimal.NewFromFloat(0.5)}
	cands := []domain.RouteCandidate{
		{Model: domain.ModelWithProvider{Model: *models["ali-r1"], Provider: aliProv}},   // 0.020
		{Model: domain.ModelWithProvider{Model: *models["huawei-r1"], Provider: hwProv}}, // 0.024 * 0.5 = 0.012
	}
	orderCandidates(cands, domain.RouteStrategyCostFirst)
	if cands[0].Model.ID != 2 {
		t.Fatalf("discounted huawei (0.012) should rank before aliyun (0.020), got model %d first", cands[0].Model.ID)
	}
}

func TestApproverLevelByAmount(t *testing.T) {
	svc := NewBudgetService(&fakeBudgetRepo{}, nil, 10000)
	if got := svc.ApproverLevelFor(decimal.NewFromInt(9999)); got != ApproverD {
		t.Errorf("9999 should go to D, got %s", got)
	}
	if got := svc.ApproverLevelFor(decimal.NewFromInt(10000)); got != ApproverD {
		t.Errorf("limit itself stays with D, got %s", got)
	}
	if got := svc.ApproverLevelFor(decimal.NewFromInt(10001)); got != ApproverCTO {
		t.Errorf("above the limit should go to CTO, got %s", got)
	}
}

func TestApplyDiscount(t *testing.T) {
	list := decimal.NewFromInt(100)
	if got := ApplyDiscount(list, decimal.NewFromFloat(0.85)); !got.Equal(decimal.NewFromInt(85)) {
		t.Errorf("0.85 discount: got %s", got)
	}
	if got := ApplyDiscount(list, decimal.Zero); !got.Equal(list) {
		t.Errorf("zero rate means no discount, got %s", got)
	}
}

func TestAutoStepKeepsChartsBounded(t *testing.T) {
	cases := []struct {
		hours float64
		want  int
	}{{3, 60}, {12, 60}, {24, 300}, {24 * 7, 900}, {24 * 30, 3600}}
	for _, c := range cases {
		from := timeAt(0)
		to := from.Add(hoursDur(c.hours))
		if got := AutoStep(from, to); got != c.want {
			t.Errorf("%vh: step %d, want %d", c.hours, got, c.want)
		}
		if n := int(to.Sub(from).Seconds()) / AutoStep(from, to); n > maxBuckets {
			t.Errorf("%vh: %d buckets exceeds cap %d", c.hours, n, maxBuckets)
		}
	}
}
