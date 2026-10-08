package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stevenchen/llm-gateway/internal/adapter/provider/mock"
	"github.com/stevenchen/llm-gateway/internal/domain"
)

// deptLimiter records department-scope calls and can deny them.
type deptLimiter struct {
	fakeRateLimiter
	denyDeptQPS bool
	reserved    int
	settled     []int
	released    int
}

func (l *deptLimiter) AllowQPS(ctx context.Context, s domain.RateLimitScope) (bool, error) {
	if s.DepartmentID != 0 {
		return !l.denyDeptQPS, nil
	}
	return true, nil
}
func (l *deptLimiter) ReserveTPM(ctx context.Context, s domain.RateLimitScope, est int) (string, bool, error) {
	if s.DepartmentID != 0 {
		l.reserved = est
		return "dept", true, nil
	}
	return "key", true, nil
}
func (l *deptLimiter) SettleTPM(ctx context.Context, id string, actual int) error {
	if id == "dept" {
		l.settled = append(l.settled, actual)
	}
	return nil
}
func (l *deptLimiter) ReleaseTPM(ctx context.Context, id string) error {
	if id == "dept" {
		l.released++
	}
	return nil
}

func TestDepartmentQuotaGate(t *testing.T) {
	depts := &memDepts{m: map[int64]*domain.Department{
		1: {ID: 1, Name: "客户体验部", Status: "active", QPSQuota: 10, TPMQuota: 100000},
		2: {ID: 2, Name: "停用部门", Status: "disabled"},
	}}
	req := domain.ChatRequest{Model: "business-alias", Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}}
	ctx := context.Background()

	lim := &deptLimiter{}
	gw, _ := newTestGateway(t, lim, &fakeHealthStore{unhealthy: map[int64]bool{}}, mock.NewClient())
	gw.WithDepartments(NewDepartmentCache(depts, time.Minute))

	key := domain.APIKey{ID: 1, Status: domain.APIKeyStatusActive, DepartmentID: i64p(1)}
	resp, err := gw.ChatCompletion(ctx, key, req, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if lim.reserved == 0 || len(lim.settled) != 1 || lim.settled[0] != resp.Usage.TotalTokens {
		t.Fatalf("department TPM must be reserved then settled with real usage: reserved=%d settled=%v usage=%d", lim.reserved, lim.settled, resp.Usage.TotalTokens)
	}

	// department QPS exhausted → 429 without touching providers
	lim.denyDeptQPS = true
	if _, err := gw.ChatCompletion(ctx, key, req, "127.0.0.1"); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited from department QPS, got %v", err)
	}
	lim.denyDeptQPS = false

	// failure path refunds the reservation
	failing := &failingModelClient{inner: mock.NewClient(), failModelID: -1}
	gw2, _ := newTestGateway(t, lim, &fakeHealthStore{unhealthy: map[int64]bool{10: true, 11: true}}, failing)
	gw2.WithDepartments(NewDepartmentCache(depts, time.Minute))
	_, _ = gw2.ChatCompletion(ctx, key, req, "127.0.0.1")
	if lim.released != 1 {
		t.Fatalf("a failed call must release the department reservation, released=%d", lim.released)
	}

	// disabled department → forbidden
	off := domain.APIKey{ID: 2, Status: domain.APIKeyStatusActive, DepartmentID: i64p(2)}
	if _, err := gw.ChatCompletion(ctx, off, req, "127.0.0.1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for disabled department, got %v", err)
	}
}

func TestAlertDedupe(t *testing.T) {
	s := &AlertService{lastSent: map[string]time.Time{}}
	k := int64(7)
	if s.suppressed(domain.AlertTypeQuota, 1, &k, domain.AlertLevelWarning) {
		t.Fatal("first alert must go through")
	}
	if !s.suppressed(domain.AlertTypeQuota, 1, &k, domain.AlertLevelWarning) {
		t.Fatal("repeat within the window must be suppressed")
	}
	if s.suppressed(domain.AlertTypeReport, 1, &k, domain.AlertLevelInfo) || s.suppressed(domain.AlertTypeReport, 1, &k, domain.AlertLevelInfo) {
		t.Fatal("reports are never deduplicated")
	}
}

// scopeRecorder records every scope the gateway checks.
type scopeRecorder struct {
	fakeRateLimiter
	scopes []domain.RateLimitScope
}

func (l *scopeRecorder) AllowQPS(ctx context.Context, s domain.RateLimitScope) (bool, error) {
	l.scopes = append(l.scopes, s)
	return true, nil
}

// A key's quota must be checked once per request across all models, and the
// model's capacity per candidate independently of the key — otherwise a key
// that fails over between N candidates gets N× its quota (found by loadtest).
func TestKeyAndModelScopesAreIndependent(t *testing.T) {
	lim := &scopeRecorder{}
	gw, _ := newTestGateway(t, lim, &fakeHealthStore{unhealthy: map[int64]bool{}}, &failingModelClient{inner: mock.NewClient(), failModelID: 10})
	// give the candidate models a capacity so model scopes are exercised
	key := domain.APIKey{ID: 7, Status: domain.APIKeyStatusActive, QPSQuota: 2, TPMQuota: 1000}
	req := domain.ChatRequest{Model: "business-alias", Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}}
	if _, err := gw.ChatCompletion(context.Background(), key, req, ""); err != nil {
		t.Fatal(err)
	}
	var keyScopes int
	for _, s := range lim.scopes {
		if s.KeyID == 7 && s.ModelID != 0 {
			t.Fatalf("key quota must not be keyed per model: %+v", s)
		}
		if s.KeyID == 7 {
			keyScopes++
			if s.QPSLimit != 2 {
				t.Fatalf("key scope carries the key's own quota, got %+v", s)
			}
		}
		if s.ModelID != 0 && s.KeyID != 0 {
			t.Fatalf("model capacity must be shared across keys: %+v", s)
		}
	}
	if keyScopes != 1 {
		t.Fatalf("key quota checked %d times for one request (failover happened), want 1", keyScopes)
	}
}
