// Package redis implements domain.HealthStore, giving the routing engine
// the "分钟级容灾" (minute-level failover) described in the article: a
// routing candidate that trips its rolling error-rate threshold is marked
// unhealthy and skipped for a cool-down period, then automatically
// reconsidered.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// errorRateThreshold: mark unhealthy once failures reach this share of
	// samples within the rolling window.
	errorRateThreshold = 0.5
	// minSamples avoids tripping the breaker on a single unlucky request.
	minSamples = 5
	// cooldown: how long a candidate stays unhealthy before being retried.
	cooldown = 60 * time.Second
	// windowBuckets: number of trailing 1-minute buckets considered.
	windowBuckets = 2
)

// record atomically increments total/fail counters for the current minute
// bucket and, if the rolling window now exceeds the error-rate threshold,
// sets the cooldown marker — all in one round trip.
const recordLua = `
local totalKey = KEYS[1]
local failKey = KEYS[2]
local downKey = KEYS[3]
local success = ARGV[1]
local minSamples = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])
local cooldownSec = tonumber(ARGV[4])

redis.call("INCR", totalKey)
redis.call("EXPIRE", totalKey, 180)
if success == "0" then
  redis.call("INCR", failKey)
  redis.call("EXPIRE", failKey, 180)
end

local total = tonumber(redis.call("GET", totalKey) or "0")
local fail = tonumber(redis.call("GET", failKey) or "0")

if total >= minSamples and (fail / total) >= threshold then
  redis.call("SET", downKey, "1", "EX", cooldownSec)
end
return 1
`

type HealthStore struct {
	client *redis.Client
	record *redis.Script
}

func NewHealthStore(client *redis.Client) *HealthStore {
	return &HealthStore{client: client, record: redis.NewScript(recordLua)}
}

func (h *HealthStore) RecordResult(ctx context.Context, modelID int64, success bool) error {
	bucket := time.Now().Format("200601021504")
	totalKey := fmt.Sprintf("hb:total:%d:%s", modelID, bucket)
	failKey := fmt.Sprintf("hb:fail:%d:%s", modelID, bucket)
	downKey := fmt.Sprintf("hb:down:%d", modelID)

	successArg := "1"
	if !success {
		successArg = "0"
	}

	return h.record.Run(ctx, h.client,
		[]string{totalKey, failKey, downKey},
		successArg, minSamples, errorRateThreshold, int(cooldown.Seconds()),
	).Err()
}

func (h *HealthStore) IsHealthy(ctx context.Context, modelID int64) (bool, error) {
	downKey := fmt.Sprintf("hb:down:%d", modelID)
	n, err := h.client.Exists(ctx, downKey).Result()
	if err != nil {
		return false, fmt.Errorf("check health: %w", err)
	}
	return n == 0, nil
}
