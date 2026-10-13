package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/openaicompat"
	"github.com/stevenchen/llm-gateway/internal/domain"
)

type memParamRepo struct {
	mu    sync.Mutex
	rules []*domain.ParamRule
}

func (r *memParamRepo) List(context.Context) ([]*domain.ParamRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.ParamRule, len(r.rules))
	for i, x := range r.rules {
		cp := *x
		out[i] = &cp
	}
	return out, nil
}
func (r *memParamRepo) Get(_ context.Context, id int64) (*domain.ParamRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.rules {
		if x.ID == id {
			cp := *x
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *memParamRepo) Create(_ context.Context, p *domain.ParamRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.ID = int64(len(r.rules) + 1)
	cp := *p
	r.rules = append(r.rules, &cp)
	return nil
}
func (r *memParamRepo) Update(_ context.Context, p *domain.ParamRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.rules {
		if x.ID == p.ID {
			cp := *p
			r.rules[i] = &cp
		}
	}
	return nil
}
func (r *memParamRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.rules {
		if x.ID == id {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			break
		}
	}
	return nil
}
func (r *memParamRepo) FindAuto(_ context.Context, pid int64, model, op string) (*domain.ParamRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.rules {
		if x.Source == domain.ParamRuleAuto && x.ProviderID != nil && *x.ProviderID == pid && x.ModelPattern == model && x.Op == op {
			cp := *x
			return &cp, nil
		}
	}
	return nil, nil
}
func (r *memParamRepo) AddHits(context.Context, map[int64]int64, time.Time) error { return nil }

func builtinMaxCompletion() *domain.ParamRule {
	return &domain.ParamRule{ID: 1, Name: "builtin", Enabled: true, Source: domain.ParamRuleBuiltin, Protocol: domain.ProtocolOpenAI,
		ModelPattern: "o1*,gpt-5*,gpt-6*", Op: domain.ParamOpChat, Priority: 100,
		Actions: []domain.ParamAction{{Type: domain.ParamActRename, Field: "max_tokens", To: "max_completion_tokens"}}}
}

func TestMatchPattern(t *testing.T) {
	for _, c := range []struct {
		pattern, model string
		want           bool
	}{
		{"", "anything", true}, {"gpt-6*", "gpt-6.1-sol", true}, {"o1*, gpt-5*", "GPT-5-mini", true},
		{"gpt-5*", "gpt-4o", false}, {"gemini-*", "models/gemini-2.5-flash", true}, {"deepseek-r1", "deepseek-r1", true},
	} {
		if got := matchPattern(c.pattern, c.model); got != c.want {
			t.Errorf("matchPattern(%q, %q) = %v", c.pattern, c.model, got)
		}
	}
}

func TestRejectedParam(t *testing.T) {
	for msg, want := range map[string][2]string{
		"Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.":                     {"max_tokens", "max_completion_tokens"},
		"Unrecognized request argument supplied: max_completion_tokens":                                                                  {"max_completion_tokens", ""},
		"Unsupported value: 'temperature' does not support 0.2 with this model. Only the default (1) value is supported.":                {"temperature", ""},
		"Unknown parameter: 'reasoning_effort'.":                                                                                         {"reasoning_effort", ""},
		`[{'type': 'extra_forbidden', 'loc': ('body', 'max_completion_tokens'), 'msg': 'Extra inputs are not permitted'}]`:               {"max_completion_tokens", ""},
		"max_completion_tokens: Extra inputs are not permitted":                                                                          {"max_completion_tokens", ""},
		`INVALID_ARGUMENT: Invalid JSON payload received. Unknown name "maxCompletionTokens" at 'generation_config': Cannot find field.`: {"generationConfig.maxCompletionTokens", ""},
		"model not found": {"", ""},
	} {
		f, r := rejectedParam(msg)
		if f != want[0] || r != want[1] {
			t.Errorf("rejectedParam(%q) = %q, %q; want %q, %q", msg, f, r, want[0], want[1])
		}
	}
}

func TestRewriteOrderAndSupplierOverride(t *testing.T) {
	legacy := int64(7)
	repo := &memParamRepo{rules: []*domain.ParamRule{
		// supplier-specific rule (added first) must still run after the preset
		{ID: 2, Name: "legacy supplier", Enabled: true, Source: domain.ParamRuleManual, ProviderID: &legacy, ModelPattern: "gpt-6*", Op: domain.ParamOpChat,
			Actions: []domain.ParamAction{{Type: domain.ParamActRename, Field: "max_completion_tokens", To: "max_tokens"}}},
		builtinMaxCompletion(),
		{ID: 3, Name: "clamp", Enabled: true, Source: domain.ParamRuleManual, Actions: []domain.ParamAction{
			{Type: domain.ParamActClamp, Field: "temperature", Max: ptrF(1)},
			{Type: domain.ParamActDefault, Field: "generationConfig.topK", Value: json.RawMessage(`40`)},
			{Type: domain.ParamActSet, Field: "seed", Value: json.RawMessage(`42`)}}},
		{ID: 4, Name: "disabled", Enabled: false, Actions: []domain.ParamAction{{Type: domain.ParamActDrop, Field: "seed"}}},
	}}
	svc := NewParamRuleService(repo, nil, zap.NewNop())
	body := func() map[string]any {
		return map[string]any{"model": "gpt-6.1-sol", "max_tokens": 100, "temperature": 1.7}
	}

	upgraded := domain.CallTarget{Provider: domain.Provider{ID: 1, Protocol: domain.ProtocolOpenAI}, Model: domain.Model{ModelKey: "gpt-6.1-sol"}}
	b := body()
	svc.Rewrite(context.Background(), upgraded, domain.ParamOpChat, b)
	if _, ok := b["max_tokens"]; ok || b["max_completion_tokens"] != 100 || b["temperature"] != 1.0 || b["seed"] != json.Number("42") {
		t.Fatalf("upgraded supplier body = %v", b)
	}
	if gc, _ := b["generationConfig"].(map[string]any); gc["topK"] != json.Number("40") {
		t.Fatalf("nested default not applied: %v", b)
	}

	legacyT := upgraded
	legacyT.Provider.ID = legacy
	b = body()
	applied := svc.Rewrite(context.Background(), legacyT, domain.ParamOpChat, b)
	if b["max_tokens"] != 100 || b["max_completion_tokens"] != nil {
		t.Fatalf("legacy supplier body = %v (applied %v)", b, applied)
	}

	other := domain.CallTarget{Provider: domain.Provider{ID: 1, Protocol: domain.ProtocolOpenAI}, Model: domain.Model{ModelKey: "deepseek-v3"}}
	b = body()
	svc.Rewrite(context.Background(), other, domain.ParamOpChat, b)
	if b["max_tokens"] != 100 {
		t.Fatalf("non-matching model must keep max_tokens: %v", b)
	}
}

func ptrF(f float64) *float64 { return &f }

func TestComposeAutoCancelsReversal(t *testing.T) {
	a := domain.ParamAction{Type: domain.ParamActRename, Field: "max_tokens", To: "max_completion_tokens"}
	back := domain.ParamAction{Type: domain.ParamActRename, Field: "max_completion_tokens", To: "max_tokens"}
	drop := domain.ParamAction{Type: domain.ParamActDrop, Field: "temperature"}
	if got := composeAuto([]domain.ParamAction{a, drop}, []domain.ParamAction{back, drop}); len(got) != 1 || got[0].Type != drop.Type || got[0].Field != drop.Field {
		t.Fatalf("got %+v", got)
	}
}

// fakeSupplier answers like an OpenAI-compatible API that only accepts one
// of the two output-limit parameters, with that vendor's real error text.
func fakeSupplier(t *testing.T, accept string, calls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		reject := func(msg string) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg, "type": "invalid_request_error"}})
		}
		if _, ok := body["max_tokens"]; ok && accept == "max_completion_tokens" {
			reject("Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.")
			return
		}
		if _, ok := body["max_completion_tokens"]; ok && accept == "max_tokens" {
			reject("Unrecognized request argument supplied: max_completion_tokens")
			return
		}
		if _, ok := body[accept]; !ok {
			t.Errorf("%s supplier got neither limit: %s", accept, raw)
		}
		if body["seed"] != float64(7) {
			t.Errorf("pass-through parameter lost: %s", raw)
		}
		_, _ = w.Write([]byte(`{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`))
	}))
}

