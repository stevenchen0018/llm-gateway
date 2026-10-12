package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

func TestChatCompletionConvertsBothWays(t *testing.T) {
	var got generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-x:generateContent" || r.Header.Get("x-goog-api-key") != "g-key" {
			t.Errorf("path = %s headers = %v", r.URL.Path, r.Header)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"plan","thought":true},{"text":"Hi!"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"thoughtsTokenCount":3,"totalTokenCount":9},"responseId":"r1"}`))
	}))
	defer srv.Close()

	temp := 0.2
	resp, err := NewClient().ChatCompletion(context.Background(), domain.CallTarget{
		Provider: domain.Provider{BaseURL: srv.URL, AuthValue: "g-key"},
		Model:    domain.Model{ModelKey: "models/gemini-x"},
	}, domain.ChatRequest{Temperature: &temp, Messages: []domain.ChatMessage{
		{Role: "system", Content: "sys"}, {Role: "user", Content: "hello"}, {Role: "assistant", Content: "yo"}, {Role: "user", Content: "again"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.SystemInstruction == nil || got.SystemInstruction.Parts[0].Text != "sys" || len(got.Contents) != 3 ||
		got.Contents[1].Role != "model" || *got.GenerationConfig.Temperature != 0.2 {
		t.Fatalf("request = %+v", got)
	}
	if resp.ID != "r1" || resp.Choices[0].Message.Content != "Hi!" || resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Usage.PromptTokens != 4 || resp.Usage.CompletionTokens != 5 {
		t.Fatalf("usage = %+v", resp.Usage)
	}
}

func TestBlockedPromptIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`))
	}))
	defer srv.Close()
	_, err := NewClient().ChatCompletion(context.Background(), domain.CallTarget{
		Provider: domain.Provider{BaseURL: srv.URL + "/v1beta"}, Model: domain.Model{ModelKey: "g"},
	}, domain.ChatRequest{Messages: []domain.ChatMessage{{Role: "user", Content: "x"}}})
	if err == nil {
		t.Fatal("expected an error for a blocked prompt")
	}
}

func TestEmbeddingsAndListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1beta/models/text-embedding-004:batchEmbedContents":
			_, _ = w.Write([]byte(`{"embeddings":[{"values":[0.1,0.2]},{"values":[0.3]}]}`))
		case "/v1beta/models":
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-flash"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewClient()
	emb, err := c.Embeddings(context.Background(), domain.CallTarget{Provider: domain.Provider{BaseURL: srv.URL}, Model: domain.Model{ModelKey: "text-embedding-004"}},
		domain.EmbeddingsRequest{Input: domain.StringList{"a", "b"}})
	if err != nil || len(emb.Data) != 2 || emb.Data[1].Embedding[0] != 0.3 || emb.Usage.PromptTokens == 0 {
		t.Fatalf("emb = %+v err = %v", emb, err)
	}
	ids, err := c.ListModels(context.Background(), domain.Provider{BaseURL: srv.URL})
	if err != nil || len(ids) != 1 || ids[0] != "gemini-2.5-flash" {
		t.Fatalf("ids = %v err = %v", ids, err)
	}
}
