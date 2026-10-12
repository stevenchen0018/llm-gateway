package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

func TestChatCompletionConvertsBothWays(t *testing.T) {
	var got messagesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "sk-ant" || r.Header.Get("anthropic-version") != APIVersion {
			t.Errorf("headers = %v", r.Header)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-x",
			"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Hello"},{"type":"text","text":" world"}],
			"stop_reason":"max_tokens","usage":{"input_tokens":10,"cache_read_input_tokens":5,"output_tokens":7}}`))
	}))
	defer srv.Close()

	maxTok := 64
	resp, err := NewClient().ChatCompletion(context.Background(), domain.CallTarget{
		Provider: domain.Provider{BaseURL: srv.URL, AuthValue: "sk-ant"},
		Model:    domain.Model{ModelKey: "claude-x"},
	}, domain.ChatRequest{MaxTokens: &maxTok, Messages: []domain.ChatMessage{
		{Role: "system", Content: "be brief"}, {Role: "user", Content: "hi"}, {Role: "user", Content: "there"}, {Role: "assistant", Content: "yes?"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.System != "be brief" || got.MaxTokens != 64 || len(got.Messages) != 2 || got.Messages[0].Content != "hi\n\nthere" {
		t.Fatalf("request = %+v", got)
	}
	c := resp.Choices[0]
	if c.Message.Content != "Hello world" || c.FinishReason != "length" {
		t.Fatalf("choice = %+v", c)
	}
	if resp.Usage.PromptTokens != 15 || resp.Usage.CompletionTokens != 7 || resp.Usage.TotalTokens != 22 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

func TestErrorsCarryStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	_, err := NewClient().ListModels(context.Background(), domain.Provider{BaseURL: srv.URL + "/v1"})
	var ue *domain.UpstreamError
	if !errors.As(err, &ue) || ue.Status != 401 || ue.Message != "authentication_error: invalid x-api-key" {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, domain.ErrUpstream) {
		t.Fatal("UpstreamError must match ErrUpstream")
	}
}
