package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// MetricsRepo reads and writes the minute-level rollup (metrics_minute).
type MetricsRepo struct{ db *gorm.DB }

func NewMetricsRepo(db *gorm.DB) *MetricsRepo { return &MetricsRepo{db: db} }

// Upsert folds a batch of per-call deltas into their minute buckets.
func (r *MetricsRepo) Upsert(ctx context.Context, deltas []domain.MetricDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	// coalesce deltas that share a bucket key so one statement never touches
	// the same row twice (ON CONFLICT cannot affect a row twice per statement)
	type key struct {
		b time.Time
		k int64
		m int64
	}
	merged := map[key]*domain.MetricDelta{}
	order := make([]key, 0, len(deltas))
	for _, d := range deltas {
		k := key{d.Bucket.Truncate(time.Minute), d.KeyID, d.ModelID}
		if m, ok := merged[k]; ok {
			m.Requests += d.Requests
			m.Failed += d.Failed
			m.PromptTokens += d.PromptTokens
			m.CompletionTokens += d.CompletionTokens
			m.TotalTokens += d.TotalTokens
			m.LatencySumMS += d.LatencySumMS
			m.Cost = m.Cost.Add(d.Cost)
			m.ListCost = m.ListCost.Add(d.ListCost)
			continue
		}
		cp := d
		cp.Bucket = k.b
		merged[k] = &cp
		order = append(order, k)
	}

	var sb strings.Builder
	args := make([]any, 0, len(order)*12)
	sb.WriteString(`INSERT INTO metrics_minute (bucket,key_id,model_id,provider_id,requests,failed,prompt_tokens,completion_tokens,total_tokens,latency_sum_ms,cost,list_cost) VALUES `)
	for i, k := range order {
		d := merged[k]
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?,?,?,?,?,?,?,?,?,?,?,?)")
		args = append(args, d.Bucket, d.KeyID, d.ModelID, d.ProviderID, d.Requests, d.Failed, d.PromptTokens,
			d.CompletionTokens, d.TotalTokens, d.LatencySumMS, d.Cost, d.ListCost)
	}
	sb.WriteString(` ON CONFLICT (bucket, key_id, model_id) DO UPDATE SET
		requests = metrics_minute.requests + EXCLUDED.requests,
		failed = metrics_minute.failed + EXCLUDED.failed,
		prompt_tokens = metrics_minute.prompt_tokens + EXCLUDED.prompt_tokens,
		completion_tokens = metrics_minute.completion_tokens + EXCLUDED.completion_tokens,
		total_tokens = metrics_minute.total_tokens + EXCLUDED.total_tokens,
		latency_sum_ms = metrics_minute.latency_sum_ms + EXCLUDED.latency_sum_ms,
		cost = metrics_minute.cost + EXCLUDED.cost,
		list_cost = metrics_minute.list_cost + EXCLUDED.list_cost`)
	if err := r.db.WithContext(ctx).Exec(sb.String(), args...).Error; err != nil {
		return fmt.Errorf("upsert metrics: %w", err)
	}
	return nil
}

// groupExpr maps a dimension to the SQL expression identifying the group, plus
// any join needed to compute it. m is the metrics_minute alias.
func groupExpr(by domain.AggGroupBy) (expr string, join string, err error) {
	switch by {
	case domain.AggByModel:
		return "m.model_id::text", "", nil
	case domain.AggByKey:
		return "m.key_id::text", "", nil
	case domain.AggByProvider:
		return "m.provider_id::text", "", nil
	case domain.AggByApp:
		return "coalesce(k.app_id, 0)::text", "LEFT JOIN api_keys k ON k.id = m.key_id", nil
	case domain.AggByDepartment:
		return "coalesce(k.department_id, 0)::text", "LEFT JOIN api_keys k ON k.id = m.key_id", nil
	case domain.AggByVendor:
		return "coalesce(md.vendor_id, 0)::text", "JOIN models md ON md.id = m.model_id", nil
	case domain.AggByKeyKind:
		return "coalesce(k.category, 'application')", "LEFT JOIN api_keys k ON k.id = m.key_id", nil
	case domain.AggByCategory:
		return "md.category", "JOIN models md ON md.id = m.model_id", nil
	default:
		return "", "", fmt.Errorf("%w: unknown group_by %q", domain.ErrInvalidArgument, by)
	}
}

// filterSQL builds the WHERE clause and any extra joins the filter needs.
func filterSQL(f domain.MetricsFilter, existingJoin string) (where string, join string, args []any) {
	clauses := []string{"m.bucket >= ? AND m.bucket < ?"}
	args = []any{f.From, f.To}
	join = existingJoin
	if f.KeyID != nil {
		clauses = append(clauses, "m.key_id = ?")
		args = append(args, *f.KeyID)
	}
	if f.ModelID != nil {
		clauses = append(clauses, "m.model_id = ?")
		args = append(args, *f.ModelID)
	}
	if f.ProviderID != nil {
		clauses = append(clauses, "m.provider_id = ?")
		args = append(args, *f.ProviderID)
	}
	if f.AppID != nil {
		if !strings.Contains(join, "api_keys k") {
			join += " LEFT JOIN api_keys k ON k.id = m.key_id"
		}
		clauses = append(clauses, "k.app_id = ?")
		args = append(args, *f.AppID)
	}
	if f.DepartmentID != nil {
		clauses = append(clauses, "m.key_id IN (SELECT dk.id FROM api_keys dk WHERE dk.department_id = ?)")
		args = append(args, *f.DepartmentID)
	}
	if f.KeyCategory != "" {
		clauses = append(clauses, "m.key_id IN (SELECT ck.id FROM api_keys ck WHERE ck.category = ?)")
		args = append(args, f.KeyCategory)
	}
	if f.VendorID != nil {
		clauses = append(clauses, "m.model_id IN (SELECT vm.id FROM models vm WHERE vm.vendor_id = ?)")
		args = append(args, *f.VendorID)
	}
	if f.Category != "" {
		if !strings.Contains(join, "models md") {
			join += " JOIN models md ON md.id = m.model_id"
		}
		clauses = append(clauses, "md.category = ?")
		args = append(args, f.Category)
	}
	return strings.Join(clauses, " AND "), join, args
}

