package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/pgerr"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

const ctxAPIKey = "api_key"

// APIKeyAuth guards the OpenAI-compatible gateway API (/v1/*). It expects
// "Authorization: Bearer sk-...", resolves it through APIKeyService, and
// rejects keys that are pending/blacklisted/expired — the middleware side
// of the article's Key lifecycle.
//
// A key with an IP whitelist only accepts calls whose client address (see
// server.trusted_proxies for how it is derived) matches an entry.
func APIKeyAuth(keys *service.APIKeyService, alerts *service.AlertService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		secret, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || secret == "" {
			response.GatewayError(c, http.StatusUnauthorized, "invalid_request_error", "missing_api_key", "Missing or malformed Authorization header")
			return
		}

		key, err := keys.Authenticate(c.Request.Context(), secret)
		if err != nil {
			if errors.Is(err, domain.ErrUnauthorized) {
				response.GatewayError(c, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", "Invalid API key")
				return
			}
			_ = c.Error(err)
			if pgerr.IsSchemaMismatch(err) {
				response.GatewayError(c, http.StatusServiceUnavailable, "service_unavailable", "schema_not_ready", "Service database is not initialized; please contact the administrator")
				return
			}
			response.GatewayError(c, http.StatusInternalServerError, "internal_error", "auth_backend_error", "Authentication backend unavailable")
			return
		}
		if !key.IsUsable(time.Now()) {
			response.GatewayError(c, http.StatusUnauthorized, "invalid_request_error", "inactive_api_key", "API key is not active")
			return
		}

		c.Set(ctxAPIKey, *key)
		if ip := c.ClientIP(); !key.AllowsIP(ip) {
			alerts.Raise(c.Request.Context(), domain.AlertTypeSecurity, key.ID, &key.ID,
				fmt.Sprintf("Key %s 收到来自非白名单 IP %s 的调用，已拒绝", key.Name, ip), domain.AlertLevelWarning)
			response.GatewayError(c, http.StatusForbidden, "permission_error", "ip_not_allowed",
				fmt.Sprintf("Source IP %s is not in this API key's whitelist", ip))
			return
		}
		c.Next()
	}
}

// APIKeyFromContext retrieves the authenticated key set by APIKeyAuth.
func APIKeyFromContext(c *gin.Context) domain.APIKey {
	key, _ := apiKeyIfAny(c)
	return key
}

// apiKeyIfAny returns the resolved key, also when the request was then
// rejected (e.g. IP whitelist), so the request log can attribute it.
func apiKeyIfAny(c *gin.Context) (domain.APIKey, bool) {
	v, ok := c.Get(ctxAPIKey)
	if !ok {
		return domain.APIKey{}, false
	}
	key, ok := v.(domain.APIKey)
	return key, ok
}
