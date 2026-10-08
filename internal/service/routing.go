package service

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// RoutingService implements the gateway's request resolution:
//
//		业务域 --(unified API, model name)--> 厂商/供应商匹配 --> 模型调度 --> 供应商服务代理
//
//	 1. 厂商/供应商匹配: the model name is mapped to every onboarded offering
//	    of it — the vendor (厂商) that makes it and each supplier (供应商) that
//	    sells it. "supplier/model" pins a supplier, "vendor/model" a vendor.
//	    Suppliers disabled for that vendor are excluded.
//	 2. 模型调度: policies matching (API Key + model [+ vendor]) pick the
//	    targets; a key-scoped policy beats global ones. With no policy, the
//	    vendor's supplier strategy orders its suppliers (cost first, supplier
//	    priority, or supplier weight).
//	 3. 供应商服务代理: GatewayService walks the ordered candidates, failing
//	    over on error/unhealthy state.
//
// The service does not consult candidate health — GatewayService skips
// candidates domain.HealthStore reports unhealthy, keeping this pure DB
// logic and easy to unit test.
type RoutingService struct {
	routes domain.RouteRepository
	models domain.ModelRepository
}

func NewRoutingService(routes domain.RouteRepository, models domain.ModelRepository) *RoutingService {
	return &RoutingService{routes: routes, models: models}
}

// Scopes describing which rule produced the candidate list.
const (
	ScopeKey    = "key"    // policy bound to the calling API key
	ScopeGlobal = "global" // policy that applies to every key
	ScopeDirect = "direct" // no policy: matched models used as-is
)

// Trace is the full explanation of one resolution, rendered by the
// console's routing simulator.
type Trace struct {
	RequestedName string                     `json:"requested_name"`
	Vendor        string                     `json:"vendor"`   // pin prefix, if any
	PinKind       string                     `json:"pin_kind"` // supplier | vendor | ""
	ModelName     string                     `json:"model_name"`
	Matched       []domain.ModelWithProvider `json:"matched"`  // step 1: 厂商匹配
	Scope         string                     `json:"scope"`    // step 2: which policy tier hit
	Policies      []*domain.ModelRoute       `json:"policies"` // policies that matched
	Strategy      domain.RouteStrategy       `json:"strategy"`
	Candidates    []domain.RouteCandidate    `json:"candidates"` // step 3: ordered targets
}

func (s *RoutingService) CreateRoute(ctx context.Context, r *domain.ModelRoute) error {
	if r.Alias == "" || r.CandidateModelID == 0 {
		return fmt.Errorf("%w: source model and target model are required", domain.ErrInvalidArgument)
	}
	if _, err := s.models.Get(ctx, r.CandidateModelID); err != nil {
		return err
	}
	if r.Strategy == "" {
		r.Strategy = domain.RouteStrategyPriority
	}
	if err := validStrategy(r.Strategy); err != nil {
		return err
	}
	if r.SourceType == "" {
		r.SourceType = "custom"
	}
	if r.Weight <= 0 {
		r.Weight = 1
	}
	r.Enabled = true
	return s.routes.Create(ctx, r)
}

func (s *RoutingService) GetRoute(ctx context.Context, id int64) (*domain.ModelRoute, error) {
	return s.routes.Get(ctx, id)
}

func validStrategy(st domain.RouteStrategy) error {
	switch st {
	case domain.RouteStrategyPriority, domain.RouteStrategyCostFirst, domain.RouteStrategyRoundRobin,
		domain.RouteStrategySupplierPriority, domain.RouteStrategySupplierWeighted:
		return nil
	}
	return fmt.Errorf("%w: unknown strategy %q", domain.ErrInvalidArgument, st)
}

func (s *RoutingService) UpdateRoute(ctx context.Context, r *domain.ModelRoute) error {
	if r.Strategy != "" {
		if err := validStrategy(r.Strategy); err != nil {
			return err
		}
	}
	if r.CandidateModelID != 0 {
		if _, err := s.models.Get(ctx, r.CandidateModelID); err != nil {
			return err
		}
	}
	return s.routes.Update(ctx, r)
}

