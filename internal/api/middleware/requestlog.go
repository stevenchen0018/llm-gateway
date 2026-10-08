package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLog emits one structured access-log line per HTTP request. Deeper
// per-call domain fields (key/model/tokens/cost/prompt) are logged
// separately by GatewayService once the gateway pipeline has resolved
// them; this middleware covers every route uniformly, including admin API
// calls and error responses that never reach that pipeline.
func RequestLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		}
		// handlers attach the real cause with c.Error(); clients only see a
		// sanitized message, so this is where the detail is recorded
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("error", c.Errors.String()))
			log.Error("http_request_failed", fields...)
			return
		}
		log.Info("http_request", fields...)
	}
}
