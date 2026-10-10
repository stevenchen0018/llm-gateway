package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestServeConsole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>console</html>"), 0o644)
	os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log(1)"), 0o644)

	r := gin.New()
	r.Use(nodeHeader("gw-test"))
	serveConsole(r, root, zap.NewNop())

	cases := []struct {
		method, path string
		code         int
		body, cache  string
	}{
		{"GET", "/", 200, "console", "no-cache"},
		{"GET", "/keys/mine", 200, "console", "no-cache"},          // SPA route
		{"GET", "/assets/app.js", 200, "console.log", "immutable"}, // hashed asset
		{"GET", "/../../etc/passwd", 400, "", ""},                  // traversal rejected
		{"GET", "/admin/v1/nope", 404, "not_found", ""},
		{"GET", "/v1/nope", 404, "not_found", ""},
		{"POST", "/somewhere", 404, "", ""},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.body) {
			t.Errorf("%s %s = %d %q, want %d containing %q", c.method, c.path, w.Code, w.Body.String(), c.code, c.body)
		}
		if c.cache != "" && !strings.Contains(w.Header().Get("Cache-Control"), c.cache) {
			t.Errorf("%s cache-control = %q, want %q", c.path, w.Header().Get("Cache-Control"), c.cache)
		}
		if w.Header().Get("X-Gateway-Node") != "gw-test" {
			t.Errorf("%s missing X-Gateway-Node", c.path)
		}
	}
}

func TestServeConsoleWithoutIndexIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	serveConsole(r, t.TempDir(), zap.NewNop())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404 when web_root has no index.html", w.Code)
	}
}
