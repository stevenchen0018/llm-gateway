package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// SecurityHandler serves 安全与合规: runtime gateway settings, prompt/content
// filter rules and the captured request records.
type SecurityHandler struct {
	settings *service.SettingsService
	filter   *service.ContentFilterService
	logs     *service.RequestLogService
	keys     *service.APIKeyService
}

func NewSecurityHandler(settings *service.SettingsService, filter *service.ContentFilterService, logs *service.RequestLogService, keys *service.APIKeyService) *SecurityHandler {
	return &SecurityHandler{settings: settings, filter: filter, logs: logs, keys: keys}
}

// ---- settings --------------------------------------------------------------------------

func (h *SecurityHandler) GetSettings(c *gin.Context) {
	response.AdminOK(c, http.StatusOK, h.settings.Get(c.Request.Context()))
}

func (h *SecurityHandler) UpdateSettings(c *gin.Context) {
	var req domain.GatewaySettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	st, err := h.settings.Update(c.Request.Context(), req, middleware.AdminUserFromContext(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, st)
}

// ---- content filter rules -----------------------------------------------------------------

// visibleRule: platform rules are visible to everyone (they apply to all
// departments); department rules only to that department.
func visibleRule(c *gin.Context, r *domain.ContentFilterRule) bool {
	return r.DepartmentID == nil || middleware.PrincipalFrom(c).Owns(r.DepartmentID)
}

// editableRule: platform rules are super-admin only.
func editableRule(c *gin.Context, dept *int64) bool {
	p := middleware.PrincipalFrom(c)
	if dept == nil {
		return p.IsSuper()
	}
	return p.Owns(dept)
}

func (h *SecurityHandler) ListRules(c *gin.Context) {
	rules, err := h.filter.List(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	scope := deptScope(c)
	out := make([]*domain.ContentFilterRule, 0, len(rules))
	for _, r := range rules {
		if visibleRule(c, r) && (r.DepartmentID == nil || inScope(scope, r.DepartmentID)) &&
			eqParam(c, "action", string(r.Action)) && matchQ(c, r.Name, r.Pattern, r.Description) {
			out = append(out, r)
		}
	}
	respondList(c, out)
}

func ruleFromRequest(req dto.FilterRuleRequest) *domain.ContentFilterRule {
	r := &domain.ContentFilterRule{
		Name: req.Name, Description: req.Description, MatchType: req.MatchType, Pattern: req.Pattern,
		Action: domain.FilterAction(req.Action), Replacement: req.Replacement, Stage: domain.FilterStage(req.Stage),
		DepartmentID: req.DepartmentID, Priority: 100, Enabled: true,
	}
	if req.Priority != nil {
		r.Priority = *req.Priority
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	return r
}

func (h *SecurityHandler) CreateRule(c *gin.Context) {
	var req dto.FilterRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	p := middleware.PrincipalFrom(c)
	if !p.IsSuper() {
		req.DepartmentID = p.DepartmentID // department admins write rules for their own department
	}
	r := ruleFromRequest(req)
	if !editableRule(c, r.DepartmentID) {
		writeAdminError(c, domain.ErrForbidden)
		return
	}
	r.CreatedBy = p.Username
	if err := h.filter.Create(c.Request.Context(), r); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, r)
}

func (h *SecurityHandler) ownedRule(c *gin.Context) *domain.ContentFilterRule {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return nil
	}
	r, err := h.filter.Get(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return nil
	}
	if !editableRule(c, r.DepartmentID) {
		writeAdminError(c, domain.ErrForbidden)
		return nil
	}
	return r
}

func (h *SecurityHandler) UpdateRule(c *gin.Context) {
	existing := h.ownedRule(c)
	if existing == nil {
		return
	}
	var req dto.FilterRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !middleware.PrincipalFrom(c).IsSuper() {
		req.DepartmentID = existing.DepartmentID
	}
	r := ruleFromRequest(req)
	if !editableRule(c, r.DepartmentID) {
		writeAdminError(c, domain.ErrForbidden)
		return
	}
	r.ID, r.CreatedBy, r.HitCount, r.LastHitAt, r.CreatedAt = existing.ID, existing.CreatedBy, existing.HitCount, existing.LastHitAt, existing.CreatedAt
	if req.Priority == nil {
		r.Priority = existing.Priority
	}
	if req.Enabled == nil {
		r.Enabled = existing.Enabled
	}
	if err := h.filter.Update(c.Request.Context(), r); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, r)
}

func (h *SecurityHandler) DeleteRule(c *gin.Context) {
	r := h.ownedRule(c)
	if r == nil {
		return
	}
	if err := h.filter.Delete(c.Request.Context(), r.ID); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"deleted": r.ID})
}

// TestRules previews what the filter would do to a text (saved rules, or a
// draft rule being edited), regardless of the master switch.
func (h *SecurityHandler) TestRules(c *gin.Context) {
	var req dto.FilterTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	p := middleware.PrincipalFrom(c)
	dept := req.DepartmentID
	if !p.IsSuper() {
		dept = p.DepartmentID
	}
	var draft *domain.ContentFilterRule
	if req.Rule != nil {
		draft = ruleFromRequest(*req.Rule)
		draft.DepartmentID = nil // a draft is tested on its own
	}
	res, out, err := h.filter.Test(c.Request.Context(), req.Text, dept, draft)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"hits": res.Hits, "blocked": res.Blocked, "block_by": res.BlockBy, "output": out})
}

// ---- request records ----------------------------------------------------------------------------

func (h *SecurityHandler) ListRequestLogs(c *gin.Context) {
	f := metricsFilter(c, 24*time.Hour)
	p := alwaysPaged(c)
	q := domain.RequestLogQuery{
		From: f.From, To: f.To, KeyID: f.KeyID, ModelID: f.ModelID, Status: c.Query("status"),
		RequestID: c.Query("request_id"), Keyword: c.Query("q"), FilterHit: c.Query("filter_hit") == "true",
		DeptID: deptScope(c), Page: p.Page, PageSize: p.Size,
	}
	items, total, err := h.logs.List(c.Request.Context(), q)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondPage(c, items, total, p)
}

// requestLogVisible writes 403 and returns false unless the caller's
// department owns the key the record belongs to.
func (h *SecurityHandler) requestLogVisible(c *gin.Context, l *domain.RequestLog) bool {
	if middleware.PrincipalFrom(c).IsSuper() {
		return true
	}
	k, err := h.keys.Get(c.Request.Context(), l.KeyID)
	if err != nil {
		writeAdminError(c, domain.ErrForbidden)
		return false
	}
	return ensureOwns(c, k.DepartmentID)
}

func (h *SecurityHandler) GetRequestLog(c *gin.Context) {
	var (
		l   *domain.RequestLog
		err error
	)
	if rid := c.Query("request_id"); rid != "" && c.Param("id") == "by-request" {
		l, err = h.logs.GetByRequestID(c.Request.Context(), rid)
	} else {
		id, perr := parseID(c)
		if perr != nil {
			response.AdminError(c, http.StatusBadRequest, "invalid id")
			return
		}
		l, err = h.logs.Get(c.Request.Context(), id)
	}
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if !h.requestLogVisible(c, l) {
		return
	}
	response.AdminOK(c, http.StatusOK, l)
}
