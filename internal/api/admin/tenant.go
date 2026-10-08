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

// TenantHandler serves 部门管理 and 用户管理. Permission rules live in
// IdentityService / TenantService; handlers only translate HTTP.
type TenantHandler struct {
	identity *service.IdentityService
	tenants  *service.TenantService
}

func NewTenantHandler(identity *service.IdentityService, tenants *service.TenantService) *TenantHandler {
	return &TenantHandler{identity: identity, tenants: tenants}
}

func (h *TenantHandler) ListDepartments(c *gin.Context) {
	out, err := h.tenants.List(c.Request.Context(), middleware.PrincipalFrom(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondList(c, filterList(out, func(d service.DepartmentView) bool {
		return eqParam(c, "status", d.Status) && matchQ(c, d.Name, d.Code, d.Leader, d.Description)
	}))
}

func deptFromReq(req dto.DepartmentRequest) *domain.Department {
	return &domain.Department{Code: req.Code, Name: req.Name, Description: req.Description, Leader: req.Leader,
		TPMQuota: req.TPMQuota, QPSQuota: req.QPSQuota, Status: req.Status}
}

func (h *TenantHandler) CreateDepartment(c *gin.Context) {
	var req dto.DepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	d := deptFromReq(req)
	if err := h.tenants.Create(c.Request.Context(), middleware.PrincipalFrom(c), d); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, d)
}

func (h *TenantHandler) UpdateDepartment(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.DepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	d := deptFromReq(req)
	d.ID = id
	if d.Status == "" {
		d.Status = "active"
	}
	if err := h.tenants.Update(c.Request.Context(), middleware.PrincipalFrom(c), d); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, d)
}

func (h *TenantHandler) DeleteDepartment(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.tenants.Delete(c.Request.Context(), middleware.PrincipalFrom(c), id); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"deleted": id})
}

func (h *TenantHandler) ListUsers(c *gin.Context) {
	users, err := h.identity.ListUsers(c.Request.Context(), middleware.PrincipalFrom(c), queryInt64Ptr(c, "department_id"))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondList(c, filterList(users, func(u *domain.AdminUser) bool {
		return eqParam(c, "role", string(u.Role)) && eqParam(c, "status", u.Status) && matchQ(c, u.Username, u.DisplayName, u.Email)
	}))
}

func (h *TenantHandler) CreateUser(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	u := &domain.AdminUser{Username: req.Username, DisplayName: req.DisplayName, Email: req.Email,
		Role: domain.Role(req.Role), DepartmentID: req.DepartmentID}
	if err := h.identity.CreateUser(c.Request.Context(), middleware.PrincipalFrom(c), u, req.Password); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, u)
}

func (h *TenantHandler) UpdateUser(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	patch := service.UserPatch{DisplayName: req.DisplayName, Email: req.Email, DepartmentID: req.DepartmentID, Status: req.Status}
	if req.Role != nil {
		r := domain.Role(*req.Role)
		patch.Role = &r
	}
	u, err := h.identity.UpdateUser(c.Request.Context(), middleware.PrincipalFrom(c), id, patch)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, u)
}

func (h *TenantHandler) ResetPassword(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.identity.ResetPassword(c.Request.Context(), middleware.PrincipalFrom(c), id, req.Password); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"reset": id})
}
