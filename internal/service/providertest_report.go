package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ModelTestStat aggregates one model's checks in the report window.
type ModelTestStat struct {
	ModelID      int64   `json:"model_id"`
	ModelKey     string  `json:"model_key"`
	CheckType    string  `json:"check_type"`
	Checks       int     `json:"checks"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Availability float64 `json:"availability"` // passed / (passed+failed); -1 = never executed
	AvgLatencyMS int     `json:"avg_latency_ms"`
	P95LatencyMS int     `json:"p95_latency_ms"`
	LastStatus   string  `json:"last_status"`
	LastError    string  `json:"last_error"`
	LastAt       string  `json:"last_at"`
}

type ErrorTypeStat struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type DailyTestStat struct {
	Date         string  `json:"date"`
	Checks       int     `json:"checks"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Availability float64 `json:"availability"` // -1 = no executed check that day
	AvgLatencyMS int     `json:"avg_latency_ms"`
}

// ProviderTestReport is the simple test report (测试报告) of one supplier.
type ProviderTestReport struct {
	Provider     *domain.Provider          `json:"provider"`
	Days         int                       `json:"days"`
	Runs         int                       `json:"runs"`
	Checks       int                       `json:"checks"` // executed (not skipped)
	Passed       int                       `json:"passed"`
	Failed       int                       `json:"failed"`
	Availability float64                   `json:"availability"` // -1 = no data
	ProbePassed  int                       `json:"probe_passed"`
	ProbeFailed  int                       `json:"probe_failed"`
	AvgLatencyMS int                       `json:"avg_latency_ms"`
	P50LatencyMS int                       `json:"p50_latency_ms"`
	P95LatencyMS int                       `json:"p95_latency_ms"`
	LastRun      *domain.ProviderTestRun   `json:"last_run,omitempty"` // with results
	Models       []ModelTestStat           `json:"models"`
	Errors       []ErrorTypeStat           `json:"errors"`
	Daily        []DailyTestStat           `json:"daily"`
	RecentRuns   []*domain.ProviderTestRun `json:"recent_runs"`
	GeneratedAt  time.Time                 `json:"generated_at"`
}

// ProviderTestOverview is one supplier's row on the availability overview.
type ProviderTestOverview struct {
	Provider     *domain.Provider        `json:"provider"`
	LastRun      *domain.ProviderTestRun `json:"last_run,omitempty"`
	Checks       int                     `json:"checks"`
	Passed       int                     `json:"passed"`
	Availability float64                 `json:"availability"`
	AvgLatencyMS int                     `json:"avg_latency_ms"`
	P95LatencyMS int                     `json:"p95_latency_ms"`
}

func normDays(days int) int { return min(max(days, 1), 90) }

func availability(passed, failed int) float64 {
	if passed+failed == 0 {
		return -1
	}
	return float64(passed) / float64(passed+failed)
}