type seriesRow struct {
	Ts               time.Time
	GroupKey         string
	Requests         int64
	Failed           int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	LatencySumMS     int64
	Cost             decimal.Decimal
}

func (r *MetricsRepo) TimeSeries(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy, stepSeconds int) ([]domain.Series, error) {
	expr, join, err := groupExpr(by)
	if err != nil {
		return nil, err
	}
	if stepSeconds < 60 {
		stepSeconds = 60
	}
	where, join, args := filterSQL(f, join)
	q := fmt.Sprintf(`SELECT to_timestamp(floor(extract(epoch FROM m.bucket) / %d) * %d) AS ts,
		%s AS group_key,
		sum(m.requests) AS requests, sum(m.failed) AS failed,
		sum(m.prompt_tokens) AS prompt_tokens, sum(m.completion_tokens) AS completion_tokens,
		sum(m.total_tokens) AS total_tokens, sum(m.latency_sum_ms) AS latency_sum_ms, sum(m.cost) AS cost
		FROM metrics_minute m %s WHERE %s GROUP BY 1, 2 ORDER BY 2, 1`, stepSeconds, stepSeconds, expr, join, where)

	var rows []seriesRow
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("metrics timeseries: %w", err)
	}

	var out []domain.Series
	idx := map[string]int{}
	for _, row := range rows {
		i, ok := idx[row.GroupKey]
		if !ok {
			i = len(out)
			idx[row.GroupKey] = i
			out = append(out, domain.Series{GroupKey: row.GroupKey})
		}
		out[i].Points = append(out[i].Points, domain.SeriesPoint{
			Ts: row.Ts, Requests: row.Requests, Failed: row.Failed, PromptTokens: row.PromptTokens,
			CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
			LatencySumMS: row.LatencySumMS, Cost: row.Cost,
		})
	}
	return out, nil
}

type aggRow struct {
	GroupKey         string
	Requests         int64
	Failed           int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	LatencySumMS     int64
	Cost             decimal.Decimal
	ListCost         decimal.Decimal
}

func (r *MetricsRepo) Aggregate(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy) ([]domain.AggRow, error) {
	expr, join, err := groupExpr(by)
	if err != nil {
		return nil, err
	}
	where, join, args := filterSQL(f, join)
	q := fmt.Sprintf(`SELECT %s AS group_key,
		coalesce(sum(m.requests),0) AS requests, coalesce(sum(m.failed),0) AS failed,
		coalesce(sum(m.prompt_tokens),0) AS prompt_tokens, coalesce(sum(m.completion_tokens),0) AS completion_tokens,
		coalesce(sum(m.total_tokens),0) AS total_tokens, coalesce(sum(m.latency_sum_ms),0) AS latency_sum_ms,
		coalesce(sum(m.cost),0) AS cost, coalesce(sum(m.list_cost),0) AS list_cost
		FROM metrics_minute m %s WHERE %s GROUP BY 1 ORDER BY sum(m.cost) DESC, 1`, expr, join, where)

	var rows []aggRow
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("metrics aggregate: %w", err)
	}
	out := make([]domain.AggRow, len(rows))
	for i, x := range rows {
		out[i] = domain.AggRow{
			GroupKey: x.GroupKey, Requests: x.Requests, Failed: x.Failed, PromptTokens: x.PromptTokens,
			CompletionTokens: x.CompletionTokens, TotalTokens: x.TotalTokens, LatencySumMS: x.LatencySumMS,
			Cost: x.Cost, ListCost: x.ListCost,
		}
	}
	return out, nil
}

type peakRow struct {
	GroupKey string
	PeakRPM  int64
	PeakTPM  int64
	LastRPM  int64
	LastTPM  int64
}

// Peaks returns, per group, the busiest single minute in the window and the
// most recent minute — the inputs to capacity management.
func (r *MetricsRepo) Peaks(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy) ([]domain.PeakRow, error) {
	expr, join, err := groupExpr(by)
	if err != nil {
		return nil, err
	}
	where, join, args := filterSQL(f, join)
	q := fmt.Sprintf(`WITH per_min AS (
			SELECT %s AS group_key, m.bucket, sum(m.requests) AS rpm, sum(m.total_tokens) AS tpm
			FROM metrics_minute m %s WHERE %s GROUP BY 1, 2
		), last_min AS (
			SELECT DISTINCT ON (group_key) group_key, rpm, tpm FROM per_min ORDER BY group_key, bucket DESC
		)
		SELECT p.group_key, max(p.rpm) AS peak_rpm, max(p.tpm) AS peak_tpm,
			l.rpm AS last_rpm, l.tpm AS last_tpm
		FROM per_min p JOIN last_min l USING (group_key) GROUP BY p.group_key, l.rpm, l.tpm ORDER BY 1`, expr, join, where)

	var rows []peakRow
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("metrics peaks: %w", err)
	}
	out := make([]domain.PeakRow, len(rows))
	for i, x := range rows {
		out[i] = domain.PeakRow{GroupKey: x.GroupKey, PeakRPM: x.PeakRPM, PeakTPM: x.PeakTPM, LastRPM: x.LastRPM, LastTPM: x.LastTPM}
	}
	return out, nil
}