func TestAdapterLearnsEachSuppliersParameter(t *testing.T) {
	repo := &memParamRepo{rules: []*domain.ParamRule{builtinMaxCompletion()}}
	svc := NewParamRuleService(repo, nil, zap.NewNop())
	client := openaicompat.NewClient().WithParams(svc)

	var upCalls, legacyCalls, oldCalls atomic.Int32
	upgraded := fakeSupplier(t, "max_completion_tokens", &upCalls)  // already moved to the new name
	legacy := fakeSupplier(t, "max_tokens", &legacyCalls)           // has not upgraded yet
	oldModel := fakeSupplier(t, "max_completion_tokens", &oldCalls) // model outside the preset's pattern
	defer upgraded.Close()
	defer legacy.Close()
	defer oldModel.Close()

	var req domain.ChatRequest
	if err := json.Unmarshal([]byte(`{"model":"gpt-6.1-sol","max_completion_tokens":64,"seed":7,"tools":[{"type":"function"}],
		"messages":[{"role":"user","content":"hi"}]}`), &req); err != nil {
		t.Fatal(err)
	}
	if *req.MaxTokens != 64 || req.Extra["seed"] == nil || req.Extra["tools"] != nil {
		t.Fatalf("inbound normalisation: %+v", req)
	}
	call := func(srv *httptest.Server, pid int64, model string) {
		t.Helper()
		target := domain.CallTarget{Provider: domain.Provider{ID: pid, Name: srv.URL, BaseURL: srv.URL, Protocol: domain.ProtocolOpenAI},
			Model: domain.Model{ModelKey: model}}
		resp, err := client.ChatCompletion(context.Background(), target, req)
		if err != nil || resp.Choices[0].Message.Content != "OK" {
			t.Fatalf("supplier %d: %v", pid, err)
		}
	}

	call(upgraded, 1, "gpt-6.1-sol") // preset already sends max_completion_tokens: 1 call
	call(legacy, 2, "gpt-6.1-sol")   // rejected, adapted back to max_tokens, learned: 2 calls
	call(oldModel, 3, "gpt-4o")      // sent max_tokens, rejected, adapted forward: 2 calls
	for i := 0; i < 3; i++ {         // afterwards everyone is right first time
		call(upgraded, 1, "gpt-6.1-sol")
		call(legacy, 2, "gpt-6.1-sol")
		call(oldModel, 3, "gpt-4o")
	}
	if upCalls.Load() != 4 || legacyCalls.Load() != 5 || oldCalls.Load() != 5 {
		t.Fatalf("calls: upgraded %d legacy %d old-model %d", upCalls.Load(), legacyCalls.Load(), oldCalls.Load())
	}
	autos := 0
	for _, r := range repo.rules {
		if r.Source == domain.ParamRuleAuto {
			autos++
			if r.ProviderID == nil || (*r.ProviderID != 2 && *r.ProviderID != 3) || !strings.Contains(r.Remark, "供应商返回") {
				t.Fatalf("unexpected auto rule %+v", r)
			}
		}
	}
	if autos != 2 {
		t.Fatalf("want 2 learned rules, got %d", autos)
	}

	// the legacy supplier upgrades later: the learned rule is reversed, not stacked
	legacy.Config.Handler = fakeSupplier(t, "max_completion_tokens", &legacyCalls).Config.Handler
	call(legacy, 2, "gpt-6.1-sol")
	call(legacy, 2, "gpt-6.1-sol")
	r, _ := repo.FindAuto(context.Background(), 2, "gpt-6.1-sol", domain.ParamOpChat)
	if r != nil {
		t.Fatalf("reversed fix should cancel the auto rule, got %+v", r.Actions)
	}
}
