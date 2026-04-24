// Package redisbackend provides a Redis backend for YARL using go-redis/v9.
//
// Supports Redis standalone and Redis Sentinel via [redis.UniversalClient].
// Requires Redis >= 7.0 (uses EXPIRE NX).
package redisbackend

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	yarl "github.com/logocomune/yarl/v4"
)

// RedisBackend stores rate-limit counters in Redis.
// Create one with [NewFromClient], [NewStandalone], or [NewSentinel].
// Call [RedisBackend.Close] on shutdown when using [NewStandalone] or [NewSentinel].
type RedisBackend struct {
	client  redis.UniversalClient
	closeFn func() error
}

// NewFromClient wraps any UniversalClient (standalone, sentinel, or cluster).
// The caller retains ownership of the client lifecycle; Close is a no-op.
func NewFromClient(c redis.UniversalClient) *RedisBackend {
	return &RedisBackend{client: c}
}

// NewStandalone creates a backend connected to a single Redis instance.
// Call [RedisBackend.Close] to release the connection on shutdown.
func NewStandalone(addr string, db int) *RedisBackend {
	c := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	return &RedisBackend{client: c, closeFn: c.Close}
}

// NewSentinel creates a backend connected via Redis Sentinel.
// Call [RedisBackend.Close] to release the connection on shutdown.
func NewSentinel(masterName string, sentinelAddrs []string, db int) *RedisBackend {
	c := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    masterName,
		SentinelAddrs: sentinelAddrs,
		DB:            db,
	})
	return &RedisBackend{client: c, closeFn: c.Close}
}

// Close releases the Redis connection when the backend owns it (NewStandalone / NewSentinel).
// It is a no-op when the backend was created with NewFromClient.
func (r *RedisBackend) Close() error {
	if r.closeFn != nil {
		return r.closeFn()
	}
	return nil
}

// IncAndGetTTL atomically increments the counter for key by 1, sets its expiry
// to ttl only on first creation (ExpireNX — requires Redis >= 7.0), and returns
// the new counter value and remaining TTL.
func (r *RedisBackend) IncAndGetTTL(ctx context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	results, err := r.IncAndGetTTLBatch(ctx, []yarl.BatchEntry{{Key: key, TTL: ttl}})
	if err != nil {
		return 0, 0, err
	}
	return results[0].Count, results[0].Remaining, nil
}

// IncAndGetTTLBatch processes all entries in a single Redis pipeline — one round-trip
// regardless of how many entries are passed. Implements [yarl.BatchBackend].
func (r *RedisBackend) IncAndGetTTLBatch(ctx context.Context, entries []yarl.BatchEntry) ([]yarl.BatchResult, error) {
	pipe := r.client.Pipeline()

	incrCmds := make([]*redis.IntCmd, len(entries))
	ttlCmds := make([]*redis.DurationCmd, len(entries))

	for i, e := range entries {
		incrCmds[i] = pipe.Incr(ctx, e.Key)
		pipe.ExpireNX(ctx, e.Key, e.TTL)
		ttlCmds[i] = pipe.TTL(ctx, e.Key)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	results := make([]yarl.BatchResult, len(entries))
	for i, e := range entries {
		remaining := ttlCmds[i].Val()
		if remaining < 0 {
			remaining = e.TTL
		}
		results[i] = yarl.BatchResult{Count: incrCmds[i].Val(), Remaining: remaining}
	}
	return results, nil
}
