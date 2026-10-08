package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

func TestWriteChatStreamIsValidSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	content := strings.Repeat("写", 150) // > 2 chunks, multi-byte runes
	writeChatStream(c, domain.ChatResponse{ID: "r1", Model: "m", Created: 1,
		Choices: []domain.ChatChoice{{Message: domain.ChatMessage{Role: "assistant", Content: content}, FinishReason: "stop"}},
		Usage:   domain.Usage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}})

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	var got strings.Builder
	var usage *domain.Usage
	events := strings.Split(strings.TrimSpace(w.Body.String()), "\n\n")
	if events[len(events)-1] != "data: [DONE]" {
		t.Fatalf("stream must end with [DONE], got %q", events[len(events)-1])
	}
	for _, ev := range events[:len(events)-1] {
		var ch chunk
		if err := json.Unmarshal([]byte(strings.TrimPrefix(ev, "data: ")), &ch); err != nil {
			t.Fatalf("bad event %q: %v", ev, err)
		}
		if ch.Object != "chat.completion.chunk" || ch.ID != "r1" {
			t.Fatalf("bad chunk %+v", ch)
		}
		for _, c := range ch.Choices {
			got.WriteString(c.Delta["content"])
		}
		if ch.Usage != nil {
			usage = ch.Usage
		}
	}
	if got.String() != content || usage == nil || usage.TotalTokens != 8 {
		t.Fatalf("reassembled content/usage mismatch: %d runes, usage %+v", len([]rune(got.String())), usage)
	}
}

func TestChatMessageAcceptsContentParts(t *testing.T) {
	var req domain.ChatRequest
	body := `{"model":"m","stream":true,"messages":[{"role":"system","content":null},{"role":"user","content":[{"type":"text","text":"重构这个函数"},{"type":"image_url","image_url":{"url":"x"}},{"type":"text","text":"保持接口不变"}]}]}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	if !req.Stream || req.Messages[0].Content != "" || req.Messages[1].Content != "重构这个函数\n[image]\n保持接口不变" {
		t.Fatalf("got %+v", req.Messages)
	}
	if err := json.Unmarshal([]byte(`{"messages":[{"role":"user","content":42}]}`), &req); err == nil {
		t.Fatal("numeric content must be rejected")
	}
}
