package postgres

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type UsageRepo struct{ db *gorm.DB }

func NewUsageRepo(db *gorm.DB) *UsageRepo { return &UsageRepo{db: db} }

func (r *UsageRepo) Create(ctx context.Context, u *domain.UsageRecord) error {
	row := UsageRecordRow{
		RequestID: u.RequestID, KeyID: u.KeyID, ModelID: u.ModelID, ProviderID: u.ProviderID,
		Alias: u.Alias, PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
		TotalTokens: u.TotalTokens, Cost: u.Cost, ListCost: u.ListCost, LatencyMS: u.LatencyMS,
		Status: string(u.Status), ErrorCode: u.ErrorCode, SourceIP: u.SourceIP,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create usage record: %w", err)
	}
	u.ID = row.ID
	u.CreatedAt = row.CreatedAt
	return nil
}

func fromUsageRow(r UsageRecordRow) domain.UsageRecord {
	return domain.UsageRecord{
		ID: r.ID, RequestID: r.RequestID, KeyID: r.KeyID, ModelID: r.ModelID, ProviderID: r.ProviderID,
		Alias: r.Alias, PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens,
		TotalTokens: r.TotalTokens, Cost: r.Cost, ListCost: r.ListCost, LatencyMS: r.LatencyMS,
		Status: domain.UsageStatus(r.Status), ErrorCode: r.ErrorCode, SourceIP: r.SourceIP, CreatedAt: r.CreatedAt,
	}
}

// List pages through the raw per-call log (调用日志).
func (r *UsageRepo) List(ctx context.Context, q domain.UsageLogQuery) ([]domain.UsageRecord, int64, error) {
	tx := r.db.WithContext(ctx).Model(&UsageRecordRow{}).Where("created_at >= ? AND created_at < ?", q.From, q.To)
	if q.KeyID != nil {
		tx = tx.Where("key_id = ?", *q.KeyID)
	}
	if q.ModelID != nil {
		tx = tx.Where("model_id = ?", *q.ModelID)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.DeptID != nil {
		tx = tx.Where("key_id IN (?)", deptKeyIDs(r.db, *q.DeptID))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count usage logs: %w", err)
	}
	page, size := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	var rows []UsageRecordRow
	if err := tx.Order("created_at desc, id desc").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list usage logs: %w", err)
	}
	out := make([]domain.UsageRecord, len(rows))
	for i, row := range rows {
		out[i] = fromUsageRow(row)
	}
	return out, total, nil
}

type summaryRow struct {
	Bucket       string
	GroupKey     int64
	RequestCount int64
	FailedCount  int64
	TotalTokens  int64
	Cost         decimal.Decimal
	AvgLatencyMS float64
}

// Summarize aggregates the minute rollup into daily buckets grouped by the
// requested dimension, powering the overview dashboard.
func (r *UsageRepo) Summarize(ctx context.Context, q domain.UsageQuery) ([]domain.UsageSummary, error) {
	groupCol, err := groupColumn(q.GroupBy)
	if err != nil {
		return nil, err
	}

	tx := r.db.WithContext(ctx).Table("metrics_minute").
		Select(fmt.Sprintf(
			`to_char(bucket, 'YYYY-MM-DD') AS bucket,
			 %s AS group_key,
			 coalesce(sum(requests), 0) AS request_count,
			 coalesce(sum(failed), 0) AS failed_count,
			 coalesce(sum(total_tokens), 0) AS total_tokens,
			 coalesce(sum(cost), 0) AS cost,
			 coalesce(sum(latency_sum_ms)::float8 / nullif(sum(requests), 0), 0) AS avg_latency_ms`, groupCol)).
		Where("bucket >= ? AND bucket < ?", q.From, q.To)

	if q.KeyID != nil {
		tx = tx.Where("key_id = ?", *q.KeyID)
	}
	if q.ModelID != nil {
		tx = tx.Where("model_id = ?", *q.ModelID)
	}
	if q.DeptID != nil {
		tx = tx.Where("key_id IN (?)", deptKeyIDs(r.db, *q.DeptID))
	}

	var rows []summaryRow
	if err := tx.Group("1, 2").Order("1, 2").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("summarize usage: %w", err)
	}

	out := make([]domain.UsageSummary, len(rows))
	for i, row := range rows {
		out[i] = domain.UsageSummary{
			Bucket: row.Bucket, GroupKey: fmt.Sprintf("%d", row.GroupKey),
			RequestCount: row.RequestCount, FailedCount: row.FailedCount, TotalTokens: row.TotalTokens,
			Cost: row.Cost, AvgLatencyMS: row.AvgLatencyMS,
		}
	}
	return out, nil
}

func groupColumn(g domain.UsageGroupBy) (string, error) {
	switch g {
	case domain.UsageGroupByKey:
		return "key_id", nil
	case domain.UsageGroupByModel:
		return "model_id", nil
	case domain.UsageGroupByProvider:
		return "provider_id", nil
	default:
		return "", fmt.Errorf("%w: unknown group_by %q", domain.ErrInvalidArgument, g)
	}
}
