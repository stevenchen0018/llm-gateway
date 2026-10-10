package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// nodeHeader tags every response with the node that served it, so requests
// behind a load balancer can be traced to a gateway instance.
func nodeHeader(node string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Gateway-Node", node)
		c.Next()
	}
}

// apiPrefixes never fall back to the console's index.html: an unknown API
// path must stay a JSON 404 rather than a 200 HTML page.
var apiPrefixes = []string{"/v1/", "/admin/", "/metrics", "/openapi.json", "/healthz", "/readyz"}

// serveConsole serves the built console (web/dist) from root: real files
// as-is (hashed /assets/* cached for a year) and every other GET path as
// index.html so client-side routes survive a page reload.
func serveConsole(r *gin.Engine, root string, log *zap.Logger) {
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err != nil {
		log.Warn("server.web_root has no index.html; console not served", zap.String("web_root", root))
		return
	}
	fs := http.Dir(root)
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		for _, pre := range apiPrefixes {
			if strings.HasPrefix(p, pre) || p == strings.TrimSuffix(pre, "/") {
				c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "not found", "type": "invalid_request_error", "code": "not_found"}})
				return
			}
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}
		if f, err := fs.Open(path.Clean(p)); err == nil {
			st, statErr := f.Stat()
			f.Close()
			if statErr == nil && !st.IsDir() {
				if strings.HasPrefix(p, "/assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				c.FileFromFS(path.Clean(p), fs)
				return
			}
		}
		c.Header("Cache-Control", "no-cache")
		c.File(index)
	})
}
