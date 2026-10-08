// Package redis implements domain.RateLimiter on top of Redis, enforcing
// the article's "Key和模型粒度的TPM、QPS流控" with atomic Lua scripts so
// concurrent requests can never race past a limit.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// qpsScript implements a fixed 1-second counter window keyed per second:
// KEYS[1] = qps key, ARGV[1] = limit. Returns 1 if allowed, 0 if rejected.
const qpsScript = `
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIRE", KEYS[1], 2)
end
local limit = tonumber(ARGV[1])
if limit <= 0 then
  return 1
end
if current > limit then
  return 0
end
return 1
`

// reserveTPMScript pre-deducts estimatedTokens from a per-minute counter.
// KEYS[1] = tpm key (bucketed per minute), ARGV[1] = limit, ARGV[2] =
// estimated tokens. Returns 1 if the reservation fits, 0 otherwise (in
// which case nothing is deducted).
const reserveTPMScript = `
local limit = tonumber(ARGV[1])
local want = tonumber(ARGV[2])
if limit <= 0 then
  redis.call("INCRBY", KEYS[1], want)
  redis.call("EXPIRE", KEYS[1], 120)
  return 1
end
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
if current + want > limit then
  return 0
end
redis.call("INCRBY", KEYS[1], want)
redis.call("EXPIRE", KEYS[1], 120)
return 1
`

// settleTPMScript adjusts a previously reserved amount to the real amount:
// KEYS[1] = tpm key, ARGV[1] = reserved, ARGV[2] = actual. delta = actual -
// reserved is added (can be negative, i.e. a refund).
const settleTPMScript = `
local delta = tonumber(ARGV[2]) - tonumber(ARGV[1])
if delta ~= 0 then
  redis.call("INCRBY", KEYS[1], delta)
end
return 1
`

type reservation struct {
	key       string
	estimated int
}

type Limiter struct {
	client *redis.Client

	qps     *redis.Script
	reserve *redis.Script
	settle  *redis.Script

	// reservations tracks in-flight TPM reservations by opaque ID so
	// SettleTPM/ReleaseTPM know which minute-bucket key and how much was
	// originally reserved. A crashed process simply lets the reservation
	// expire with its bucket's TTL (120s) — it does not leak forever.
	reservations *reservationStore
}

func NewLimiter(client *redis.Client) *Limiter {
	return &Limiter{
		client:       client,
		qps:          redis.NewScript(qpsScript),
		reserve:      redis.NewScript(reserveTPMScript),
		settle:       redis.NewScript(settleTPMScript),
		reservations: newReservationStore(),
	}
}

func (l *Limiter) AllowQPS(ctx context.Context, scope domain.RateLimitScope) (bool, error) {
	key := fmt.Sprintf("rl:qps:%d:%d:%d", scope.KeyID, scope.ModelID, time.Now().Unix())
	if scope.DepartmentID != 0 {
		key = fmt.Sprintf("rl:qps:dept:%d:%d", scope.DepartmentID, time.Now().Unix())
	}
	res, err := l.qps.Run(ctx, l.client, []string{key}, scope.QPSLimit).Int()
	if err != nil {
		return false, fmt.Errorf("qps script: %w", err)
	}
	return res == 1, nil
}

func (l *Limiter) ReserveTPM(ctx context.Context, scope domain.RateLimitScope, estimatedTokens int) (string, bool, error) {
	bucket := time.Now().Format("200601021504") // per-minute bucket
	key := fmt.Sprintf("rl:tpm:%d:%d:%s", scope.KeyID, scope.ModelID, bucket)
	if scope.DepartmentID != 0 {
		key = fmt.Sprintf("rl:tpm:dept:%d:%s", scope.DepartmentID, bucket)
	}

	res, err := l.reserve.Run(ctx, l.client, []string{key}, scope.TPMLimit, estimatedTokens).Int()
	if err != nil {
		return "", false, fmt.Errorf("reserve tpm script: %w", err)
	}
	if res != 1 {
		return "", false, nil
	}

	id := uuid.NewString()
	l.reservations.put(id, reservation{key: key, estimated: estimatedTokens})
	return id, true, nil
}

func (l *Limiter) SettleTPM(ctx context.Context, reservationID string, actualTokens int) error {
	r, ok := l.reservations.take(reservationID)
	if !ok {
		return nil // already settled/released, or expired reservation — no-op
	}
	if err := l.settle.Run(ctx, l.client, []string{r.key}, r.estimated, actualTokens).Err(); err != nil {
		return fmt.Errorf("settle tpm script: %w", err)
	}
	return nil
}

func (l *Limiter) ReleaseTPM(ctx context.Context, reservationID string) error {
	r, ok := l.reservations.take(reservationID)
	if !ok {
		return nil
	}
	// Releasing is settling to zero actual usage: full refund.
	if err := l.settle.Run(ctx, l.client, []string{r.key}, r.estimated, 0).Err(); err != nil {
		return fmt.Errorf("release tpm script: %w", err)
	}
	return nil
}
