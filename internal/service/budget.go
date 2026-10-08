package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// Approver levels of the tiered budget approval flow (源头管控).
const (
	ApproverD   = "D"   // director: amounts up to the configured limit
	ApproverCTO = "CTO" // CTO: anything above it
)

// BudgetService implements the "预算申请→审批→预算消耗→预算预警" leg of the
// article's cost-governance closed loop. A budget is applied for, routed to
// D or the CTO by amount, and only enforced once approved.
type BudgetService struct {
	budgets       domain.BudgetRepository
	alerts        *AlertService
	directorLimit decimal.Decimal
}

func NewBudgetService(budgets domain.BudgetRepository, alerts *AlertService, directorLimit float64) *BudgetService {
	return &BudgetService{budgets: budgets, alerts: alerts, directorLimit: decimal.NewFromFloat(directorLimit)}
}

// ApproverLevelFor decides who must approve a budget of the given amount.
func (s *BudgetService) ApproverLevelFor(amount decimal.Decimal) string {
	if amount.GreaterThan(s.directorLimit) {
		return ApproverCTO
	}
	return ApproverD
}

// Apply files a budget request in pending state.
func (s *BudgetService) Apply(ctx context.Context, b *domain.Budget) error {
	if b.KeyID == 0 {
		return fmt.Errorf("%w: key_id is required", domain.ErrInvalidArgument)
	}
	if !b.Amount.IsPositive() {
		return fmt.Errorf("%w: amount must be positive", domain.ErrInvalidArgument)
	}
	if open, err := s.budgets.GetByKeyID(ctx, b.KeyID); err == nil {
		return fmt.Errorf("%w: 该 Key 已有%s的预算 #%d，请勿重复申请", domain.ErrInvalidArgument,
			map[string]string{"pending": "待审批", "approved": "生效中"}[open.ApprovalStatus], open.ID)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if b.Currency == "" {
		b.Currency = "CNY"
	}
	if b.Period == "" {
		b.Period = domain.BudgetPeriodMonthly
	}
	if b.AlertThresholdPct == 0 {
		b.AlertThresholdPct = 80
	}
	b.Status = domain.BudgetStatusPending
	b.ApprovalStatus = "pending"
	b.ApproverLevel = s.ApproverLevelFor(b.Amount)
	return s.budgets.Create(ctx, b)
}

func (s *BudgetService) Approve(ctx context.Context, id int64, approver string) (*domain.Budget, error) {
	b, err := s.budgets.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.ApprovalStatus != "pending" {
		return nil, fmt.Errorf("%w: budget is already %s", domain.ErrInvalidArgument, b.ApprovalStatus)
	}
	now := time.Now()
	b.ApprovalStatus, b.Status, b.Approver, b.ApprovedAt = "approved", domain.BudgetStatusActive, approver, &now
	return b, s.budgets.Update(ctx, b)
}

func (s *BudgetService) Reject(ctx context.Context, id int64, approver, reason string) (*domain.Budget, error) {
	b, err := s.budgets.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.ApprovalStatus != "pending" {
		return nil, fmt.Errorf("%w: budget is already %s", domain.ErrInvalidArgument, b.ApprovalStatus)
	}
	b.ApprovalStatus, b.Status, b.Approver, b.RejectReason = "rejected", domain.BudgetStatusClosed, approver, reason
	return b, s.budgets.Update(ctx, b)
}

func (s *BudgetService) Get(ctx context.Context, id int64) (*domain.Budget, error) {
	return s.budgets.Get(ctx, id)
}

func (s *BudgetService) List(ctx context.Context) ([]*domain.Budget, error) {
	return s.budgets.List(ctx)
}

func (s *BudgetService) Page(ctx context.Context, q domain.BudgetQuery) ([]*domain.Budget, int64, error) {
	return s.budgets.Page(ctx, q)
}

func (s *BudgetService) GetByKeyID(ctx context.Context, keyID int64) (*domain.Budget, error) {
	return s.budgets.GetByKeyID(ctx, keyID)
}

// PostCost atomically adds cost to budgetID's consumption and raises a
// warning/critical alert if the new total crosses the configured threshold
// or the budget's full amount. Called from the async usage-posting worker,
// never inline on the request path.
func (s *BudgetService) PostCost(ctx context.Context, budgetID int64, cost decimal.Decimal) error {
	updated, err := s.budgets.AddConsumed(ctx, budgetID, cost)
	if err != nil {
		return err
	}

	ratio := updated.ConsumedRatio()
	switch {
	case updated.IsExceeded():
		if updated.Status != domain.BudgetStatusExhausted {
			updated.Status = domain.BudgetStatusExhausted
			_ = s.budgets.Update(ctx, updated)
		}
		s.alerts.Raise(ctx, domain.AlertTypeBudget, budgetID, &updated.KeyID,
			fmt.Sprintf("budget %d exhausted: consumed=%s amount=%s", budgetID, updated.Consumed, updated.Amount),
			domain.AlertLevelCritical)
	case ratio*100 >= float64(updated.AlertThresholdPct):
		s.alerts.Raise(ctx, domain.AlertTypeBudget, budgetID, &updated.KeyID,
			fmt.Sprintf("budget %d at %.1f%% of quota (threshold %d%%)", budgetID, ratio*100, updated.AlertThresholdPct),
			domain.AlertLevelWarning)
	}
	return nil
}
