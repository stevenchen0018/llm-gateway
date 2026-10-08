package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/pkg/response"
)

// Recovery converts a panic in any handler into a well-formed error
// response instead of an aborted connection, using the OpenAI-style
// envelope for /v1/* and the admin envelope otherwise.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		log.Error("panic recovered", zap.Any("recovered", recovered), zap.String("path", c.Request.URL.Path))

		if len(c.Request.URL.Path) >= 3 && c.Request.URL.Path[:3] == "/v1" {
			response.GatewayError(c, http.StatusInternalServerError, "internal_error", "panic", "Internal server error")
			return
		}
		response.AdminError(c, http.StatusInternalServerError, "internal server error")
	})
}
