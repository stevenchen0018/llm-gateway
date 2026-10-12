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
	origin := scheme + "://" + host
	out["servers"] = []map[string]string{{"url": origin + "/v1", "description": "当前网关"}}
	out["paths"] = absoluteOpServers(parsed["paths"], origin)
	c.Header("Cache-Control", "no-cache")
	c.JSON(http.StatusOK, out)
}

// absoluteOpServers copies paths, turning relative operation-level servers
// (the Gemini endpoints live under /v1beta) into absolute URLs on this host.
func absoluteOpServers(paths any, origin string) any {
	ps, ok := paths.(map[string]any)
	if !ok {
		return paths
	}
	out := make(map[string]any, len(ps))
	for path, item := range ps {
		ops, ok := item.(map[string]any)
		if !ok {
			out[path] = item
			continue
		}
		copied := make(map[string]any, len(ops))
		for method, op := range ops {
			o, ok := op.(map[string]any)
			servers, has := o["servers"].([]any)
			if !ok || !has {
				copied[method] = op
				continue
			}
			oc := make(map[string]any, len(o))
			for k, v := range o {
				oc[k] = v
			}
			abs := make([]any, len(servers))
			for i, sv := range servers {
				m, _ := sv.(map[string]any)
				u, _ := m["url"].(string)
				if len(u) > 0 && u[0] == '/' {
					u = origin + u
				}
				abs[i] = map[string]any{"url": u, "description": m["description"]}
			}
			oc["servers"] = abs
			copied[method] = oc
		}
		out[path] = copied
	}
	return out
}
