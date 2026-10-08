package domain

import "context"

// RateLimitScope identifies the (key, model) pair a limit check applies to
// and carries the effective limits for that pair — the article's "Key和模型
// 粒度的TPM容量管理体系". The service layer computes the *effective* limit
// (min of the key's quota and the model's own low-throughput ceiling)
// before calling the limiter, so the limiter itself stays a dumb, fast
// Redis primitive.
type RateLimitScope struct {
	KeyID    int64
	ModelID  int64
	QPSLimit int // requests per second; 0 = unlimited
	TPMLimit int // tokens per minute; 0 = unlimited
	// DepartmentID, when non-zero, makes this a department-wide scope shared
	// by every key of the department (KeyID/ModelID are then ignored).
	DepartmentID int64
}

// RateLimiter enforces QPS and TPM quotas. TPM is enforced with a
// reserve/settle protocol because the true token cost of a call is only
// known after the upstream response arrives:
//  1. AllowQPS is checked first (cheap, request-count based).
//  2. ReserveTPM pre-deducts an *estimated* token cost (see
//     internal/pkg/tokencount) so concurrent requests can't all pass a stale
//     window check ("Token级别的精准限流").
//  3. After the call completes, SettleTPM reconciles the reservation against
//     the real usage.Total from the provider response (refunding the
//     difference if the estimate was high, or over-drawing it if low).
//  4. If the call fails before completion, ReleaseTPM fully refunds the
//     reservation.
type RateLimiter interface {
	AllowQPS(ctx context.Context, scope RateLimitScope) (allowed bool, err error)
	ReserveTPM(ctx context.Context, scope RateLimitScope, estimatedTokens int) (reservationID string, allowed bool, err error)
	SettleTPM(ctx context.Context, reservationID string, actualTokens int) error
	ReleaseTPM(ctx context.Context, reservationID string) error
}
