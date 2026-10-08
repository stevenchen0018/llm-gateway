package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// MonitorService backs the minute-level observability boards: RPM/TPM/RT and
// token/call trends (per Key or per model), failure-rate and RT rankings, and
// capacity management (Key / model quota vs observed peak).
type MonitorService struct {
	metrics domain.MetricsRepository
	keys    domain.APIKeyRepository
	models  domain.ModelRepository
	depts   domain.DepartmentRepository
}

func NewMonitorService(metrics domain.MetricsRepository, keys domain.APIKeyRepository, models domain.ModelRepository, depts domain.DepartmentRepository) *MonitorService {
	return &MonitorService{metrics: metrics, keys: keys, models: models, depts: depts}
}

var stepLadder = []int{60, 300, 600, 900, 1800, 3600, 7200, 21600, 86400}

const (
	maxBuckets = 720
	maxRange   = 31 * 24 * time.Hour
)

// AutoStep picks the finest step (in seconds) that keeps the chart under
// maxBuckets points.
func AutoStep(from, to time.Time) int {
	span := int(to.Sub(from).Seconds())
	for _, s := range stepLadder {
		if span/s <= maxBuckets {
			return s
		}
	}
	return stepLadder[len(stepLadder)-1]
}

type SeriesResult struct {
	StepSeconds int             `json:"step_seconds"`
	From        time.Time       `json:"from"`
	To          time.Time       `json:"to"`
	Series      []domain.Series `json:"series"`
}

func validateRange(f domain.MetricsFilter) error {
	if !f.To.After(f.From) {
		return fmt.Errorf("%w: 'to' must be after 'from'", domain.ErrInvalidArgument)
	}
	if f.To.Sub(f.From) > maxRange {
		return fmt.Errorf("%w: time range must not exceed 31 days", domain.ErrInvalidArgument)
	}
	return nil
}

// TimeSeries returns per-group series aligned to step-second buckets. Each
// point carries raw counters; RPM/TPM are derived by dividing by step/60.
func (s *MonitorService) TimeSeries(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy, step int) (*SeriesResult, error) {
	if err := validateRange(f); err != nil {
		return nil, err
	}
	if step <= 0 {
		step = AutoStep(f.From, f.To)
	}
	if int(f.To.Sub(f.From).Seconds())/step > maxBuckets*2 {
		return nil, fmt.Errorf("%w: step too small for the requested range", domain.ErrInvalidArgument)
	}
	series, err := s.metrics.TimeSeries(ctx, f, by, step)
	if err != nil {
		return nil, err
	}
	if series == nil {
		series = []domain.Series{}
	}
	return &SeriesResult{StepSeconds: step, From: f.From, To: f.To, Series: series}, nil
}

// RankRow is one bar of the failure-rate / average-RT rankings.
type RankRow struct {
	GroupKey         string  `json:"group_key"`
	Requests         int64   `json:"requests"`
	Failed           int64   `json:"failed"`
	FailureRate      float64 `json:"failure_rate"` // percent
	AvgLatencyMS     float64 `json:"avg_latency_ms"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost"`
}

func (s *MonitorService) Ranking(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy) ([]RankRow, error) {
	if err := validateRange(f); err != nil {
		return nil, err
	}
	rows, err := s.metrics.Aggregate(ctx, f, by)
	if err != nil {
		return nil, err
	}
	out := make([]RankRow, 0, len(rows))
	for _, r := range rows {
		rr := RankRow{GroupKey: r.GroupKey, Requests: r.Requests, Failed: r.Failed, PromptTokens: r.PromptTokens,
			CompletionTokens: r.CompletionTokens, TotalTokens: r.TotalTokens, Cost: r.Cost.InexactFloat64()}
		if r.Requests > 0 {
			rr.FailureRate = float64(r.Failed) / float64(r.Requests) * 100
			rr.AvgLatencyMS = float64(r.LatencySumMS) / float64(r.Requests)
		}
		out = append(out, rr)
	}
	return out, nil
}

// CapacityRow compares a configured ceiling with the observed peak.
type CapacityRow struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Sub      string  `json:"sub"`
	Status   string  `json:"status"`
	TPMLimit int     `json:"tpm_limit"`
	QPSLimit int     `json:"qps_limit"`
	PeakRPM  int64   `json:"peak_rpm"`
	PeakTPM  int64   `json:"peak_tpm"`
	LastRPM  int64   `json:"last_rpm"`
	LastTPM  int64   `json:"last_tpm"`
	TPMUtil  float64 `json:"tpm_util"` // peak TPM / limit, percent; 0 when unlimited
	QPSUtil  float64 `json:"qps_util"` // (peak RPM / 60) / limit, percent
}

