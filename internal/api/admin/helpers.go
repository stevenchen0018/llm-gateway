package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/pgerr"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

// schemaHint is shown when the database is missing tables/columns the code needs.
const schemaHint = "数据库结构未初始化或版本落后：请启用 postgres.auto_migrate 或执行 make migrate-up 后重试"

func writeAdminError(c *gin.Context, err error) {
	_ = c.Error(err) // full detail goes to the server log (RequestLog), never to the client
	switch {
	case pgerr.IsSchemaMismatch(err):
		response.AdminError(c, http.StatusServiceUnavailable, schemaHint)
	case errors.Is(err, domain.ErrNotFound):
		response.AdminError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidArgument):
		response.AdminError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		msg := "无权访问该资源（角色或部门不匹配）"
		if err != domain.ErrForbidden {
			msg = err.Error()
		}
		response.AdminError(c, http.StatusForbidden, msg)
	case errors.Is(err, service.ErrTooManyAttempts):
		response.AdminError(c, http.StatusTooManyRequests, "登录失败次数过多，请 15 分钟后再试")
	case errors.Is(err, domain.ErrUnauthorized):
		response.AdminError(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrUpstream):
		response.AdminError(c, http.StatusBadGateway, err.Error())
	default:
		response.AdminError(c, http.StatusInternalServerError, "internal server error")
	}
}

var timeLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"}

// parseTimeParam accepts RFC3339 or local-time layouts (date-only means local
// midnight), so console date pickers work in the operator's timezone.
func parseTimeParam(s string) (time.Time, bool) {
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func queryTime(c *gin.Context, name string, def time.Time) time.Time {
	if v := c.Query(name); v != "" {
		if t, ok := parseTimeParam(v); ok {
			return t
		}
	}
	return def
}

func queryInt64Ptr(c *gin.Context, name string) *int64 {
	if v := c.Query(name); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return &n
		}
	}
	return nil
}

func queryInt(c *gin.Context, name string, def int) int {
	if v := c.Query(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// metricsFilter builds a domain.MetricsFilter from the common query params
// (from,to,key_id,model_id,provider_id,app_id,category). Default window: the
// last defaultWindow.
func metricsFilter(c *gin.Context, defaultWindow time.Duration) domain.MetricsFilter {
	now := time.Now()
	to := queryTime(c, "to", now.Add(time.Minute))
	from := queryTime(c, "from", to.Add(-defaultWindow))
	return domain.MetricsFilter{
		From: from, To: to,
		KeyID: queryInt64Ptr(c, "key_id"), ModelID: queryInt64Ptr(c, "model_id"),
		ProviderID: queryInt64Ptr(c, "provider_id"), AppID: queryInt64Ptr(c, "app_id"),
		Category: c.Query("category"), DepartmentID: deptScope(c),
		VendorID: queryInt64Ptr(c, "vendor_id"), KeyCategory: c.Query("key_category"),
	}
}

// deptScope is the department filter for list and metrics queries:
// department users are always confined to their own department; super
// admins see everything, or one department via ?department_id=.
func deptScope(c *gin.Context) *int64 {
	p := middleware.PrincipalFrom(c)
	if !p.IsSuper() {
		if p.DepartmentID == nil {
			none := int64(-1) // a non-super account without a department sees nothing
			return &none
		}
		return p.DepartmentID
	}
	return queryInt64Ptr(c, "department_id")
}

// inScope reports whether a resource owned by dept passes the scope filter.
func inScope(scope, dept *int64) bool {
	return scope == nil || (dept != nil && *dept == *scope)
}

// ensureOwns writes 403 and returns false unless the caller may act on a
// resource owned by dept.
func ensureOwns(c *gin.Context, dept *int64) bool {
	if middleware.PrincipalFrom(c).Owns(dept) {
		return true
	}
	writeAdminError(c, domain.ErrForbidden)
	return false
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
