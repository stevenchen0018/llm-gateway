package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/gemini"
	"github.com/stevenchen/llm-gateway/internal/domain"
)

func TestAnthropicRequestToChat(t *testing.T) {
	var in anthropicRequest
	body := `{"model":"claude-x","max_tokens":128,"system":[{"type":"text","text":"你是助手"}],"messages":[
		{"role":"user","content":"你好"},
		{"role":"assistant","content":[{"type":"text","text":"查一下"},{"type":"tool_use","name":"search","input":{"q":"天气"}}]},
		{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"晴"}]},{"type":"image","source":{}}]}]}`
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatal(err)
	}
	req, err := in.toChat()
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "claude-x" || *req.MaxTokens != 128 || len(req.Messages) != 4 {
		t.Fatalf("req = %+v", req)
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "你是助手" {
		t.Fatalf("system = %+v", req.Messages[0])
	}
	if !strings.Contains(req.Messages[2].Content, `[tool_use search {"q":"天气"}]`) || req.Messages[3].Content != "[tool_result] 晴\n[image]" {
		t.Fatalf("blocks = %q / %q", req.Messages[2].Content, req.Messages[3].Content)
	}
	if _, err := anthropicText(json.RawMessage(`42`)); err == nil {
		t.Fatal("numeric content must be rejected")
	}
}

func TestAnthropicStreamEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	content := strings.Repeat("流", 150)
	m := toAnthropic(domain.ChatResponse{ID: "abc-1", Choices: []domain.ChatChoice{{Message: domain.ChatMessage{Content: content}, FinishReason: "length"}},
		Usage: domain.Usage{PromptTokens: 4, CompletionTokens: 9}}, "claude-x")
	if m.ID != "msg_abc1" || m.StopReason != "max_tokens" || m.Usage.OutputTokens != 9 {
		t.Fatalf("message = %+v", m)
	}
	writeAnthropicStream(c, m)

	var order []string
	var text strings.Builder
	for _, ev := range strings.Split(strings.TrimSpace(w.Body.String()), "\n\n") {
		lines := strings.SplitN(ev, "\n", 2)
		name := strings.TrimPrefix(lines[0], "event: ")
		var data map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &data); err != nil || data["type"] != name {
			t.Fatalf("bad event %q", ev)
		}
		order = append(order, name)
		if name == "content_block_delta" {
			text.WriteString(data["delta"].(map[string]any)["text"].(string))
		}
	}
	if order[0] != "message_start" || order[len(order)-1] != "message_stop" || order[len(order)-2] != "message_delta" {
		t.Fatalf("event order = %v", order)
	}
	if text.String() != content {
		t.Fatal("reassembled text mismatch")
	}
}

func TestGeminiRequestAndStream(t *testing.T) {
	var in geminiRequest
	body := `{"systemInstruction":{"parts":[{"text":"简洁"}]},"generationConfig":{"maxOutputTokens":50,"temperature":0.3},
		"contents":[{"role":"user","parts":[{"text":"讲个"},{"text":"笑话"}]},{"role":"model","parts":[{"text":"想","thought":true},{"text":"好"}]},{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"x"}}]}]}`
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatal(err)
	}
	req := in.toChat("deepseek-v3")
	if req.Model != "deepseek-v3" || *req.MaxTokens != 50 || len(req.Messages) != 4 || req.Messages[1].Content != "讲个\n笑话" ||
		req.Messages[2].Role != "assistant" || req.Messages[2].Content != "好" || req.Messages[3].Content != "[media]" {
		t.Fatalf("req = %+v", req)
	}

	for _, alt := range []string{"sse", ""} {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/v1beta/models/x:streamGenerateContent?alt="+alt, nil)
		text := strings.Repeat("双", 100)
		full := geminiResponse{Candidates: []geminiCandidate{{FinishReason: "STOP"}}, UsageMetadata: &gemini.UsageMetadata{TotalTokenCount: 7}, ResponseID: "r"}
		writeGeminiStream(c, full, text)

		var chunks []geminiResponse
		if alt == "sse" {
			for _, ev := range strings.Split(strings.TrimSpace(w.Body.String()), "\r\n\r\n") {
				var ch geminiResponse
				if err := json.Unmarshal([]byte(strings.TrimPrefix(ev, "data: ")), &ch); err != nil {
					t.Fatalf("bad sse event %q", ev)
				}
				chunks = append(chunks, ch)
			}
		} else if err := json.Unmarshal(w.Body.Bytes(), &chunks); err != nil {
			t.Fatalf("json array stream invalid: %v", err)
		}
		var got strings.Builder
		for _, ch := range chunks {
			got.WriteString(ch.Candidates[0].Content.Parts[0].Text)
		}
		last := chunks[len(chunks)-1]
		if got.String() != text || last.Candidates[0].FinishReason != "STOP" || last.UsageMetadata.TotalTokenCount != 7 || chunks[0].UsageMetadata != nil {
			t.Fatalf("alt=%q: reassembly/last chunk mismatch", alt)
		}
	}
}
