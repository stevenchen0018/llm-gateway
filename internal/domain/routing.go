package domain

import (
	"context"
	"time"
)

// RouteStrategy controls how multiple healthy candidates for the same alias
// are ordered before the first one is attempted.
type RouteStrategy string

const (
	// RouteStrategyPriority always prefers the lowest Priority value first.
	RouteStrategyPriority RouteStrategy = "priority"
	// RouteStrategyCostFirst prefers the cheapest candidate (by blended
	// input/output price) among those currently healthy.
	RouteStrategyCostFirst RouteStrategy = "cost_first"
	// RouteStrategyRoundRobin distributes load across candidates by Weight.
	RouteStrategyRoundRobin RouteStrategy = "round_robin"
	// RouteStrategySupplierPriority orders candidates by the vendor→supplier
	// priority configured for each target's supplier (供应商优先级).
	RouteStrategySupplierPriority RouteStrategy = "supplier_priority"
	// RouteStrategySupplierWeighted spreads load by the vendor→supplier
	// weight of each target's supplier (供应商权重).
	RouteStrategySupplierWeighted RouteStrategy = "supplier_weighted"
)

// ModelRoute is one candidate row for a business-facing model alias (e.g.
// "gpt-4-equivalent" -> several vendor-specific candidate models). Multiple
// rows sharing the same Alias form the failover/scheduling group described
// in the article's "模型调度" capability.
type ModelRoute struct {
	ID               int64         `json:"id"`
	Alias            string        `json:"alias"`
	CandidateModelID int64         `json:"candidate_model_id"`
	Priority         int           `json:"priority"` // lower = tried first under RouteStrategyPriority
	Weight           int           `json:"weight"`   // relative weight under RouteStrategyRoundRobin
	Strategy         RouteStrategy `json:"strategy"`
	Enabled          bool          `json:"enabled"`
	Name             string        `json:"name"`
	AppID            *int64        `json:"app_id,omitempty"`
	APIKeyID         *int64        `json:"api_key_id,omitempty"`       // nil = applies to every key
	SourceType       string        `json:"source_type"`                // custom | vendor
	SourceVendorID   *int64        `json:"source_vendor_id,omitempty"` // 厂商 (vendors.id) for vendor-type sources
	Remark           string        `json:"remark"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// RouteCandidate is a resolved, ready-to-call routing target.
type RouteCandidate struct {
	Route ModelRoute        `json:"route"`
	Model ModelWithProvider `json:"model"`
}

type RouteRepository interface {
	Create(ctx context.Context, r *ModelRoute) error
	Update(ctx context.Context, r *ModelRoute) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*ModelRoute, error)
	ListByAlias(ctx context.Context, alias string) ([]*ModelRoute, error)
	// ListForKey returns enabled routes for alias that are either global
	// (api_key_id IS NULL) or scoped to keyID.
	ListForKey(ctx context.Context, alias string, keyID int64) ([]*ModelRoute, error)
	List(ctx context.Context) ([]*ModelRoute, error)
}

// HealthStore records rolling error-rate state per candidate so the routing
// engine can implement the article's "分钟级容灾" (minute-level failover):
// a candidate that trips its error threshold is marked unhealthy and skipped
// until a cool-down window elapses.
type HealthStore interface {
	// RecordResult appends a success/failure sample for modelID's rolling
	// window.
	RecordResult(ctx context.Context, modelID int64, success bool) error
	// IsHealthy reports whether modelID is currently eligible for routing.
	IsHealthy(ctx context.Context, modelID int64) (bool, error)
}
