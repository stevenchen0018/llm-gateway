package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type UsageStatus string

const (
	UsageStatusSuccess UsageStatus = "success"
	UsageStatusFailed  UsageStatus = "failed"
)

// UsageRecord is one gateway call's accounting row — the raw fact table
// behind every cost/observability dashboard described in the article
// (Key/model/provider/project cost boards, TPM/RPM time series, RT).
type UsageRecord struct {
	ID               int64           `json:"id"`
	RequestID        string          `json:"request_id"`
	KeyID            int64           `json:"key_id"`
	ModelID          int64           `json:"model_id"`
	ProviderID       int64           `json:"provider_id"`
	Alias            string          `json:"alias"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens"`
	Cost             decimal.Decimal `json:"cost"`
	ListCost         decimal.Decimal `json:"list_cost"` // price before vendor discount
	LatencyMS        int             `json:"latency_ms"`
	Status           UsageStatus     `json:"status"`
	ErrorCode        string          `json:"error_code,omitempty"`
	SourceIP         string          `json:"source_ip"`
	CreatedAt        time.Time       `json:"created_at"`
}

// UsageSummary is an aggregated slice used by dashboard endpoints.
type UsageSummary struct {
	Bucket       string          `json:"bucket"` // e.g. "2026-09-26" or "2026-09-26 14:32"
	GroupKey     string          `json:"group_key"`
	RequestCount int64           `json:"request_count"`
	FailedCount  int64           `json:"failed_count"`
	TotalTokens  int64           `json:"total_tokens"`
	Cost         decimal.Decimal `json:"cost"`
	AvgLatencyMS float64         `json:"avg_latency_ms"`
}

type UsageGroupBy string

const (
	UsageGroupByKey      UsageGroupBy = "key"
	UsageGroupByModel    UsageGroupBy = "model"
	UsageGroupByProvider UsageGroupBy = "provider"
)

type UsageQuery struct {
	From    time.Time
	To      time.Time
	GroupBy UsageGroupBy
	KeyID   *int64
	ModelID *int64
	DeptID  *int64
}

type UsageRepository interface {
	Create(ctx context.Context, u *UsageRecord) error
	Summarize(ctx context.Context, q UsageQuery) ([]UsageSummary, error)
	List(ctx context.Context, q UsageLogQuery) ([]UsageRecord, int64, error)
}

// UsageLogQuery filters the raw per-call log (调用日志).
type UsageLogQuery struct {
	From, To time.Time
	KeyID    *int64
	ModelID  *int64
	Status   string
	DeptID   *int64
	Page     int
	PageSize int
}
