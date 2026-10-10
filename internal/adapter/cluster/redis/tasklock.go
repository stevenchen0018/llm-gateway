// Package redis implements cluster coordination on the shared Redis: a
// lease lock so a scheduled job runs on exactly one gateway node per period
// even when several nodes (or several masters) are deployed.
package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "llmgw:task:"

// TaskLock grants per-task leases with SET NX PX. A lease is never released
// early: it simply expires, which also keeps a node that just ran the job
// from being followed by another node in the same period.
type TaskLock struct {
	rdb  *redis.Client
	node string
}

func NewTaskLock(rdb *redis.Client, node string) *TaskLock {
	return &TaskLock{rdb: rdb, node: node}
}

// TryAcquire reports whether this node won the lease for task. Redis errors
// count as "not acquired": skipping one run is safer than running it twice.
func (l *TaskLock) TryAcquire(ctx context.Context, task string, ttl time.Duration) bool {
	ok, err := l.rdb.SetNX(ctx, keyPrefix+task, l.node, ttl).Result()
	return err == nil && ok
}
