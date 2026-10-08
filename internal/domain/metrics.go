package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// MetricDelta is one call's contribution to a minute bucket.
type MetricDelta struct {
	Bucket           time.Time
	KeyID            int64
	ModelID          int64
	ProviderID       int64
	Requests         int64
	Failed           int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	LatencySumMS     int64
	Cost             decimal.Decimal
	ListCost         decimal.Decimal
}

type AggGroupBy string

const (
	AggByModel      AggGroupBy = "model"
	AggByKey        AggGroupBy = "key"
	AggByProvider   AggGroupBy = "provider"
	AggByApp        AggGroupBy = "app"
	AggByCategory   AggGroupBy = "category"
	AggByDepartment AggGroupBy = "department"
	AggByVendor     AggGroupBy = "vendor"       // 厂商 (model maker)
	AggByKeyKind    AggGroupBy = "key_category" // personal | application
)

// MetricsFilter narrows a metrics query. Nil/empty fields do not filter.
type MetricsFilter struct {
	From, To     time.Time
	KeyID        *int64
	ModelID      *int64
	ProviderID   *int64
	AppID        *int64
	DepartmentID *int64
	Category     string
	VendorID     *int64
	KeyCategory  string // personal | application
}

type SeriesPoint struct {
	Ts               time.Time       `json:"ts"`
	Requests         int64           `json:"requests"`
	Failed           int64           `json:"failed"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	TotalTokens      int64           `json:"total_tokens"`
	LatencySumMS     int64           `json:"latency_sum_ms"`
	Cost             decimal.Decimal `json:"cost"`
}

type Series struct {
	GroupKey string        `json:"group_key"`
	Points   []SeriesPoint `json:"points"`
}

// AggRow is a whole-range aggregate for one group (ranking / cost boards).
type AggRow struct {
	GroupKey         string          `json:"group_key"`
	Requests         int64           `json:"requests"`
	Failed           int64           `json:"failed"`
	PromptTokens     int64           `json:"prompt_tokens"`
	CompletionTokens int64           `json:"completion_tokens"`
	TotalTokens      int64           `json:"total_tokens"`
	LatencySumMS     int64           `json:"latency_sum_ms"`
	Cost             decimal.Decimal `json:"cost"`
	ListCost         decimal.Decimal `json:"list_cost"`
}

// PeakRow is the busiest minute observed for a group (capacity management).
type PeakRow struct {
	GroupKey string `json:"group_key"`
	PeakRPM  int64  `json:"peak_rpm"`
	PeakTPM  int64  `json:"peak_tpm"`
	// LastRPM/LastTPM are the most recent minute bucket in the window.
	LastRPM int64 `json:"last_rpm"`
	LastTPM int64 `json:"last_tpm"`
}

// MetricsRepository is the minute-level rollup store behind every
// monitoring and cost dashboard.
type MetricsRepository interface {
	Upsert(ctx context.Context, deltas []MetricDelta) error
	TimeSeries(ctx context.Context, f MetricsFilter, by AggGroupBy, stepSeconds int) ([]Series, error)
	Aggregate(ctx context.Context, f MetricsFilter, by AggGroupBy) ([]AggRow, error)
	Peaks(ctx context.Context, f MetricsFilter, by AggGroupBy) ([]PeakRow, error)
}
