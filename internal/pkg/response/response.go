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

// GatewayError writes an OpenAI-style error response.
func GatewayError(c *gin.Context, status int, errType, code, message string) {
	c.Set(CtxErrorCode, code)
	c.AbortWithStatusJSON(status, OpenAIErrorEnvelope{
		Error: OpenAIError{
			Message: message,
			Type:    errType,
			Code:    code,
		},
	})
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
