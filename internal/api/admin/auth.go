// Package admin implements the JWT-guarded management API described in the
// article: model marketplace administration, routing/scheduling policy
// configuration, API key lifecycle, budgets, usage/cost dashboards, and
// multi-tenant (department) administration.
package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/config"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/jwtauth"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

type AuthHandler struct {
	identity *service.IdentityService
	depts    domain.DepartmentRepository
	jwt      config.JWTConfig
}

func NewAuthHandler(identity *service.IdentityService, depts domain.DepartmentRepository, jwt config.JWTConfig) *AuthHandler {
	return &AuthHandler{identity: identity, depts: depts, jwt: jwt}
}

// Me is the console's view of the signed-in user and what they may do.
type Me struct {
	*domain.AdminUser
	Department  *domain.Department `json:"department,omitempty"`
	Permissions map[string]bool    `json:"permissions"`
}

func (h *AuthHandler) me(c *gin.Context, u *domain.AdminUser) Me {
	m := Me{AdminUser: u, Permissions: map[string]bool{
		"super":          u.Role == domain.RoleSuperAdmin,
		"write":          u.Role != domain.RoleViewer,
		"manage_users":   u.Role != domain.RoleViewer,
		"manage_depts":   u.Role == domain.RoleSuperAdmin,
		"platform":       u.Role == domain.RoleSuperAdmin, // vendors, models, announcements, global policies
		"view_prompts":   u.Role != domain.RoleViewer,     // request records contain prompts and completions
		"security":       u.Role == domain.RoleSuperAdmin, // gateway settings, platform-wide filter rules
		"apply_personal": u.DepartmentID != nil,           // self-service personal coding key
	}}
	if u.DepartmentID != nil {
		if d, err := h.depts.Get(c.Request.Context(), *u.DepartmentID); err == nil {
			m.Department = d
		}
	}
	return m
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	u, err := h.identity.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if err == domain.ErrUnauthorized {
			response.AdminError(c, http.StatusUnauthorized, "用户名或密码错误")
			return
		}
		writeAdminError(c, err)
		return
	}
	token, err := jwtauth.Issue(h.jwt.Secret, u.ID, u.Username, h.jwt.TTL)
	if err != nil {
		response.AdminError(c, http.StatusInternalServerError, "failed to issue token")
		return
	}
	response.AdminOK(c, http.StatusOK, dto.LoginResponse{Token: token, ExpiresAt: time.Now().Add(h.jwt.TTL), User: h.me(c, u)})
}

func (h *AuthHandler) Me(c *gin.Context) {
	_, u, err := h.identity.Principal(c.Request.Context(), middleware.PrincipalFrom(c).UserID)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, h.me(c, u))
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.identity.ChangePassword(c.Request.Context(), middleware.PrincipalFrom(c).UserID, req.OldPassword, req.NewPassword); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"changed": true})
}
