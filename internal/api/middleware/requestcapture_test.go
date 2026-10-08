package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func TestInspectBody(t *testing.T) {
	cases := []struct{ body, model, preview string }{
		{`{"model":"gpt-4o","messages":[{"role":"system","content":"sys"},{"role":"user","content":"第一问"},{"role":"assistant","content":"a"},{"role":"user","content":"最后一问"}]}`, "gpt-4o", "最后一问"},
		{`{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x"}},{"type":"text","text":"看图"}]}]}`, "m", "看图"},
		{`{"model":"c","prompt":"legacy prompt"}`, "c", "legacy prompt"},
		{`{"model":"e","input":["first","second"]}`, "e", "first"},
		{`not json`, "", ""},
	}
	for _, c := range cases {
		m, p := inspectBody([]byte(c.body))
		if m != c.model || p != c.preview {
			t.Errorf("inspectBody(%s) = %q,%q; want %q,%q", c.body, m, p, c.model, c.preview)
		}
	}
	_, long := inspectBody([]byte(`{"prompt":"` + strings.Repeat("长", 500) + `"}`))
	if utf8.RuneCountInString(long) != previewRunes+1 { // + ellipsis
		t.Errorf("preview not truncated: %d runes", utf8.RuneCountInString(long))
	}
}

func TestCaptureWriterIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	w := &captureWriter{ResponseWriter: c.Writer, limit: 10}
	_, _ = w.Write([]byte("0123456"))
	_, _ = w.WriteString("789abc")
	if w.buf.String() != "0123456789" || !w.truncated {
		t.Fatalf("buf=%q truncated=%v", w.buf.String(), w.truncated)
	}
}

func TestTruncateBytesKeepsUTF8Valid(t *testing.T) {
	s := strings.Repeat("中", 10) // 3 bytes each
	for n := 0; n < len(s); n++ {
		if out := truncateBytes(s, n); !utf8.ValidString(out) || len(out) > n {
			t.Fatalf("truncateBytes(%d) = %q", n, out)
		}
	}
}