func (s *RoutingService) DeleteRoute(ctx context.Context, id int64) error {
	return s.routes.Delete(ctx, id)
}

func (s *RoutingService) ListRoutes(ctx context.Context) ([]*domain.ModelRoute, error) {
	return s.routes.List(ctx)
}

// Resolve returns the ordered candidate targets for keyID calling model name.
func (s *RoutingService) Resolve(ctx context.Context, keyID int64, name string) ([]domain.RouteCandidate, error) {
	t, err := s.Explain(ctx, keyID, name)
	if err != nil {
		return nil, err
	}
	return t.Candidates, nil
}

// Explain performs the resolution and returns every intermediate step.
func (s *RoutingService) Explain(ctx context.Context, keyID int64, name string) (*Trace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: model name is required", domain.ErrInvalidArgument)
	}
	t := &Trace{RequestedName: name, ModelName: name}
	if i := strings.Index(name, "/"); i > 0 {
		t.Vendor, t.ModelName = strings.ToLower(name[:i]), name[i+1:]
	}

	// 1. 厂商匹配
	found, err := s.models.FindActiveByName(ctx, t.ModelName)
	if err != nil {
		return nil, err
	}
	// a prefix names a supplier first; failing that, a vendor
	pinSupplier, pinVendor := false, false
	if t.Vendor != "" {
		for _, m := range found {
			if strings.EqualFold(m.Provider.Code, t.Vendor) {
				pinSupplier = true
			}
			if m.Vendor != nil && strings.EqualFold(m.Vendor.Code, t.Vendor) {
				pinVendor = true
			}
		}
		switch {
		case pinSupplier:
			t.PinKind = "supplier"
		case pinVendor:
			t.PinKind = "vendor"
		}
	}
	for _, m := range found {
		switch {
		case t.Vendor == "",
			pinSupplier && strings.EqualFold(m.Provider.Code, t.Vendor),
			!pinSupplier && pinVendor && m.Vendor != nil && strings.EqualFold(m.Vendor.Code, t.Vendor):
			t.Matched = append(t.Matched, *m)
		}
	}

	// 2. 模型调度: policies for this key first, then global ones
	routes, err := s.routes.ListForKey(ctx, t.ModelName, keyID)
	if err != nil {
		return nil, err
	}
	var keyScoped, global []*domain.ModelRoute
	for _, r := range routes {
		// a "厂商模型" source only applies when that vendor makes the model requested
		if r.SourceVendorID != nil && !matchesVendor(t.Matched, *r.SourceVendorID) {
			continue
		}
		if r.APIKeyID != nil {
			keyScoped = append(keyScoped, r)
		} else {
			global = append(global, r)
		}
	}
	switch {
	case len(keyScoped) > 0:
		t.Scope, t.Policies = ScopeKey, keyScoped
	case len(global) > 0:
		t.Scope, t.Policies = ScopeGlobal, global
	default:
		t.Scope = ScopeDirect
	}

	// 3. 厂商服务代理 targets
	if t.Scope == ScopeDirect {
		for _, m := range t.Matched {
			t.Candidates = append(t.Candidates, domain.RouteCandidate{
				Route: domain.ModelRoute{Alias: t.ModelName, CandidateModelID: m.ID, Weight: 1, Strategy: domain.RouteStrategyCostFirst, Enabled: true, SourceType: "vendor"},
				Model: m,
			})
		}
		t.Strategy = directStrategy(t.Matched)
	} else {
		for _, r := range t.Policies {
			mwp, err := s.models.GetWithProvider(ctx, r.CandidateModelID)
			if err != nil || mwp.Status != domain.ModelStatusActive || mwp.Provider.Status == domain.ProviderStatusDisabled ||
				!mwp.Supply.Active() || (mwp.Vendor != nil && mwp.Vendor.Status == "disabled") {
				continue // target (or its supplier for this vendor) was disabled after the policy was created
			}
			t.Candidates = append(t.Candidates, domain.RouteCandidate{Route: *r, Model: *mwp})
		}
		t.Strategy = t.Policies[0].Strategy
	}
	if len(t.Candidates) == 0 {
		return t, domain.ErrNotFound
	}
	orderCandidates(t.Candidates, t.Strategy)
	return t, nil
}

