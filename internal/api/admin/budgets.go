package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// BudgetHandler implements 源头管控: budget application → tiered approval
// (D / CTO by amount) → consumption tracking. The "预算预警" leg fires
// automatically from service.BudgetService.PostCost as usage is posted.
type BudgetHandler struct {
	budgets *service.BudgetService
	keys    *service.APIKeyService
}

func NewBudgetHandler(budgets *service.BudgetService, keys *service.APIKeyService) *BudgetHandler {
	return &BudgetHandler{budgets: budgets, keys: keys}
}

func (h *BudgetHandler) keyDepts(c *gin.Context) (map[int64]*int64, error) {
	keys, err := h.keys.List(c.Request.Context())
	if err != nil {
		return nil, err
	}
	m := make(map[int64]*int64, len(keys))
	for _, k := range keys {
		m[k.ID] = k.DepartmentID
	}
	return m, nil
}

func (h *BudgetHandler) List(c *gin.Context) {
	if p, ok := pageParams(c); ok {
		items, total, err := h.budgets.Page(c.Request.Context(), domain.BudgetQuery{
			PageQuery: p.query(), DeptID: deptScope(c), ApprovalStatus: c.Query("approval_status"), Q: c.Query("q"),
		})
		if err != nil {
			writeAdminError(c, err)
			return
		}
		respondPage(c, items, total, p)
		return
	}
	budgets, err := h.budgets.List(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	depts, err := h.keyDepts(c)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	scope := deptScope(c)
	out := make([]*domain.Budget, 0, len(budgets))
	for _, b := range budgets {
		if inScope(scope, depts[b.KeyID]) {
			out = append(out, b)
		}
	}
	respondList(c, out)
}

// ownedBudget loads a budget and enforces that its key belongs to the caller's department.
func (h *BudgetHandler) ownedBudget(c *gin.Context) *domain.Budget {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return nil
	}
	b, err := h.budgets.Get(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return nil
	}
	k, err := h.keys.Get(c.Request.Context(), b.KeyID)
	if err != nil {
		writeAdminError(c, err)
		return nil
	}
	if !ensureOwns(c, k.DepartmentID) {
		return nil
	}
	return b
}

// canDecide enforces the tiered approval: D-level budgets can be decided by the
// department's admin (or a super admin); CTO-level budgets only by a super admin.
func canDecide(c *gin.Context, b *domain.Budget) bool {
	if b.ApproverLevel == service.ApproverCTO && !middleware.PrincipalFrom(c).IsSuper() {
		response.AdminError(c, http.StatusForbidden, "该预算超过总监审批额度，需 CTO（超级管理员）审批")
		return false
	}
	return true
}

// Create files a budget application; it becomes effective (and is attached to
// its key) only once the approver signs it off.
func (h *BudgetHandler) Create(c *gin.Context) {
	var req dto.CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid amount")
		return
	}
	key, err := h.keys.Get(c.Request.Context(), req.KeyID)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if !ensureOwns(c, key.DepartmentID) {
		return
	}

	b := &domain.Budget{
		KeyID: req.KeyID, Period: domain.BudgetPeriod(req.Period), Amount: amount,
		Currency: req.Currency, AlertThresholdPct: req.AlertThresholdPct,
		Applicant: req.Applicant, Project: req.Project, Reason: req.Reason,
	}
	if b.Applicant == "" {
		b.Applicant = middleware.AdminUserFromContext(c)
	}
	if err := h.budgets.Apply(c.Request.Context(), b); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, b)
}

func (h *BudgetHandler) Approve(c *gin.Context) {
	owned := h.ownedBudget(c)
	if owned == nil || !canDecide(c, owned) {
		return
	}
	id := owned.ID
	b, err := h.budgets.Approve(c.Request.Context(), id, middleware.AdminUserFromContext(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if err := h.keys.AttachBudget(c.Request.Context(), b.KeyID, b.ID); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, b)
}

func (h *BudgetHandler) Reject(c *gin.Context) {
	owned := h.ownedBudget(c)
	if owned == nil || !canDecide(c, owned) {
		return
	}
	id := owned.ID
	var req dto.RejectBudgetRequest
	_ = c.ShouldBindJSON(&req)
	b, err := h.budgets.Reject(c.Request.Context(), id, middleware.AdminUserFromContext(c), req.Reason)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, b)
}

func (h *BudgetHandler) Get(c *gin.Context) {
	b := h.ownedBudget(c)
	if b == nil {
		return
	}
	response.AdminOK(c, http.StatusOK, b)
}

// Consumption returns the budget's current consumed/amount/ratio — the
// "费用查看" step closing the article's cost-governance loop.
func (h *BudgetHandler) Consumption(c *gin.Context) {
	b := h.ownedBudget(c)
	if b == nil {
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{
		"budget_id":      b.ID,
		"amount":         b.Amount,
		"consumed":       b.Consumed,
		"consumed_ratio": b.ConsumedRatio(),
		"currency":       b.Currency,
		"status":         b.Status,
	})
}

// BudgetSummary backs the KPI cards above the paged budget table, which can
// no longer be computed from the rows on screen.
type BudgetSummary struct {
	Total      int     `json:"total"`
	Pending    int     `json:"pending"`
	PendingD   int     `json:"pending_d"`
	PendingCTO int     `json:"pending_cto"`
	Approved   int     `json:"approved"`
	Rejected   int     `json:"rejected"`
	Amount     float64 `json:"amount"`   // approved budgets
	Consumed   float64 `json:"consumed"` // approved budgets
	Attention  int     `json:"attention"`
	OpenKeyIDs []int64 `json:"open_key_ids"` // keys with a pending/approved budget
}

func (h *BudgetHandler) Summary(c *gin.Context) {
	budgets, err := h.budgets.List(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	depts, err := h.keyDepts(c)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	scope := deptScope(c)
	s := BudgetSummary{OpenKeyIDs: []int64{}}
	for _, b := range budgets {
		if !inScope(scope, depts[b.KeyID]) {
			continue
		}
		s.Total++
		switch b.ApprovalStatus {
		case "pending":
			s.Pending++
			if b.ApproverLevel == "CTO" {
				s.PendingCTO++
			} else {
				s.PendingD++
			}
		case "approved":
			s.Approved++
			amount, consumed := b.Amount.InexactFloat64(), b.Consumed.InexactFloat64()
			s.Amount += amount
			s.Consumed += consumed
			if b.Status == domain.BudgetStatusExhausted || (amount > 0 && consumed/amount*100 >= float64(b.AlertThresholdPct)) {
				s.Attention++
			}
		case "rejected":
			s.Rejected++
		}
		if b.ApprovalStatus != "rejected" {
			s.OpenKeyIDs = append(s.OpenKeyIDs, b.KeyID)
		}
	}
	response.AdminOK(c, http.StatusOK, s)
}
