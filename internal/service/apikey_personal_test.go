package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type memKeyRepo struct {
	mu   sync.Mutex
	keys map[int64]*domain.APIKey
	next int64
}

func (r *memKeyRepo) Create(_ context.Context, k *domain.APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	k.ID = r.next
	if k.KeyPrefix == "" {
		k.KeyPrefix = "pd-x"
	}
	c := *k
	r.keys[k.ID] = &c
	return nil
}
func (r *memKeyRepo) Update(_ context.Context, k *domain.APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *k
	r.keys[k.ID] = &c
	return nil
}
func (r *memKeyRepo) Get(_ context.Context, id int64) (*domain.APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.keys[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c := *k
	return &c, nil
}
func (r *memKeyRepo) GetByPrefix(_ context.Context, p string) (*domain.APIKey, error) {
	for _, k := range r.keys {
		if k.KeyPrefix == p {
			c := *k
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *memKeyRepo) List(context.Context) ([]*domain.APIKey, error)           { return nil, nil }
func (r *memKeyRepo) SetDepartmentForApp(context.Context, int64, *int64) error { return nil }
func (r *memKeyRepo) Search(_ context.Context, q domain.KeySearch) ([]*domain.APIKey, int64, error) {
	var out []*domain.APIKey
	for _, k := range r.keys {
		okStatus := len(q.Statuses) == 0
		for _, s := range q.Statuses {
			okStatus = okStatus || k.Status == s
		}
		if okStatus && (q.Category == "" || k.Category == q.Category) &&
			(q.Q == "" || strings.Contains(strings.ToLower(k.OwnerEmail+k.Name), strings.ToLower(q.Q))) {
			c := *k
			out = append(out, &c)
		}
	}
	return out, int64(len(out)), nil
}

type memAuditRepo struct{}

func (memAuditRepo) Create(context.Context, *domain.AuditLog) error               { return nil }
func (memAuditRepo) ListByKey(context.Context, int64) ([]*domain.AuditLog, error) { return nil, nil }
func (memAuditRepo) List(context.Context, int) ([]*domain.AuditLog, error)        { return nil, nil }
func (memAuditRepo) Page(context.Context, domain.AuditQuery) ([]*domain.AuditLog, int64, error) {
	return nil, 0, nil
}

func personalKey(email string, holder *int64) *domain.APIKey {
	dept := int64(3)
	return &domain.APIKey{Name: "dev", Owner: "张伟", OwnerEmail: email, Category: domain.KeyCategoryPersonal,
		DepartmentID: &dept, HolderUserID: holder, AllowedModels: []string{" deepseek-v3 ", "DEEPSEEK-V3", "qwen3-coder"}}
}

func TestPersonalKeyLifecycle(t *testing.T) {
	repo := &memKeyRepo{keys: map[int64]*domain.APIKey{}}
	svc := NewAPIKeyService(repo, memAuditRepo{}, nil)
	ctx := context.Background()
	holder := int64(42)

	k := personalKey("zhang@example.com", &holder)
	if err := svc.Apply(ctx, k, "zhang"); err != nil {
		t.Fatal(err)
	}
	if k.TPMQuota != personalTPMDefault || k.QPSQuota != personalQPSDefault || len(k.AllowedModels) != 2 {
		t.Fatalf("personal defaults / model de-dup not applied: %+v", k)
	}
	// one open personal key per employee
	if err := svc.Apply(ctx, personalKey("ZHANG@example.com", &holder), "zhang"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("second personal key for the same employee must be refused, got %v", err)
	}
	// a personal key must not belong to an application
	bad := personalKey("li@example.com", nil)
	app := int64(1)
	bad.AppID = &app
	if err := svc.Apply(ctx, bad, "x"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("personal key with app must be refused, got %v", err)
	}

	// the approver (not the holder) never sees the secret
	secret, claim, err := svc.Approve(ctx, k.ID, "cx_admin", 7)
	if err != nil || secret != "" || !claim {
		t.Fatalf("approve of a held key: secret=%q claim=%v err=%v", secret, claim, err)
	}
	if _, err := svc.Claim(ctx, k.ID, 7, "cx_admin"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("only the holder may claim, got %v", err)
	}
	secret, err = svc.Claim(ctx, k.ID, holder, "zhang")
	if err != nil || !strings.HasPrefix(secret, "sk-") {
		t.Fatalf("holder claim: %q %v", secret, err)
	}
	got, _ := svc.Authenticate(ctx, secret)
	if got == nil || got.ID != k.ID {
		t.Fatal("claimed secret must authenticate")
	}
	if _, err := svc.Claim(ctx, k.ID, holder, "zhang"); err == nil {
		t.Fatal("a secret can be claimed only once")
	}

	// an admin rotating a held key revokes it; the holder claims a new one
	_, claim, err = svc.Rotate(ctx, k.ID, 7, "cx_admin")
	if err != nil || !claim {
		t.Fatalf("admin rotate should revoke and require a claim: %v %v", claim, err)
	}
	if _, err := svc.Authenticate(ctx, secret); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("the old secret must stop working after rotation")
	}
	if s2, err := svc.Claim(ctx, k.ID, holder, "zhang"); err != nil || s2 == secret {
		t.Fatalf("re-claim after revoke: %v", err)
	}
}

func TestAllowsModel(t *testing.T) {
	k := domain.APIKey{AllowedModels: []string{"deepseek-v3", "qwen3-coder"}}
	for name, want := range map[string]bool{
		"deepseek-v3": true, "DeepSeek-V3": true, "aliyun/deepseek-v3": true, "qwen3-coder": true, "gpt-4o": false, "": false,
	} {
		if got := k.AllowsModel(name); got != want {
			t.Errorf("AllowsModel(%q)=%v want %v", name, got, want)
		}
	}
	if open := (domain.APIKey{}); !open.AllowsModel("anything") {
		t.Error("an empty allowlist allows every model")
	}
}

func TestGatewayRejectsModelOutsideAllowlist(t *testing.T) {
	gw, _ := newTestGateway(t, &fakeRateLimiter{}, &fakeHealthStore{unhealthy: map[int64]bool{}}, nil)
	key := domain.APIKey{ID: 1, Status: domain.APIKeyStatusActive, AllowedModels: []string{"qwen3-coder"}}
	_, err := gw.ChatCompletion(context.Background(), key, domain.ChatRequest{Model: "business-alias",
		Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}}, "127.0.0.1")
	if !errors.Is(err, domain.ErrModelNotAllowed) {
		t.Fatalf("expected ErrModelNotAllowed, got %v", err)
	}
}
