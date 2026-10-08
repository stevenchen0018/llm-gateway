package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// MonitorHandler serves the minute-level monitoring boards (调用/用量/性能
// 监控 by Key and by model), capacity management, and the cost boards.
type MonitorHandler struct {
	monitor *service.MonitorService
	cost    *service.CostService
}

func NewMonitorHandler(monitor *service.MonitorService, cost *service.CostService) *MonitorHandler {
	return &MonitorHandler{monitor: monitor, cost: cost}
}

func groupBy(c *gin.Context, def domain.AggGroupBy) domain.AggGroupBy {
	if v := c.Query("group_by"); v != "" {
		return domain.AggGroupBy(v)
	}
	return def
}

// TimeSeries: GET /monitor/timeseries?group_by=model|key|provider|app|category
// &from&to&step&key_id&model_id&provider_id&app_id&category
func (h *MonitorHandler) TimeSeries(c *gin.Context) {
	res, err := h.monitor.TimeSeries(c.Request.Context(), metricsFilter(c, 3*time.Hour), groupBy(c, domain.AggByModel), queryInt(c, "step", 0))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, res)
}

// Ranking: per-group failure rate / average RT / volume over the window.
func (h *MonitorHandler) Ranking(c *gin.Context) {
	rows, err := h.monitor.Ranking(c.Request.Context(), metricsFilter(c, 3*time.Hour), groupBy(c, domain.AggByModel))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, rows)
}

func (h *MonitorHandler) Capacity(c *gin.Context) {
	view, err := h.monitor.Capacity(c.Request.Context(), time.Duration(queryInt(c, "window_minutes", 60))*time.Minute, deptScope(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, view)
}

func (h *MonitorHandler) CostOverview(c *gin.Context) {
	rows, err := h.cost.Overview(c.Request.Context(), metricsFilter(c, 7*24*time.Hour), groupBy(c, domain.AggByModel))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, rows)
}

func (h *MonitorHandler) CostBenchmark(c *gin.Context) {
	boards, err := h.cost.Benchmark(c.Request.Context(), metricsFilter(c, 7*24*time.Hour))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, boards)
}

func (h *MonitorHandler) CostSuggestions(c *gin.Context) {
	s, err := h.cost.Suggestions(c.Request.Context(), metricsFilter(c, 7*24*time.Hour))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if s == nil {
		s = []service.Suggestion{}
	}
	response.AdminOK(c, http.StatusOK, s)
}

func (h *MonitorHandler) Digest(c *gin.Context) {
	items, err := h.cost.Digest(c.Request.Context(), queryInt(c, "days", 7), deptScope(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, items)
}

func (h *MonitorHandler) SendDigest(c *gin.Context) {
	n, err := h.cost.SendDigest(c.Request.Context(), queryInt(c, "days", 7))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"sent": n})
}
