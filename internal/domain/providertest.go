package domain

import (
	"context"
	"time"
)

// Supplier availability testing (供应商可用性测试): a run probes the
// supplier's API (connectivity + credential) and calls each of its models
// with a minimal request, recording pass/fail, latency and the error class.
// Runs are kept so the console can show a report per supplier.

const (
	TestTriggerManual    = "manual"
	TestTriggerBatch     = "batch"
	TestTriggerScheduled = "scheduled"

	TestRunRunning = "running"
	TestRunSuccess = "success" // every executed check passed
	TestRunPartial = "partial" // some checks failed
	TestRunFailed  = "failed"  // no check passed

	TestCheckProbe     = "probe"     // list models: connectivity + credential, no tokens spent
	TestCheckChat      = "chat"      // minimal chat call
	TestCheckEmbedding = "embedding" // minimal embeddings call

	TestPassed  = "passed"
	TestFailed  = "failed"
	TestSkipped = "skipped"
)

// Error classes of a failed check.
const (
	TestErrTimeout         = "timeout"
	TestErrNetwork         = "network"
	TestErrAuth            = "auth"
	TestErrNotFound        = "not_found"
	TestErrRateLimited     = "rate_limited"
	TestErrUpstream5xx     = "upstream_5xx"
	TestErrBadRequest      = "bad_request"
	TestErrInvalidResponse = "invalid_response"
	TestErrUnsupported     = "unsupported"
)

type ProviderTestRun struct {
	ID           int64                 `json:"id"`
	ProviderID   int64                 `json:"provider_id"`
	Trigger      string                `json:"trigger"`
	Operator     string                `json:"operator"`
	Protocol     string                `json:"protocol"`
	Status       string                `json:"status"`
	Total        int                   `json:"total"`
	Passed       int                   `json:"passed"`
	Failed       int                   `json:"failed"`
	Skipped      int                   `json:"skipped"`
	AvgLatencyMS int                   `json:"avg_latency_ms"`
	P95LatencyMS int                   `json:"p95_latency_ms"`
	Summary      string                `json:"summary"`
	StartedAt    time.Time             `json:"started_at"`
	FinishedAt   *time.Time            `json:"finished_at,omitempty"`
	Results      []*ProviderTestResult `json:"results,omitempty"`
}

type ProviderTestResult struct {
	ID               int64     `json:"id"`
	RunID            int64     `json:"run_id"`
	ProviderID       int64     `json:"provider_id"`
	ModelID          *int64    `json:"model_id,omitempty"`
	ModelKey         string    `json:"model_key"`
	CheckType        string    `json:"check_type"`
	Round            int       `json:"round"`
	Status           string    `json:"status"`
	LatencyMS        int       `json:"latency_ms"`
	HTTPStatus       int       `json:"http_status"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	ErrorType        string    `json:"error_type"`
	ErrorMessage     string    `json:"error_message"`
	ResponsePreview  string    `json:"response_preview"`
	CreatedAt        time.Time `json:"created_at"`
}

type ProviderTestRunQuery struct {
	PageQuery
	ProviderID int64
	Trigger    string
	Status     string
}

type ProviderTestRepository interface {
	CreateRun(ctx context.Context, r *ProviderTestRun) error
	// FinishRun stores the results and the run's final counters atomically.
	FinishRun(ctx context.Context, r *ProviderTestRun, results []*ProviderTestResult) error
	// GetRun returns the run with its results.
	GetRun(ctx context.Context, id int64) (*ProviderTestRun, error)
	ListRuns(ctx context.Context, q ProviderTestRunQuery) ([]*ProviderTestRun, int64, error)
	// RunsSince returns runs started at or after since (without results), newest first.
	RunsSince(ctx context.Context, providerID int64, since time.Time) ([]*ProviderTestRun, error)
	// ResultsSince returns check results recorded since the given time
	// (providerID 0 = every provider).
	ResultsSince(ctx context.Context, providerID int64, since time.Time) ([]*ProviderTestResult, error)
	// LatestRuns returns the newest run of every provider.
	LatestRuns(ctx context.Context) (map[int64]*ProviderTestRun, error)
	// RunningFor returns the provider's unfinished run started after since, if any.
	RunningFor(ctx context.Context, providerID int64, since time.Time) (*ProviderTestRun, error)
	// AbandonStale marks runs still "running" since before t as failed.
	AbandonStale(ctx context.Context, before time.Time) error
	PurgeBefore(ctx context.Context, before time.Time) (int64, error)
}

// ProviderTestSchedule is the scheduled availability patrol (定时巡检).
type ProviderTestSchedule struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"` // 5 .. 1440
	TimeoutSeconds  int  `json:"timeout_seconds"`  // per check, 5 .. 120
	AlertOnFailure  bool `json:"alert_on_failure"` // raise an availability alert when a run is not fully successful
	RetentionDays   int  `json:"retention_days"`   // test history kept, 7 .. 365
}

func DefaultProviderTestSchedule() ProviderTestSchedule {
	return ProviderTestSchedule{Enabled: false, IntervalMinutes: 60, TimeoutSeconds: 30, AlertOnFailure: true, RetentionDays: 90}
}
