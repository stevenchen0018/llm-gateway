package service

import (
	"context"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// DashboardService backs the article's usage/cost boards: "Key维度成本大盘、
// 模型维度成本大盘、厂商维度成本大盘...日/周/月/自定义时间维度汇总".
type DashboardService struct {
	usage domain.UsageRepository
}

func NewDashboardService(usage domain.UsageRepository) *DashboardService {
	return &DashboardService{usage: usage}
}

type DashboardRange struct {
	From time.Time
	To   time.Time
}

func (d DashboardRange) orDefault() DashboardRange {
	if d.To.IsZero() {
		d.To = time.Now()
	}
	if d.From.IsZero() {
		d.From = d.To.AddDate(0, 0, -30)
	}
	return d
}

func (s *DashboardService) Usage(ctx context.Context, r DashboardRange, groupBy domain.UsageGroupBy, keyID, modelID, deptID *int64) ([]domain.UsageSummary, error) {
	r = r.orDefault()
	return s.usage.Summarize(ctx, domain.UsageQuery{From: r.From, To: r.To, GroupBy: groupBy, KeyID: keyID, ModelID: modelID, DeptID: deptID})
}

// Logs pages the raw per-call log.
func (s *DashboardService) Logs(ctx context.Context, q domain.UsageLogQuery) ([]domain.UsageRecord, int64, error) {
	items, total, err := s.usage.List(ctx, q)
	if items == nil {
		items = []domain.UsageRecord{}
	}
	return items, total, err
}
