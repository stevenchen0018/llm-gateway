package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/mock"
	"github.com/stevenchen/llm-gateway/internal/domain"
)

type fakeFilterRepo struct{ rules []*domain.ContentFilterRule }

func (f *fakeFilterRepo) Create(_ context.Context, r *domain.ContentFilterRule) error {
	r.ID = int64(len(f.rules) + 1)
	f.rules = append(f.rules, r)
	return nil
}
func (f *fakeFilterRepo) Update(context.Context, *domain.ContentFilterRule) error { return nil }
func (f *fakeFilterRepo) Delete(context.Context, int64) error                     { return nil }
func (f *fakeFilterRepo) Get(context.Context, int64) (*domain.ContentFilterRule, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeFilterRepo) List(context.Context) ([]*domain.ContentFilterRule, error) {
	return f.rules, nil
}
func (f *fakeFilterRepo) AddHits(context.Context, map[int64]int64, time.Time) error { return nil }

type fakeSettingsRepo struct{ st *domain.GatewaySettings }

func (f *fakeSettingsRepo) Load(_ context.Context, _ string, out any) (bool, error) {
	if f.st == nil {
		return false, nil
	}
	*(out.(*domain.GatewaySettings)) = *f.st
	return true, nil
}
func (f *fakeSettingsRepo) Save(_ context.Context, _ string, v any, _ string) error {
	st := v.(domain.GatewaySettings)
	f.st = &st
	return nil
}

func newTestFilter(t *testing.T, enabled, output bool, rules ...*domain.ContentFilterRule) *ContentFilterService {
	t.Helper()
	st := domain.DefaultGatewaySettings()
	st.ContentFilter.Enabled, st.ContentFilter.CheckOutput = enabled, output
	settings := NewSettingsService(&fakeSettingsRepo{st: &st}, time.Minute)
	repo := &fakeFilterRepo{}
	for i, r := range rules {
		r.ID, r.Enabled = int64(i+1), true
		if r.Stage == "" {
			r.Stage = domain.FilterStageInput
		}
		repo.rules = append(repo.rules, r)
	}
	return NewContentFilterService(repo, settings, NewAlertService(fakeAlertRepo{}, fakeNotifier{}, zap.NewNop()), zap.NewNop())
}

var (
	phoneRule  = &domain.ContentFilterRule{Name: "手机号", MatchType: "regex", Pattern: `1[3-9]\d{9}`, Action: domain.FilterActionMask, Replacement: "[手机号]"}
	secretRule = &domain.ContentFilterRule{Name: "机密词", MatchType: "keyword", Pattern: "绝密\nTOP SECRET", Action: domain.FilterActionBlock}
)

func TestFilterMaskBlockAndLog(t *testing.T) {
	logRule := &domain.ContentFilterRule{Name: "竞品", MatchType: "keyword", Pattern: "友商,竞品", Action: domain.FilterActionLog}
	f := newTestFilter(t, true, false, phoneRule, secretRule, logRule)
	key := domain.APIKey{ID: 1, Name: "k"}

	text := "我的电话 13812345678，另一个 15900001111，对比一下竞品"
	res, err := f.Check(context.Background(), key, domain.FilterStageInput, []*string{&text})
	if err != nil {
		t.Fatal(err)
	}
	if text != "我的电话 [手机号]，另一个 [手机号]，对比一下竞品" {
		t.Fatalf("masked text = %q", text)
	}
	if len(res.Hits) != 2 || res.Hits[0].Matches != 2 || res.Hits[0].Sample != "" || res.Hits[1].Sample != "竞品" {
		t.Fatalf("hits = %+v (mask hits must not keep the sample)", res.Hits)
	}

	blocked := "this is top secret material" // keywords are case-insensitive
	_, err = f.Check(context.Background(), key, domain.FilterStageInput, []*string{&blocked})
	if !errors.Is(err, domain.ErrContentBlocked) {
		t.Fatalf("expected ErrContentBlocked, got %v", err)
	}
}

func TestFilterMasterSwitchAndStages(t *testing.T) {
	off := newTestFilter(t, false, false, secretRule)
	text := "绝密"
	if _, err := off.Check(context.Background(), domain.APIKey{}, domain.FilterStageInput, []*string{&text}); err != nil {
		t.Fatalf("filter must be inert while switched off, got %v", err)
	}

	outRule := &domain.ContentFilterRule{Name: "out", MatchType: "keyword", Pattern: "绝密", Action: domain.FilterActionBlock, Stage: domain.FilterStageOutput}
	on := newTestFilter(t, true, false, outRule)
	if _, err := on.Check(context.Background(), domain.APIKey{}, domain.FilterStageInput, []*string{&text}); err != nil {
		t.Fatalf("output-only rule must not fire on input, got %v", err)
	}
	if _, err := on.Check(context.Background(), domain.APIKey{}, domain.FilterStageOutput, []*string{&text}); err != nil {
		t.Fatalf("output checking is off, got %v", err)
	}
	withOutput := newTestFilter(t, true, true, outRule)
	if _, err := withOutput.Check(context.Background(), domain.APIKey{}, domain.FilterStageOutput, []*string{&text}); !errors.Is(err, domain.ErrContentBlocked) {
		t.Fatalf("expected output block, got %v", err)
	}
}

