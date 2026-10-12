package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type ProviderTestRunRow struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	ProviderID   int64      `gorm:"column:provider_id"`
	Trigger      string     `gorm:"column:trigger"`
	Operator     string     `gorm:"column:operator"`
	Protocol     string     `gorm:"column:protocol"`
	Status       string     `gorm:"column:status"`
	Total        int        `gorm:"column:total"`
	Passed       int        `gorm:"column:passed"`
	Failed       int        `gorm:"column:failed"`
	Skipped      int        `gorm:"column:skipped"`
	AvgLatencyMS int        `gorm:"column:avg_latency_ms"`
	P95LatencyMS int        `gorm:"column:p95_latency_ms"`
	Summary      string     `gorm:"column:summary"`
	StartedAt    time.Time  `gorm:"column:started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at"`
}

func (ProviderTestRunRow) TableName() string { return "provider_test_runs" }

type ProviderTestResultRow struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	RunID            int64     `gorm:"column:run_id"`
	ProviderID       int64     `gorm:"column:provider_id"`
	ModelID          *int64    `gorm:"column:model_id"`
	ModelKey         string    `gorm:"column:model_key"`
	CheckType        string    `gorm:"column:check_type"`
	Round            int       `gorm:"column:round"`
	Status           string    `gorm:"column:status"`
	LatencyMS        int       `gorm:"column:latency_ms"`
	HTTPStatus       int       `gorm:"column:http_status"`
	PromptTokens     int       `gorm:"column:prompt_tokens"`
	CompletionTokens int       `gorm:"column:completion_tokens"`
	ErrorType        string    `gorm:"column:error_type"`
	ErrorMessage     string    `gorm:"column:error_message"`
	ResponsePreview  string    `gorm:"column:response_preview"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (ProviderTestResultRow) TableName() string { return "provider_test_results" }

type ProviderTestRepo struct{ db *gorm.DB }

func NewProviderTestRepo(db *gorm.DB) *ProviderTestRepo { return &ProviderTestRepo{db: db} }

func toTestRunRow(r *domain.ProviderTestRun) *ProviderTestRunRow {
	return &ProviderTestRunRow{ID: r.ID, ProviderID: r.ProviderID, Trigger: r.Trigger, Operator: r.Operator, Protocol: r.Protocol,
		Status: r.Status, Total: r.Total, Passed: r.Passed, Failed: r.Failed, Skipped: r.Skipped,
		AvgLatencyMS: r.AvgLatencyMS, P95LatencyMS: r.P95LatencyMS, Summary: r.Summary, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt}
}

func fromTestRunRow(r *ProviderTestRunRow) *domain.ProviderTestRun {
	return &domain.ProviderTestRun{ID: r.ID, ProviderID: r.ProviderID, Trigger: r.Trigger, Operator: r.Operator, Protocol: r.Protocol,
		Status: r.Status, Total: r.Total, Passed: r.Passed, Failed: r.Failed, Skipped: r.Skipped,
		AvgLatencyMS: r.AvgLatencyMS, P95LatencyMS: r.P95LatencyMS, Summary: r.Summary, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt}
}

func fromTestResultRow(r *ProviderTestResultRow) *domain.ProviderTestResult {
	return &domain.ProviderTestResult{ID: r.ID, RunID: r.RunID, ProviderID: r.ProviderID, ModelID: r.ModelID, ModelKey: r.ModelKey,
		CheckType: r.CheckType, Round: r.Round, Status: r.Status, LatencyMS: r.LatencyMS, HTTPStatus: r.HTTPStatus,
		PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens, ErrorType: r.ErrorType, ErrorMessage: r.ErrorMessage,
		ResponsePreview: r.ResponsePreview, CreatedAt: r.CreatedAt}
}

func (r *ProviderTestRepo) CreateRun(ctx context.Context, run *domain.ProviderTestRun) error {
	row := toTestRunRow(run)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create provider test run: %w", err)
	}
	run.ID = row.ID
	return nil
}

func (r *ProviderTestRepo) FinishRun(ctx context.Context, run *domain.ProviderTestRun, results []*domain.ProviderTestResult) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows := make([]ProviderTestResultRow, len(results))
		for i, x := range results {
			rows[i] = ProviderTestResultRow{RunID: run.ID, ProviderID: run.ProviderID, ModelID: x.ModelID, ModelKey: x.ModelKey,
				CheckType: x.CheckType, Round: x.Round, Status: x.Status, LatencyMS: x.LatencyMS, HTTPStatus: x.HTTPStatus,
				PromptTokens: x.PromptTokens, CompletionTokens: x.CompletionTokens, ErrorType: x.ErrorType,
				ErrorMessage: x.ErrorMessage, ResponsePreview: x.ResponsePreview, CreatedAt: x.CreatedAt}
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(rows, 200).Error; err != nil {
				return fmt.Errorf("store provider test results: %w", err)
			}
		}
		if err := tx.Model(&ProviderTestRunRow{}).Where("id = ?", run.ID).
			Select("status", "total", "passed", "failed", "skipped", "avg_latency_ms", "p95_latency_ms", "summary", "finished_at").
			Updates(toTestRunRow(run)).Error; err != nil {
			return fmt.Errorf("finish provider test run: %w", err)
		}
		return nil
	})
}

func (r *ProviderTestRepo) GetRun(ctx context.Context, id int64) (*domain.ProviderTestRun, error) {
	var row ProviderTestRunRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get provider test run: %w", err)
	}
	var rows []ProviderTestResultRow
	if err := r.db.WithContext(ctx).Where("run_id = ?", id).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list provider test results: %w", err)
	}
	run := fromTestRunRow(&row)
	run.Results = make([]*domain.ProviderTestResult, len(rows))
	for i := range rows {
		run.Results[i] = fromTestResultRow(&rows[i])
	}
	return run, nil
}

func (r *ProviderTestRepo) ListRuns(ctx context.Context, q domain.ProviderTestRunQuery) ([]*domain.ProviderTestRun, int64, error) {
	tx := r.db.WithContext(ctx).Model(&ProviderTestRunRow{})
	if q.ProviderID > 0 {
		tx = tx.Where("provider_id = ?", q.ProviderID)
	}
	if q.Trigger != "" {
		tx = tx.Where("trigger = ?", q.Trigger)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count provider test runs: %w", err)
	}
	var rows []ProviderTestRunRow
	if q.Limit > 0 {
		tx = tx.Offset(q.Offset).Limit(q.Limit)
	}
	if err := tx.Order("started_at desc, id desc").Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list provider test runs: %w", err)
	}
	return runsOf(rows), total, nil
}

func (r *ProviderTestRepo) RunsSince(ctx context.Context, providerID int64, since time.Time) ([]*domain.ProviderTestRun, error) {
	var rows []ProviderTestRunRow
	if err := r.db.WithContext(ctx).Where("provider_id = ? AND started_at >= ?", providerID, since).
		Order("started_at desc, id desc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list provider test runs: %w", err)
	}
	return runsOf(rows), nil
}

func (r *ProviderTestRepo) ResultsSince(ctx context.Context, providerID int64, since time.Time) ([]*domain.ProviderTestResult, error) {
	var rows []ProviderTestResultRow
	tx := r.db.WithContext(ctx).Where("created_at >= ?", since)
	if providerID > 0 {
		tx = tx.Where("provider_id = ?", providerID)
	}
	if err := tx.Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list provider test results: %w", err)
	}
	out := make([]*domain.ProviderTestResult, len(rows))
	for i := range rows {
		out[i] = fromTestResultRow(&rows[i])
	}
	return out, nil
}

func (r *ProviderTestRepo) LatestRuns(ctx context.Context) (map[int64]*domain.ProviderTestRun, error) {
	var rows []ProviderTestRunRow
	if err := r.db.WithContext(ctx).Raw(`SELECT DISTINCT ON (provider_id) * FROM provider_test_runs
		ORDER BY provider_id, started_at DESC, id DESC`).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("latest provider test runs: %w", err)
	}
	out := make(map[int64]*domain.ProviderTestRun, len(rows))
	for i := range rows {
		out[rows[i].ProviderID] = fromTestRunRow(&rows[i])
	}
	return out, nil
}

func (r *ProviderTestRepo) RunningFor(ctx context.Context, providerID int64, since time.Time) (*domain.ProviderTestRun, error) {
	var row ProviderTestRunRow
	err := r.db.WithContext(ctx).Where("provider_id = ? AND status = ? AND started_at >= ?", providerID, domain.TestRunRunning, since).
		Order("started_at desc").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find running provider test: %w", err)
	}
	return fromTestRunRow(&row), nil
}

func (r *ProviderTestRepo) AbandonStale(ctx context.Context, before time.Time) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&ProviderTestRunRow{}).Where("status = ? AND started_at < ?", domain.TestRunRunning, before).
		Updates(map[string]any{"status": domain.TestRunFailed, "summary": "测试未完成（节点重启或超时中断）", "finished_at": now}).Error; err != nil {
		return fmt.Errorf("abandon stale provider tests: %w", err)
	}
	return nil
}

func (r *ProviderTestRepo) PurgeBefore(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("started_at < ?", before).Delete(&ProviderTestRunRow{})
	if res.Error != nil {
		return 0, fmt.Errorf("purge provider test runs: %w", res.Error)
	}
	return res.RowsAffected, nil
}

func runsOf(rows []ProviderTestRunRow) []*domain.ProviderTestRun {
	out := make([]*domain.ProviderTestRun, len(rows))
	for i := range rows {
		out[i] = fromTestRunRow(&rows[i])
	}
	return out
}
