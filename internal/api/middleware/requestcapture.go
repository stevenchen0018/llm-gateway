package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/idgen"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// MaxGatewayBody caps a /v1 request body; larger requests fail to bind (400).
const MaxGatewayBody = 8 << 20

const previewRunes = 200

// captureWriter tees the response body into a bounded buffer.
type captureWriter struct {
	gin.ResponseWriter
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *captureWriter) capture(b []byte) {
	if room := w.limit - w.buf.Len(); room > 0 {
		if len(b) > room {
			b, w.truncated = b[:room], true
		}
		w.buf.Write(b)
	} else if len(b) > 0 {
		w.truncated = true
	}
}

func (w *captureWriter) Write(b []byte) (int, error) {
	w.capture(b)
	return w.ResponseWriter.Write(b)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// RequestCapture assigns every /v1 call its request id (X-Request-Id, also
// used by usage records), exposes a domain.RequestTrace to the gateway
// pipeline, and — when request logging is switched on — hands a record of
// the call (bodies optional, bounded, optionally redacted) to the async
// RequestLogWorker. It must run before APIKeyAuth so rejected calls from a
// known key (e.g. IP whitelist) are recorded too.
func RequestCapture(settings *service.SettingsService, worker *service.RequestLogWorker, filter *service.ContentFilterService) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		trace := &domain.RequestTrace{RequestID: idgen.RequestID()}
		c.Request = c.Request.WithContext(domain.WithTrace(c.Request.Context(), trace))
		c.Header("X-Request-Id", trace.RequestID)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxGatewayBody)

		st := settings.Get(c.Request.Context()).RequestLog
		if !st.Enabled || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		limit := st.MaxBodyKB * 1024
		var reqBody []byte
		if st.CaptureBody {
			body, err := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), errReader{err}))
			reqBody = body
		}
		cw := &captureWriter{ResponseWriter: c.Writer, limit: limit}
		if st.CaptureBody {
			c.Writer = cw
		}

		c.Next()

		key, ok := apiKeyIfAny(c)
		if !ok {
			return // unauthenticated noise is not attributable to any tenant
		}
		code := c.Writer.Status()
		rec := &domain.RequestLog{
			RequestID: trace.RequestID, KeyID: key.ID, Endpoint: c.FullPath(), HTTPStatus: code,
			LatencyMS: int(time.Since(start).Milliseconds()), SourceIP: c.ClientIP(),
			UserAgent: truncateRunes(c.Request.UserAgent(), 250), CreatedAt: start,
		}
		if v, ok := c.Get(response.CtxErrorCode); ok {
			rec.ErrorCode, _ = v.(string)
		}
		rec.Model, rec.PromptPreview = inspectBody(reqBody)

		// the handler has finished: the trace now holds what the pipeline decided
		rec.ModelID, rec.ProviderID = trace.ModelID, trace.ProviderID
		rec.PromptTokens, rec.CompletionTokens = trace.Usage.PromptTokens, trace.Usage.CompletionTokens
		rec.FilterHits = trace.Hits
		switch {
		case trace.Blocked || rec.ErrorCode == "ip_not_allowed" || rec.ErrorCode == "content_blocked":
			rec.Status = domain.RequestLogBlocked
		case code < 400:
			rec.Status = domain.RequestLogSuccess
		default:
			rec.Status = domain.RequestLogFailed
		}

		if st.CaptureBody {
			body := string(reqBody)
			if len(body) > limit {
				body, rec.BodyTruncated = truncateBytes(body, limit), true
			}
			rec.RequestBody = body
			rec.ResponseBody = strings.ToValidUTF8(cw.buf.String(), "")
			rec.BodyTruncated = rec.BodyTruncated || cw.truncated
			if st.MaskSensitive && filter != nil {
				if mask := filter.Masker(c.Request.Context(), key.DepartmentID); mask != nil {
					rec.RequestBody, rec.ResponseBody, rec.PromptPreview = mask(rec.RequestBody), mask(rec.ResponseBody), mask(rec.PromptPreview)
				}
			}
		} else {
			rec.PromptPreview = ""
		}
		worker.Submit(rec)
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return 0, io.EOF
}

// inspectBody extracts the requested model and a short preview of the
// prompt (last user message / prompt / first input) from a /v1 JSON body.
func inspectBody(body []byte) (model, preview string) {
	if len(body) == 0 {
		return "", ""
	}
	var b struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
		Prompt any `json:"prompt"`
		Input  any `json:"input"`
	}
	if json.Unmarshal(body, &b) != nil {
		return "", ""
	}
	model = truncateRunes(b.Model, 120)
	for i := len(b.Messages) - 1; i >= 0; i-- {
		if b.Messages[i].Role == "user" {
			preview = textOf(b.Messages[i].Content)
			break
		}
	}
	if preview == "" && len(b.Messages) > 0 {
		preview = textOf(b.Messages[len(b.Messages)-1].Content)
	}
	if preview == "" {
		preview = textOf(b.Prompt)
	}
	if preview == "" {
		preview = textOf(b.Input)
	}
	return model, truncateRunes(strings.TrimSpace(preview), previewRunes)
}

func textOf(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		for _, e := range x {
			if t := textOf(e); t != "" {
				return t
			}
		}
	case map[string]any: // multimodal content part {"type":"text","text":...}
		if t, ok := x["text"].(string); ok {
			return t
		}
	}
	return ""
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// truncateBytes cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