func TestFilterDepartmentRulesOnlyApplyToTheirDepartment(t *testing.T) {
	dept := int64(7)
	rule := &domain.ContentFilterRule{Name: "d7", MatchType: "keyword", Pattern: "内部代号", Action: domain.FilterActionBlock, DepartmentID: &dept}
	f := newTestFilter(t, true, false, rule)
	text := "内部代号"
	other, mine := int64(8), dept
	if _, err := f.Check(context.Background(), domain.APIKey{DepartmentID: &other}, domain.FilterStageInput, []*string{&text}); err != nil {
		t.Fatalf("rule of dept 7 fired for dept 8: %v", err)
	}
	if _, err := f.Check(context.Background(), domain.APIKey{DepartmentID: &mine}, domain.FilterStageInput, []*string{&text}); !errors.Is(err, domain.ErrContentBlocked) {
		t.Fatalf("expected block for own department, got %v", err)
	}
}

func TestFilterRuleValidation(t *testing.T) {
	bad := []*domain.ContentFilterRule{
		{Name: "empty-match", MatchType: "regex", Pattern: `a*`, Action: domain.FilterActionLog},
		{Name: "syntax", MatchType: "regex", Pattern: `(unclosed`, Action: domain.FilterActionLog},
		{Name: "no-keywords", MatchType: "keyword", Pattern: " ,\n ", Action: domain.FilterActionLog},
		{Name: "action", MatchType: "keyword", Pattern: "x", Action: "drop"},
		{Name: "", MatchType: "keyword", Pattern: "x", Action: domain.FilterActionLog},
	}
	for _, r := range bad {
		if err := normalizeRule(r); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Errorf("rule %q: expected ErrInvalidArgument, got %v", r.Name, err)
		}
	}
}

// Masking happens before the vendor sees the prompt: the mock echoes the
// prompt back, so the echoed reply must contain the placeholder only.
func TestGatewayMasksPromptBeforeUpstreamAndBlocks(t *testing.T) {
	gw, usage := newTestGateway(t, &fakeRateLimiter{}, &fakeHealthStore{unhealthy: map[int64]bool{}}, mock.NewClient())
	gw.WithContentFilter(newTestFilter(t, true, false, phoneRule, secretRule))
	key := domain.APIKey{ID: 1, Status: domain.APIKeyStatusActive}

	trace := &domain.RequestTrace{RequestID: "req-test-1"}
	ctx := domain.WithTrace(context.Background(), trace)
	resp, err := gw.ChatCompletion(ctx, key, domain.ChatRequest{Model: "business-alias",
		Messages: []domain.ChatMessage{{Role: "user", Content: "call 13812345678"}}}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Choices[0].Message.Content; strings.Contains(got, "13812345678") || !strings.Contains(got, "[手机号]") {
		t.Fatalf("vendor saw the raw phone number: %q", got)
	}
	if resp.ID != "req-test-1" || len(trace.Hits) != 1 || trace.ModelID == nil || trace.Usage.TotalTokens == 0 {
		t.Fatalf("trace not filled: id=%s %+v", resp.ID, trace)
	}

	before := usage.len()
	_, err = gw.ChatCompletion(context.Background(), key, domain.ChatRequest{Model: "business-alias",
		Messages: []domain.ChatMessage{{Role: "user", Content: "绝密文件"}}}, "127.0.0.1")
	if !errors.Is(err, domain.ErrContentBlocked) {
		t.Fatalf("expected ErrContentBlocked, got %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if usage.len() != before+1 { // only the first (successful) call was billed
		t.Fatalf("a blocked request must never reach a vendor or be billed")
	}
}

func TestGatewayWithholdsBlockedOutput(t *testing.T) {
	gw, _ := newTestGateway(t, &fakeRateLimiter{}, &fakeHealthStore{unhealthy: map[int64]bool{}}, mock.NewClient())
	outRule := &domain.ContentFilterRule{Name: "echo", MatchType: "keyword", Pattern: "echo:", Action: domain.FilterActionBlock, Stage: domain.FilterStageOutput}
	gw.WithContentFilter(newTestFilter(t, true, true, outRule))
	resp, err := gw.ChatCompletion(context.Background(), domain.APIKey{ID: 1}, domain.ChatRequest{Model: "business-alias",
		Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if c := resp.Choices[0]; c.FinishReason != "content_filter" || strings.Contains(c.Message.Content, "echo:") {
		t.Fatalf("blocked output leaked: %+v", c)
	}
}
