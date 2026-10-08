package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// PlatformHandler serves 平台管理 (applications, announcements) and the
// "我的模型" list.
type PlatformHandler struct {
	platform *service.PlatformService
}

func NewPlatformHandler(platform *service.PlatformService) *PlatformHandler {
	return &PlatformHandler{platform: platform}
}

func appFromReq(req dto.ApplicationRequest) *domain.Application {
	return &domain.Application{Name: req.Name, Description: req.Description, DepartmentID: req.DepartmentID,
		Owner: req.Owner, OwnerEmail: req.OwnerEmail, Manager: req.Manager, ManagerEmail: req.ManagerEmail, Status: req.Status}
}

func (h *PlatformHandler) CreateApplication(c *gin.Context) {
	var req dto.ApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	a := appFromReq(req)
	if pr := middleware.PrincipalFrom(c); !pr.IsSuper() {
		a.DepartmentID = pr.DepartmentID // department admins create inside their own department
	}
	if err := h.platform.CreateApplication(c.Request.Context(), a); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, a)
}

func (h *PlatformHandler) UpdateApplication(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.ApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := h.platform.GetApplication(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if !ensureOwns(c, existing.DepartmentID) {
		return
	}
	a := appFromReq(req)
	a.ID = id
	if pr := middleware.PrincipalFrom(c); !pr.IsSuper() || a.DepartmentID == nil {
		a.DepartmentID = existing.DepartmentID // only super admins move applications between departments
	}
	if a.Status == "" {
		a.Status = "active"
	}
	if err := h.platform.UpdateApplication(c.Request.Context(), a); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, a)
}

func (h *PlatformHandler) ListApplications(c *gin.Context) {
	if p, ok := pageParams(c); ok {
		items, total, err := h.platform.PageApplications(c.Request.Context(), domain.ApplicationQuery{
			PageQuery: p.query(), DeptID: deptScope(c), Status: c.Query("status"), Q: c.Query("q"),
		})
		if err != nil {
			writeAdminError(c, err)
			return
		}
		respondPage(c, items, total, p)
		return
	}
	apps, err := h.platform.ListApplications(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	scope := deptScope(c)
	respondList(c, filterList(apps, func(a *domain.Application) bool { return inScope(scope, a.DepartmentID) }))
}

func (h *PlatformHandler) ListAnnouncements(c *gin.Context) {
	items, err := h.platform.ListAnnouncements(c.Request.Context(), c.Query("active") == "true")
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondList(c, filterList(items, func(a *domain.Announcement) bool {
		return eqParam(c, "level", a.Level) && matchQ(c, a.Content)
	}))
}

func (h *PlatformHandler) CreateAnnouncement(c *gin.Context) {
	var req dto.AnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	a := &domain.Announcement{Content: req.Content, Level: req.Level, Active: req.Active == nil || *req.Active}
	if err := h.platform.CreateAnnouncement(c.Request.Context(), a); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, a)
}

func (h *PlatformHandler) UpdateAnnouncement(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.AnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	a := &domain.Announcement{ID: id, Content: req.Content, Level: req.Level, Active: req.Active == nil || *req.Active}
	if a.Level == "" {
		a.Level = "info"
	}
	if err := h.platform.UpdateAnnouncement(c.Request.Context(), a); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, a)
}

func (h *PlatformHandler) DeleteAnnouncement(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.platform.DeleteAnnouncement(c.Request.Context(), id); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"deleted": id})
}

func (h *PlatformHandler) MyModels(c *gin.Context) {
	models, err := h.platform.MyModels(c.Request.Context(), middleware.PrincipalFrom(c).UserID)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondList(c, filterList(models, func(m *domain.ModelWithProvider) bool {
		return eqParam(c, "category", m.Category) && matchQ(c, m.DisplayName, m.ModelKey, m.Provider.Name)
	}))
}

func (h *PlatformHandler) AddMyModel(c *gin.Context) {
	var req dto.AddMyModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.platform.AddMyModel(c.Request.Context(), middleware.PrincipalFrom(c).UserID, req.ModelID, middleware.AdminUserFromContext(c)); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, gin.H{"model_id": req.ModelID})
}

func (h *PlatformHandler) RemoveMyModel(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.platform.RemoveMyModel(c.Request.Context(), middleware.PrincipalFrom(c).UserID, id); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"removed": id})
}
