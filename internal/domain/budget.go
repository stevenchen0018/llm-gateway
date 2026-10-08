package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type BudgetPeriod string

const (
	BudgetPeriodMonthly   BudgetPeriod = "monthly"
	BudgetPeriodQuarterly BudgetPeriod = "quarterly"
	BudgetPeriodNone      BudgetPeriod = "none" // one-off / lifetime cap
)

type BudgetStatus string

const (
	BudgetStatusActive    BudgetStatus = "active"
	BudgetStatusExhausted BudgetStatus = "exhausted"
	BudgetStatusClosed    BudgetStatus = "closed"
	BudgetStatusPending   BudgetStatus = "pending"
)

// Budget implements the "预算申请→调用监控→预算预警" leg of the article's
// cost-governance closed loop. One Budget is attached to one APIKey.
type Budget struct {
	ID                int64           `json:"id"`
	KeyID             int64           `json:"key_id"`
	Period            BudgetPeriod    `json:"period"`
	Amount            decimal.Decimal `json:"amount"`
	Consumed          decimal.Decimal `json:"consumed"`
	Currency          string          `json:"currency"`
	AlertThresholdPct int             `json:"alert_threshold_pct"` // e.g. 80 -> alert at 80% consumed
	Status            BudgetStatus    `json:"status"`
	ApprovalStatus    string          `json:"approval_status"` // pending | approved | rejected
	ApproverLevel     string          `json:"approver_level"`  // D | CTO, decided by amount
	Approver          string          `json:"approver"`
	Applicant         string          `json:"applicant"`
	Project           string          `json:"project"`
	Reason            string          `json:"reason"`
	RejectReason      string          `json:"reject_reason"`
	ApprovedAt        *time.Time      `json:"approved_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// RemainingRatio returns consumed/amount in [0, +inf). A zero Amount is
// treated as "unlimited" and always returns 0.
func (b Budget) ConsumedRatio() float64 {
	if b.Amount.IsZero() {
		return 0
	}
	ratio, _ := b.Consumed.Div(b.Amount).Float64()
	return ratio
}

func (b Budget) IsExceeded() bool {
	if b.Amount.IsZero() {
		return false
	}
	return b.Consumed.GreaterThanOrEqual(b.Amount)
}

type BudgetRepository interface {
	Create(ctx context.Context, b *Budget) error
	Get(ctx context.Context, id int64) (*Budget, error)
	GetByKeyID(ctx context.Context, keyID int64) (*Budget, error)
	List(ctx context.Context) ([]*Budget, error)
	Page(ctx context.Context, q BudgetQuery) ([]*Budget, int64, error)
	// AddConsumed atomically increments Consumed by delta and returns the
	// updated row; used by the async cost-posting worker so concurrent
	// requests against the same budget never race.
	AddConsumed(ctx context.Context, id int64, delta decimal.Decimal) (*Budget, error)
	Update(ctx context.Context, b *Budget) error
}
