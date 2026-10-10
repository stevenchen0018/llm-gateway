package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/metrics"
)

const (
	requestLogBatch      = 200
	requestLogFlushEvery = time.Second
)

// RequestLogWorker writes captured /v1 request records (prompts, responses,
// filter hits) in batches off the request path. Like AsyncUsageWorker it
// never applies backpressure: when the buffer is full a record is dropped and
// counted in llm_gateway_request_log_dropped_total.
type RequestLogWorker struct {
	ch       chan *domain.RequestLog
	repo     domain.RequestLogRepository
	settings *SettingsService
	log      *zap.Logger
	done     chan struct{}
	gate     TaskGate
}

// TaskGate elects which node runs a cluster-wide scheduled job: TryAcquire
// returns true on at most one node per ttl.
type TaskGate interface {
	TryAcquire(ctx context.Context, task string, ttl time.Duration) bool
}

// WithTaskGate makes the retention purge run on one node per period.
func (w *RequestLogWorker) WithTaskGate(g TaskGate) *RequestLogWorker {
	w.gate = g
	return w
}

func NewRequestLogWorker(bufferSize int, repo domain.RequestLogRepository, settings *SettingsService, log *zap.Logger) *RequestLogWorker {
	if bufferSize <= 0 {
		bufferSize = 10000
	}
	return &RequestLogWorker{ch: make(chan *domain.RequestLog, bufferSize), repo: repo, settings: settings, log: log, done: make(chan struct{})}
}

func (w *RequestLogWorker) Submit(l *domain.RequestLog) bool {
	select {
	case w.ch <- l:
		return true
	default:
		metrics.RequestLogDroppedTotal.Inc()
		return false
	}
}

// Start runs the batch writer until ctx ends (on shutdown it drains what is
// already buffered) and, when purge is set, the retention purge. In a cluster
// every node writes its own records but only the master purges.
func (w *RequestLogWorker) Start(ctx context.Context, purge bool) {
	go w.writer(ctx)
	if purge {
		go w.purger(ctx)
	}
}

// Wait blocks until the writer has drained after its context was cancelled,
// or until timeout.
func (w *RequestLogWorker) Wait(timeout time.Duration) {
	select {
	case <-w.done:
	case <-time.After(timeout):
	}
}

func (w *RequestLogWorker) writer(ctx context.Context) {
	defer close(w.done)
	t := time.NewTicker(requestLogFlushEvery)
	defer t.Stop()
	batch := make([]*domain.RequestLog, 0, requestLogBatch)
	flush := func(c context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := w.repo.CreateBatch(c, batch); err != nil {
			w.log.Error("persist request logs failed", zap.Error(err), zap.Int("records", len(batch)))
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case l := <-w.ch:
					batch = append(batch, l)
					if len(batch) >= requestLogBatch {
						flush(context.Background())
					}
				default:
					flush(context.Background())
					return
				}
			}
		case l := <-w.ch:
			batch = append(batch, l)
			if len(batch) >= requestLogBatch {
				flush(ctx)
			}
		case <-t.C:
			flush(ctx)
		}
	}
}

func (w *RequestLogWorker) purger(ctx context.Context) {
	run := func() {
		days := w.settings.Get(ctx).RequestLog.RetentionDays
		if days <= 0 {
			return
		}
		if w.gate != nil && !w.gate.TryAcquire(ctx, "request-log-purge", 5*time.Hour) {
			return // another node purged this period
		}
		n, err := w.repo.PurgeBefore(ctx, time.Now().AddDate(0, 0, -days))
		if err != nil && ctx.Err() == nil {
			w.log.Warn("purge request logs failed", zap.Error(err))
		} else if n > 0 {
			w.log.Info("purged expired request logs", zap.Int64("rows", n), zap.Int("retention_days", days))
		}
	}
	run()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// RequestLogService is the console's read side of the request records.
type RequestLogService struct {
	repo domain.RequestLogRepository
}

func NewRequestLogService(repo domain.RequestLogRepository) *RequestLogService {
	return &RequestLogService{repo: repo}
}

func (s *RequestLogService) List(ctx context.Context, q domain.RequestLogQuery) ([]*domain.RequestLog, int64, error) {
	return s.repo.List(ctx, q)
}

func (s *RequestLogService) Get(ctx context.Context, id int64) (*domain.RequestLog, error) {
	return s.repo.Get(ctx, id)
}

func (s *RequestLogService) GetByRequestID(ctx context.Context, rid string) (*domain.RequestLog, error) {
	return s.repo.GetByRequestID(ctx, rid)
}