// Report summarizes a supplier's test history over the last days.
func (s *ProviderTestService) Report(ctx context.Context, providerID int64, days int) (*ProviderTestReport, error) {
	p, err := s.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	days = normDays(days)
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	runs, err := s.repo.RunsSince(ctx, providerID, since)
	if err != nil {
		return nil, err
	}
	results, err := s.repo.ResultsSince(ctx, providerID, since)
	if err != nil {
		return nil, err
	}

	rep := &ProviderTestReport{Provider: p, Days: days, Runs: len(runs), Models: []ModelTestStat{}, Errors: []ErrorTypeStat{},
		RecentRuns: runs[:min(len(runs), 30)], GeneratedAt: now}
	if len(runs) > 0 {
		if last, err := s.repo.GetRun(ctx, runs[0].ID); err == nil {
			rep.LastRun = last
		}
	}

	type acc struct {
		stat *ModelTestStat
		lat  []int
	}
	byModel := map[string]*acc{}
	var order []string
	daily := map[string]*DailyTestStat{}
	dailyLat := map[string][]int{}
	errs := map[string]int{}
	var lat []int
	for _, r := range results {
		if r.Status == domain.TestSkipped {
			continue
		}
		ok := r.Status == domain.TestPassed
		if r.CheckType == domain.TestCheckProbe {
			if ok {
				rep.ProbePassed++
			} else {
				rep.ProbeFailed++
			}
		}
		rep.Checks++
		day := r.CreatedAt.In(now.Location()).Format("2006-01-02")
		d := daily[day]
		if d == nil {
			d = &DailyTestStat{Date: day}
			daily[day] = d
		}
		d.Checks++
		if ok {
			rep.Passed++
			d.Passed++
			if r.CheckType != domain.TestCheckProbe {
				lat = append(lat, r.LatencyMS)
				dailyLat[day] = append(dailyLat[day], r.LatencyMS)
			}
		} else {
			rep.Failed++
			d.Failed++
			errs[r.ErrorType]++
		}
		if r.CheckType == domain.TestCheckProbe {
			continue
		}
		k := r.ModelKey
		a := byModel[k]
		if a == nil {
			a = &acc{stat: &ModelTestStat{ModelKey: r.ModelKey, CheckType: r.CheckType}}
			if r.ModelID != nil {
				a.stat.ModelID = *r.ModelID
			}
			byModel[k] = a
			order = append(order, k)
		}
		a.stat.Checks++
		if ok {
			a.stat.Passed++
			a.lat = append(a.lat, r.LatencyMS)
		} else {
			a.stat.Failed++
			a.stat.LastError = fmt.Sprintf("%s：%s", testErrLabel(r.ErrorType), r.ErrorMessage)
		}
		a.stat.LastStatus, a.stat.LastAt = r.Status, r.CreatedAt.Format(time.RFC3339)
	}
	rep.Availability = availability(rep.Passed, rep.Failed)
	rep.AvgLatencyMS, rep.P50LatencyMS, rep.P95LatencyMS = latencyStats(lat)

	for _, k := range order {
		a := byModel[k]
		a.stat.Availability = availability(a.stat.Passed, a.stat.Failed)
		a.stat.AvgLatencyMS, _, a.stat.P95LatencyMS = latencyStats(a.lat)
		rep.Models = append(rep.Models, *a.stat)
	}
	// weakest models first: they are what the reader acts on
	sort.SliceStable(rep.Models, func(i, j int) bool { return rep.Models[i].Availability < rep.Models[j].Availability })

	for t, n := range errs {
		rep.Errors = append(rep.Errors, ErrorTypeStat{Type: t, Label: testErrLabel(t), Count: n})
	}
	sort.Slice(rep.Errors, func(i, j int) bool { return rep.Errors[i].Count > rep.Errors[j].Count })

	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		d := daily[day]
		if d == nil {
			d = &DailyTestStat{Date: day}
		}
		d.Availability = availability(d.Passed, d.Failed)
		d.AvgLatencyMS, _, _ = latencyStats(dailyLat[day])
		rep.Daily = append(rep.Daily, *d)
	}
	return rep, nil
}

// Overview lists every supplier with its latest run and availability over
// the last days.
func (s *ProviderTestService) Overview(ctx context.Context, days int) ([]ProviderTestOverview, error) {
	ps, err := s.providers.List(ctx)
	if err != nil {
		return nil, err
	}
	latest, err := s.repo.LatestRuns(ctx)
	if err != nil {
		return nil, err
	}
	results, err := s.repo.ResultsSince(ctx, 0, time.Now().AddDate(0, 0, -normDays(days)))
	if err != nil {
		return nil, err
	}
	type acc struct {
		passed, failed int
		lat            []int
	}
	by := map[int64]*acc{}
	for _, r := range results {
		if r.Status == domain.TestSkipped {
			continue
		}
		a := by[r.ProviderID]
		if a == nil {
			a = &acc{}
			by[r.ProviderID] = a
		}
		if r.Status == domain.TestPassed {
			a.passed++
			if r.CheckType != domain.TestCheckProbe {
				a.lat = append(a.lat, r.LatencyMS)
			}
		} else {
			a.failed++
		}
	}
	out := make([]ProviderTestOverview, 0, len(ps))
	for _, p := range ps {
		row := ProviderTestOverview{Provider: p, LastRun: latest[p.ID], Availability: -1}
		if a := by[p.ID]; a != nil {
			row.Checks, row.Passed = a.passed+a.failed, a.passed
			row.Availability = availability(a.passed, a.failed)
			row.AvgLatencyMS, _, row.P95LatencyMS = latencyStats(a.lat)
		}
		out = append(out, row)
	}
	return out, nil
}
