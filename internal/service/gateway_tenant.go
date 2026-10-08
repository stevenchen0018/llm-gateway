package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/metrics"
)

// DepartmentCache serves department quotas to the request path without a
// database round trip per call. Entries expire after ttl, and admin edits
// invalidate them immediately.
type DepartmentCache struct {
	repo domain.DepartmentRepository
	ttl  time.Duration

	mu      sync.Mutex
	entries map[int64]deptEntry
}

type deptEntry struct {
	d   *domain.Department
	exp time.Time
}

func NewDepartmentCache(repo domain.DepartmentRepository, ttl time.Duration) *DepartmentCache {
	return &DepartmentCache{repo: repo, ttl: ttl, entries: map[int64]deptEntry{}}
}

func (c *DepartmentCache) Get(ctx context.Context, id int64) (*domain.Department, error) {
	c.mu.Lock()
	e, ok := c.entries[id]
	c.mu.Unlock()
	if ok && time.Now().Before(e.exp) {
		return e.d, nil
	}
	d, err := c.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries[id] = deptEntry{d: d, exp: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return d, nil
}

func (c *DepartmentCache) Invalidate(id int64) {
	c.mu.Lock()
	delete(c.entries, id)
	c.mu.Unlock()
}

// WithDepartments enables department-level (tenant) admission control.
func (g *GatewayService) WithDepartments(c *DepartmentCache) *GatewayService {
	g.depts = c
	return g
}

// admit applies one rate-limit scope (QPS check + TPM reservation) for a
// whole request. It returns a settle function that must be called exactly once
// with the call's real token usage (0 on failure, which refunds the whole
// reservation). label names the scope in metrics; describe builds the alert text.
func (g *GatewayService) admit(ctx context.Context, scope domain.RateLimitScope, estTokens int, label string,
	alertRef int64, keyID int64, describe func(dim string, limit int) string) (func(actual int), error) {
	noop := func(int) {}
	if scope.QPSLimit == 0 && scope.TPMLimit == 0 {
		return noop, nil
	}
	if scope.QPSLimit > 0 {
		ok, err := g.limiter.AllowQPS(ctx, scope)
		if err != nil {
			return nil, fmt.Errorf("rate limiter unavailable: %w", err)
		}
		if !ok {
			metrics.RateLimitRejectedTotal.WithLabelValues(label + "_qps").Inc()
			g.alerts.Raise(ctx, domain.AlertTypeQuota, alertRef, &keyID, describe("QPS", scope.QPSLimit), domain.AlertLevelWarning)
			return nil, domain.ErrRateLimited
		}
	}
	if scope.TPMLimit == 0 {
		return noop, nil
	}
	resID, ok, err := g.limiter.ReserveTPM(ctx, scope, estTokens)
	if err != nil {
		return nil, fmt.Errorf("rate limiter unavailable: %w", err)
	}
	if !ok {
		metrics.RateLimitRejectedTotal.WithLabelValues(label + "_tpm").Inc()
		g.alerts.Raise(ctx, domain.AlertTypeQuota, alertRef, &keyID, describe("TPM", scope.TPMLimit), domain.AlertLevelWarning)
		return nil, domain.ErrRateLimited
	}
	return func(actual int) {
		// the request context may already be cancelled; settling must still happen
		c := context.WithoutCancel(ctx)
		if actual > 0 {
			_ = g.limiter.SettleTPM(c, resID, actual)
		} else {
			_ = g.limiter.ReleaseTPM(c, resID)
		}
	}, nil
}

// enterKeyAndDepartment applies the request-level quotas, in order:
//  1. the API key's own QPS/TPM quota — across every model it calls, so a
//     request that fails over between candidates is still one request;
//  2. the key's department-wide (tenant) QPS/TPM ceiling shared by all its keys.
//
// Model-level vendor capacity is checked separately per candidate, so a
// saturated model makes the gateway fail over instead of rejecting.
func (g *GatewayService) enterKeyAndDepartment(ctx context.Context, key domain.APIKey, estTokens int) (func(actual int), error) {
	settleKey, err := g.admit(ctx, domain.RateLimitScope{KeyID: key.ID, QPSLimit: key.QPSQuota, TPMLimit: key.TPMQuota},
		estTokens, "key", key.ID, key.ID, func(dim string, limit int) string {
			return fmt.Sprintf("Key %s %s 超过配额 %d", key.Name, dim, limit)
		})
	if err != nil {
		return nil, err
	}
	settleDept, err := g.enterDepartment(ctx, key, estTokens)
	if err != nil {
		settleKey(0)
		return nil, err
	}
	return func(actual int) { settleDept(actual); settleKey(actual) }, nil
}

// enterDepartment applies the calling key's department-wide QPS/TPM ceiling.
func (g *GatewayService) enterDepartment(ctx context.Context, key domain.APIKey, estTokens int) (func(actual int), error) {
	noop := func(int) {}
	if g.depts == nil || key.DepartmentID == nil {
		return noop, nil
	}
	d, err := g.depts.Get(ctx, *key.DepartmentID)
	if errors.Is(err, domain.ErrNotFound) {
		return noop, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load department: %w", err)
	}
	if d.Status != "active" {
		return nil, fmt.Errorf("%w: department %q is disabled", domain.ErrForbidden, d.Name)
	}
	return g.admit(ctx, domain.RateLimitScope{DepartmentID: d.ID, QPSLimit: d.QPSQuota, TPMLimit: d.TPMQuota},
		estTokens, "dept", d.ID, key.ID, func(dim string, limit int) string {
			return fmt.Sprintf("部门 %s %s 超过上限 %d（key=%s）", d.Name, dim, limit, key.Name)
		})
}
