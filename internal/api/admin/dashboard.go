package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// DashboardHandler backs the overview board (Key/model/provider dimensions,
// daily buckets) and the alert feed.
type DashboardHandler struct {
	dashboard *service.DashboardService
	alerts    *service.AlertService
}

func NewDashboardHandler(dashboard *service.DashboardService, alerts *service.AlertService) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard, alerts: alerts}
}

func (h *DashboardHandler) Usage(c *gin.Context) {
	groupBy := domain.UsageGroupBy(c.DefaultQuery("group_by", "model"))

	r := service.DashboardRange{}
	if v := c.Query("from"); v != "" {
		if t, ok := parseTimeParam(v); ok {
			r.From = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, ok := parseTimeParam(v); ok {
			r.To = t
		}
	}

	var keyID, modelID *int64
	if v := c.Query("key_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			keyID = &n
		}
	}
	if v := c.Query("model_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			modelID = &n
		}
	}

	summary, err := h.dashboard.Usage(c.Request.Context(), r, groupBy, keyID, modelID, deptScope(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, summary)
}

func (h *DashboardHandler) Alerts(c *gin.Context) {
	if p, ok := pageParams(c); ok {
		items, total, err := h.alerts.Page(c.Request.Context(), domain.AlertQuery{
			PageQuery: p.query(), DeptID: deptScope(c), Type: c.Query("type"), Level: c.Query("level"), Q: c.Query("q"),
			Notified: queryBoolPtr(c, "notified"),
		})
		if err != nil {
			writeAdminError(c, err)
			return
		}
		respondPage(c, items, total, p)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	events, err := h.alerts.List(c.Request.Context(), limit, deptScope(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, events)
}

// Logs pages the raw per-call log (调用日志: request id, key, model, tokens,
// cost, latency, status, source IP).
func (h *DashboardHandler) Logs(c *gin.Context) {
	p := alwaysPaged(c)
	now := time.Now()
	to := queryTime(c, "to", now.Add(time.Minute))
	from := queryTime(c, "from", to.Add(-24*time.Hour))
	items, total, err := h.dashboard.Logs(c.Request.Context(), domain.UsageLogQuery{
		From: from, To: to, KeyID: queryInt64Ptr(c, "key_id"), ModelID: queryInt64Ptr(c, "model_id"),
		Status: c.Query("status"), DeptID: deptScope(c), Page: p.Page, PageSize: p.Size,
	})
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondPage(c, items, total, p)
}
