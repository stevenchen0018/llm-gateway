package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// UsageJob is one accounting write to perform off the request hot path.
type UsageJob struct {
	Record   domain.UsageRecord
	BudgetID *int64
}

// metricsFlushInterval bounds how stale the minute-level dashboards can be.
const metricsFlushInterval = time.Second

// AsyncUsageWorker persists usage_records, folds every call into the
// minute-level rollup (metrics_minute) and posts cost against budgets in a
// bounded pool of background goroutines, so the request path (which already
// has its answer once the provider call returns) never blocks on database
// writes. The rollup is the in-process equivalent of the article's Flink
// real-time aggregation: monitoring and cost boards read it, giving
// minute-level observability. A full buffer drops the job with a logged
// warning rather than applying backpressure to live traffic — accounting
// completeness is traded for latency/availability under extreme overload.
type AsyncUsageWorker struct {
	jobs    chan UsageJob
	usage   domain.UsageRepository
	metrics domain.MetricsRepository
	budgets *BudgetService
	log     *zap.Logger

	mu      sync.Mutex
	pending []domain.MetricDelta
}

func NewAsyncUsageWorker(bufferSize int, usage domain.UsageRepository, metrics domain.MetricsRepository, budgets *BudgetService, log *zap.Logger) *AsyncUsageWorker {
	return &AsyncUsageWorker{
		jobs:    make(chan UsageJob, bufferSize),
		usage:   usage,
		metrics: metrics,
		budgets: budgets,
		log:     log,
	}
}

func (w *AsyncUsageWorker) Start(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		go w.loop(ctx)
	}
	go w.flusher(ctx)
}

func (w *AsyncUsageWorker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.jobs:
			w.process(ctx, job)
		}
	}
}

func (w *AsyncUsageWorker) flusher(ctx context.Context) {
	t := time.NewTicker(metricsFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.flush(context.Background())
			return
		case <-t.C:
			w.flush(ctx)
		}
	}
}

func (w *AsyncUsageWorker) flush(ctx context.Context) {
	w.mu.Lock()
	batch := w.pending
	w.pending = nil
	w.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := w.metrics.Upsert(ctx, batch); err != nil {
		w.log.Error("flush minute metrics failed", zap.Error(err), zap.Int("deltas", len(batch)))
	}
}

func (w *AsyncUsageWorker) process(ctx context.Context, job UsageJob) {
	rec := job.Record
	if err := w.usage.Create(ctx, &rec); err != nil {
		w.log.Error("persist usage record failed", zap.Error(err), zap.String("request_id", rec.RequestID))
	}

	d := domain.MetricDelta{
		Bucket: time.Now().Truncate(time.Minute), KeyID: rec.KeyID, ModelID: rec.ModelID, ProviderID: rec.ProviderID,
		Requests: 1, PromptTokens: int64(rec.PromptTokens), CompletionTokens: int64(rec.CompletionTokens),
		TotalTokens: int64(rec.TotalTokens), LatencySumMS: int64(rec.LatencyMS), Cost: rec.Cost, ListCost: rec.ListCost,
	}
	if rec.Status == domain.UsageStatusFailed {
		d.Failed = 1
	}
	w.mu.Lock()
	w.pending = append(w.pending, d)
	w.mu.Unlock()

	if job.BudgetID != nil && rec.Status == domain.UsageStatusSuccess {
		if err := w.budgets.PostCost(ctx, *job.BudgetID, rec.Cost); err != nil {
			w.log.Error("post cost to budget failed", zap.Error(err), zap.Int64("budget_id", *job.BudgetID))
		}
	}
}

// Submit enqueues job without blocking. Returns false if the buffer was
// full and the job was dropped (logged internally).
func (w *AsyncUsageWorker) Submit(job UsageJob) bool {
	select {
	case w.jobs <- job:
		return true
	default:
		w.log.Warn("usage job buffer full, dropping accounting record", zap.String("request_id", job.Record.RequestID))
		return false
	}
}