type CapacityView struct {
	WindowMinutes int           `json:"window_minutes"`
	Keys          []CapacityRow `json:"keys"`
	Models        []CapacityRow `json:"models"`
	Departments   []CapacityRow `json:"departments"`
}

// Capacity reports Key- and model-level TPM/QPS headroom over the last hour
// ("Key和模型粒度的TPM容量管理体系").
// With deptID set, only that department's keys and department row are
// returned; model ceilings are platform-wide, so model rows are only
// included for the unscoped (super admin) view.
func (s *MonitorService) Capacity(ctx context.Context, window time.Duration, deptID *int64) (*CapacityView, error) {
	if window <= 0 {
		window = time.Hour
	}
	now := time.Now()
	f := domain.MetricsFilter{From: now.Add(-window), To: now.Add(time.Minute)}
	deptPeaks, err := s.metrics.Peaks(ctx, domain.MetricsFilter{From: f.From, To: f.To, DepartmentID: deptID}, domain.AggByDepartment)
	if err != nil {
		return nil, err
	}
	depts, err := s.depts.List(ctx)
	if err != nil {
		return nil, err
	}

	keyPeaks, err := s.metrics.Peaks(ctx, f, domain.AggByKey)
	if err != nil {
		return nil, err
	}
	modelPeaks, err := s.metrics.Peaks(ctx, f, domain.AggByModel)
	if err != nil {
		return nil, err
	}
	keys, err := s.keys.List(ctx)
	if err != nil {
		return nil, err
	}
	models, err := s.models.List(ctx)
	if err != nil {
		return nil, err
	}
	kp, mp := peakMap(keyPeaks), peakMap(modelPeaks)

	view := &CapacityView{WindowMinutes: int(window.Minutes()), Keys: []CapacityRow{}, Models: []CapacityRow{}, Departments: []CapacityRow{}}
	dp := peakMap(deptPeaks)
	for _, d := range depts {
		if deptID != nil && d.ID != *deptID {
			continue
		}
		row := CapacityRow{ID: d.ID, Name: d.Name, Sub: d.Leader, Status: d.Status, TPMLimit: d.TPMQuota, QPSLimit: d.QPSQuota}
		fillPeak(&row, dp[strconv.FormatInt(d.ID, 10)])
		view.Departments = append(view.Departments, row)
	}
	for _, k := range keys {
		if k.Status == domain.APIKeyStatusPending {
			continue
		}
		if deptID != nil && (k.DepartmentID == nil || *k.DepartmentID != *deptID) {
			continue
		}
		row := CapacityRow{ID: k.ID, Name: k.Name, Sub: k.Owner, Status: string(k.Status), TPMLimit: k.TPMQuota, QPSLimit: k.QPSQuota}
		fillPeak(&row, kp[strconv.FormatInt(k.ID, 10)])
		view.Keys = append(view.Keys, row)
	}
	for _, m := range models {
		if deptID != nil {
			break
		}
		row := CapacityRow{ID: m.ID, Name: m.DisplayName, Sub: m.Provider.Name, Status: string(m.Status), TPMLimit: m.TPMLimit, QPSLimit: m.QPSLimit}
		fillPeak(&row, mp[strconv.FormatInt(m.ID, 10)])
		view.Models = append(view.Models, row)
	}
	return view, nil
}

func peakMap(rows []domain.PeakRow) map[string]domain.PeakRow {
	m := make(map[string]domain.PeakRow, len(rows))
	for _, r := range rows {
		m[r.GroupKey] = r
	}
	return m
}

func fillPeak(row *CapacityRow, p domain.PeakRow) {
	row.PeakRPM, row.PeakTPM, row.LastRPM, row.LastTPM = p.PeakRPM, p.PeakTPM, p.LastRPM, p.LastTPM
	if row.TPMLimit > 0 {
		row.TPMUtil = float64(p.PeakTPM) / float64(row.TPMLimit) * 100
	}
	if row.QPSLimit > 0 {
		row.QPSUtil = float64(p.PeakRPM) / 60 / float64(row.QPSLimit) * 100
	}
}
