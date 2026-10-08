// Package docs serves the OpenAPI description of the gateway's business API
// (/v1/*) so integrators — and the console's API docs page — have a single,
// machine-readable source of truth.
package docs

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.json
var spec []byte

var parsed = func() map[string]any {
	var m map[string]any
	if err := json.Unmarshal(spec, &m); err != nil {
		panic("docs: invalid embedded openapi.json: " + err.Error())
	}
	return m
}()

// OpenAPIHandler returns the spec with `servers` pointing at the host the
// caller used, so "try it" tools and generated SDKs hit the right gateway.
func OpenAPIHandler(c *gin.Context) {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if p := c.GetHeader("X-Forwarded-Proto"); p == "https" || p == "http" {
		scheme = p
	}
	host := c.Request.Host
	if h := c.GetHeader("X-Forwarded-Host"); h != "" {
		host = h
	}
	out := make(map[string]any, len(parsed))
	for k, v := range parsed {
		out[k] = v
	}
	out["servers"] = []map[string]string{{"url": scheme + "://" + host + "/v1", "description": "当前网关"}}
	c.Header("Cache-Control", "no-cache")
	c.JSON(http.StatusOK, out)
}
