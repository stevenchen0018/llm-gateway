package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
)

// Pagination contract shared by every admin list endpoint:
//
//   - request:  ?page=1&page_size=20 (page_size 1–200, default 20), plus the
//     endpoint's own filters (q, status, ...)
//   - response: {"items": [...], "total": N, "page": P, "page_size": S}
//
// A request WITHOUT page/page_size keeps the original behaviour and receives
// the plain (filtered) array, so dropdowns and older clients stay compatible.

const (
	defaultPageSize = 20
	maxPageSize     = 200
)

type pageReq struct {
	Page, Size int
}

func (p pageReq) query() domain.PageQuery {
	return domain.PageQuery{Offset: (p.Page - 1) * p.Size, Limit: p.Size}
}

// Page is the paged response envelope.
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// pageParams reports whether the caller asked for a page, and which.
func pageParams(c *gin.Context) (pageReq, bool) {
	if c.Query("page") == "" && c.Query("page_size") == "" {
		return pageReq{}, false
	}
	p := pageReq{Page: 1, Size: defaultPageSize}
	if n, err := strconv.Atoi(c.Query("page")); err == nil && n > 0 {
		p.Page = n
	}
	if n, err := strconv.Atoi(c.Query("page_size")); err == nil && n > 0 {
		p.Size = min(n, maxPageSize)
	}
	return p, true
}

// alwaysPaged is for endpoints that only exist in paged form (call logs,
// request records, key search): missing params fall back to page 1.
func alwaysPaged(c *gin.Context) pageReq {
	p, ok := pageParams(c)
	if !ok {
		p = pageReq{Page: 1, Size: defaultPageSize}
	}
	return p
}

func respondPage[T any](c *gin.Context, items []T, total int64, p pageReq) {
	if items == nil {
		items = []T{}
	}
	response.AdminOK(c, http.StatusOK, Page[T]{Items: items, Total: total, Page: p.Page, PageSize: p.Size})
}

// respondList serves an already-loaded, already-filtered list: the whole
// array for legacy callers, or one page of it. Used for bounded
// configuration lists (vendors, departments, users, rules...); unbounded
// tables page in SQL instead (see respondPage callers).
func respondList[T any](c *gin.Context, all []T) {
	p, paged := pageParams(c)
	if !paged {
		if all == nil {
			all = []T{}
		}
		response.AdminOK(c, http.StatusOK, all)
		return
	}
	total := len(all)
	from := min((p.Page-1)*p.Size, total)
	to := min(from+p.Size, total)
	respondPage(c, all[from:to], int64(total), p)
}

// filterList keeps the items for which keep returns true.
func filterList[T any](items []T, keep func(T) bool) []T {
	out := make([]T, 0, len(items))
	for _, it := range items {
		if keep(it) {
			out = append(out, it)
		}
	}
	return out
}

// matchQ reports whether the ?q= keyword (case-insensitive) occurs in any
// of fields; an empty keyword matches everything.
func matchQ(c *gin.Context, fields ...string) bool {
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if q == "" {
		return true
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// eqParam: an absent/empty query parameter matches everything.
func eqParam(c *gin.Context, name, value string) bool {
	v := c.Query(name)
	return v == "" || v == value
}

// queryBoolPtr: "true"/"false" → pointer; absent or anything else → nil.
func queryBoolPtr(c *gin.Context, name string) *bool {
	switch c.Query(name) {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	}
	return nil
}
