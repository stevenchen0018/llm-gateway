// Package response provides consistent JSON response helpers for both the
// OpenAI-compatible gateway API and the admin management API.
package response

import "github.com/gin-gonic/gin"

// OpenAIError mirrors the OpenAI error envelope so gateway clients written
// against the OpenAI SDK work unmodified against this gateway.
type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Param   string `json:"param,omitempty"`
}

type OpenAIErrorEnvelope struct {
	Error OpenAIError `json:"error"`
}

// CtxErrorCode is where GatewayError leaves the error code for the request
// log middleware.
const CtxErrorCode = "gateway_error_code"

// CtxProtocol is where the gateway routes record which client protocol a
// request speaks, so every error (auth, quota, upstream...) is rendered in
// that protocol's own envelope.
const CtxProtocol = "gateway_protocol"

// Client protocols of the gateway API.
const (
	ProtocolOpenAI    = "openai"
	ProtocolAnthropic = "anthropic"
	ProtocolGemini    = "gemini"
)

// SetProtocol marks the request as speaking protocol p.
func SetProtocol(p string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(CtxProtocol, p)
		c.Next()
	}
}

// GatewayError writes an error response in the request's protocol: the
// OpenAI envelope by default, the Anthropic or Gemini envelope on those
// routes. code is the gateway's stable error code in every format.
func GatewayError(c *gin.Context, status int, errType, code, message string) {
	c.Set(CtxErrorCode, code)
	switch c.GetString(CtxProtocol) {
	case ProtocolAnthropic:
		c.AbortWithStatusJSON(status, gin.H{"type": "error", "error": gin.H{"type": anthropicErrorType(status), "message": message, "code": code}})
	case ProtocolGemini:
		c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": status, "message": message, "status": googleStatus(status),
			"details": []gin.H{{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": code, "domain": "llm-gateway"}}}})
	default:
		c.AbortWithStatusJSON(status, OpenAIErrorEnvelope{
			Error: OpenAIError{
				Message: message,
				Type:    errType,
				Code:    code,
			},
		})
	}
}

// anthropicErrorType maps an HTTP status to the Anthropic error type.
func anthropicErrorType(status int) string {
	switch status {
	case 400:
		return "invalid_request_error"
	case 401:
		return "authentication_error"
	case 402:
		return "billing_error"
	case 403:
		return "permission_error"
	case 404:
		return "not_found_error"
	case 413:
		return "request_too_large"
	case 429:
		return "rate_limit_error"
	case 503, 529:
		return "overloaded_error"
	}
	if status < 500 {
		return "invalid_request_error"
	}
	return "api_error"
}

// googleStatus maps an HTTP status to the google.rpc status name.
func googleStatus(status int) string {
	switch status {
	case 400, 413:
		return "INVALID_ARGUMENT"
	case 401:
		return "UNAUTHENTICATED"
	case 402, 429:
		return "RESOURCE_EXHAUSTED"
	case 403:
		return "PERMISSION_DENIED"
	case 404:
		return "NOT_FOUND"
	case 501:
		return "UNIMPLEMENTED"
	case 502, 503:
		return "UNAVAILABLE"
	case 504:
		return "DEADLINE_EXCEEDED"
	}
	if status < 500 {
		return "FAILED_PRECONDITION"
	}
	return "INTERNAL"
}

// Admin API envelopes: a plain {"data": ...} on success and
// {"error": {"message": ...}} on failure, matching common REST admin-console
// conventions and keeping the two API surfaces visually distinct.
type AdminEnvelope struct {
	Data any `json:"data,omitempty"`
}

type AdminErrorEnvelope struct {
	Error AdminErrorBody `json:"error"`
}

type AdminErrorBody struct {
	Message string `json:"message"`
}

func AdminOK(c *gin.Context, status int, data any) {
	c.JSON(status, AdminEnvelope{Data: data})
}

func AdminError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, AdminErrorEnvelope{Error: AdminErrorBody{Message: message}})
}