func matchesVendor(matched []domain.ModelWithProvider, vendorID int64) bool {
	for _, m := range matched {
		if m.VendorID != nil && *m.VendorID == vendorID {
			return true
		}
	}
	return false
}

// directStrategy is the supplier strategy of the vendor behind the matched
// offerings; offerings from several vendors (or none) fall back to cost first.
func directStrategy(matched []domain.ModelWithProvider) domain.RouteStrategy {
	var vendor *domain.Vendor
	for _, m := range matched {
		if m.Vendor == nil || (vendor != nil && vendor.ID != m.Vendor.ID) {
			return domain.RouteStrategyCostFirst
		}
		vendor = m.Vendor
	}
	if vendor == nil {
		return domain.RouteStrategyCostFirst
	}
	switch vendor.RoutingStrategy {
	case domain.VendorRoutingPriority:
		return domain.RouteStrategySupplierPriority
	case domain.VendorRoutingWeighted:
		return domain.RouteStrategySupplierWeighted
	default:
		return domain.RouteStrategyCostFirst
	}
}

func supplyPriority(c domain.RouteCandidate) int {
	if c.Model.Supply == nil {
		return 100
	}
	return c.Model.Supply.Priority
}

func supplyWeight(c domain.RouteCandidate) int {
	if c.Model.Supply == nil {
		return 100
	}
	return c.Model.Supply.Weight
}

// effectivePrice is the blended per-1K price after the vendor discount; a
// zero/unset discount rate means "no discount".
func effectivePrice(c domain.RouteCandidate) decimal.Decimal {
	price := c.Model.InputPricePer1K.Add(c.Model.OutputPricePer1K)
	rate := c.Model.Provider.DiscountRate
	if rate.IsZero() {
		return price
	}
	return price.Mul(rate)
}

func orderCandidates(candidates []domain.RouteCandidate, strategy domain.RouteStrategy) {
	switch strategy {
	case domain.RouteStrategyCostFirst:
		sort.SliceStable(candidates, func(i, j int) bool {
			return effectivePrice(candidates[i]).LessThan(effectivePrice(candidates[j]))
		})
	case domain.RouteStrategyRoundRobin:
		weightedShuffle(candidates, func(c domain.RouteCandidate) int { return c.Route.Weight })
	case domain.RouteStrategySupplierPriority:
		sort.SliceStable(candidates, func(i, j int) bool {
			pi, pj := supplyPriority(candidates[i]), supplyPriority(candidates[j])
			if pi != pj {
				return pi < pj
			}
			return effectivePrice(candidates[i]).LessThan(effectivePrice(candidates[j]))
		})
	case domain.RouteStrategySupplierWeighted:
		weightedShuffle(candidates, supplyWeight)
	default: // domain.RouteStrategyPriority
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].Route.Priority < candidates[j].Route.Priority
		})
	}
}

// weightedShuffle orders candidates by drawing without replacement from a
// weighted distribution (higher Weight -> more likely to be tried first),
// giving round-robin-style load spreading across healthy candidates without
// needing any shared counter state.
func weightedShuffle(candidates []domain.RouteCandidate, weightOf func(domain.RouteCandidate) int) {
	remaining := append([]domain.RouteCandidate(nil), candidates...)
	ordered := make([]domain.RouteCandidate, 0, len(candidates))

	for len(remaining) > 0 {
		total := 0
		for _, c := range remaining {
			w := weightOf(c)
			if w <= 0 {
				w = 1
			}
			total += w
		}

		pick := rand.Intn(total)
		acc := 0
		idx := len(remaining) - 1
		for i, c := range remaining {
			w := weightOf(c)
			if w <= 0 {
				w = 1
			}
			acc += w
			if pick < acc {
				idx = i
				break
			}
		}

		ordered = append(ordered, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}

	copy(candidates, ordered)
}
