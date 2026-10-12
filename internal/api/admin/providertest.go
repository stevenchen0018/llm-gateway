package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// ProviderTestHandler serves supplier availability tests and their reports
// (供应商可用性测试 / 测试报告).
type ProviderTestHandler struct {
	tests *service.ProviderTestService
}

func NewProviderTestHandler(tests *service.ProviderTestService) *ProviderTestHandler {
	return &ProviderTestHandler{tests: tests}
}

func daysParam(c *gin.Context) int {
	n, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	return n
}

// Start runs a test of one supplier: POST /providers/:id/tests.
func (h *ProviderTestHandler) Start(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var opts service.ProviderTestOptions
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&opts); err != nil {
			response.AdminError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	run, err := h.tests.Start(c.Request.Context(), id, opts, domain.TestTriggerManual, middleware.AdminUserFromContext(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusAccepted, run)
}

// StartAll tests every active supplier: POST /provider-tests/run-all.
func (h *ProviderTestHandler) StartAll(c *gin.Context) {
	var opts service.ProviderTestOptions
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&opts); err != nil {
			response.AdminError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	runs, err := h.tests.RunAll(c.Request.Context(), domain.TestTriggerBatch, middleware.AdminUserFromContext(c), opts)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if runs == nil {
		runs = []*domain.ProviderTestRun{}
	}
	response.AdminOK(c, http.StatusAccepted, runs)
}

// Run returns one run with its check results (poll it while running).
func (h *ProviderTestHandler) Run(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	run, err := h.tests.GetRun(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, run)
}

// Runs lists runs, newest first: GET /provider-tests/runs?provider_id=&trigger=&status=&page=&page_size=.
func (h *ProviderTestHandler) Runs(c *gin.Context) {
	p := alwaysPaged(c)
	pid, _ := strconv.ParseInt(c.Query("provider_id"), 10, 64)
	runs, total, err := h.tests.ListRuns(c.Request.Context(), domain.ProviderTestRunQuery{
		PageQuery: p.query(), ProviderID: pid, Trigger: c.Query("trigger"), Status: c.Query("status")})
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondPage(c, runs, total, p)
}

// Report is the supplier's test report: GET /providers/:id/test-report?days=7.
func (h *ProviderTestHandler) Report(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	rep, err := h.tests.Report(c.Request.Context(), id, daysParam(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, rep)
}

// Overview lists every supplier's availability: GET /provider-tests/overview?days=7.
func (h *ProviderTestHandler) Overview(c *gin.Context) {
	rows, err := h.tests.Overview(c.Request.Context(), daysParam(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, rows)
}

func (h *ProviderTestHandler) Schedule(c *gin.Context) {
	st, err := h.tests.Schedule(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, st)
}

func (h *ProviderTestHandler) UpdateSchedule(c *gin.Context) {
	var st domain.ProviderTestSchedule
	if err := c.ShouldBindJSON(&st); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	st, err := h.tests.UpdateSchedule(c.Request.Context(), st, middleware.AdminUserFromContext(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, st)
}
