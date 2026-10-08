package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/jwtauth"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

const ctxPrincipal = "principal"

// JWTAuth guards the /admin/v1 management API. The token only identifies the
// user; the role and department are loaded fresh so disabling an account or
// changing its role takes effect on the next request.
func JWTAuth(secret string, identity *service.IdentityService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || token == "" {
			response.AdminError(c, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		uid, err := jwtauth.Parse(secret, token)
		if err != nil {
			response.AdminError(c, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		p, _, err := identity.Principal(c.Request.Context(), uid)
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			response.AdminError(c, http.StatusUnauthorized, "account no longer exists")
			return
		case errors.Is(err, domain.ErrForbidden):
			response.AdminError(c, http.StatusUnauthorized, err.Error())
			return
		case err != nil:
			_ = c.Error(err)
			response.AdminError(c, http.StatusInternalServerError, "internal server error")
			return
		}
		c.Set(ctxPrincipal, p)
		c.Next()
	}
}

// RequireSuper restricts a route to super admins.
func RequireSuper() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !PrincipalFrom(c).IsSuper() {
			response.AdminError(c, http.StatusForbidden, "该操作仅超级管理员可执行")
			return
		}
		c.Next()
	}
}

// RequireWriter rejects read-only (viewer) accounts.
func RequireWriter() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !PrincipalFrom(c).CanWrite() {
			response.AdminError(c, http.StatusForbidden, "只读账号无权执行该操作")
			return
		}
		c.Next()
	}
}

func PrincipalFrom(c *gin.Context) domain.Principal {
	v, _ := c.Get(ctxPrincipal)
	p, _ := v.(domain.Principal)
	return p
}

// AdminUserFromContext returns the operator's username (for audit fields).
func AdminUserFromContext(c *gin.Context) string {
	return PrincipalFrom(c).Username
}
