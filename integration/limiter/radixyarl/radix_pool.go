// Package radixyarl provides a Redis backend for YARL using the Radix v3 client library.
//
// It uses a Redis INCR + EXPIRE pipeline for atomic counter increments with TTL support,
// making it suitable for distributed rate limiting across multiple processes or machines.
package radixyarl

import (
	"github.com/mediocregopher/radix/v3"
)

// Pool is a YARL backend that stores rate-limit counters in Redis using the
// Radix connection pool. Create one with [New].
type Pool struct {
	pool *radix.Pool
}

// New wraps an existing Radix connection pool into a YARL-compatible backend.
func New(p *radix.Pool) *Pool {
	return &Pool{
		pool: p,
	}
}

// Inc atomically increments the counter for key in Redis and sets its TTL to
// ttlSeconds. The INCR and EXPIRE commands are sent in a single pipeline for
// efficiency. Returns the new counter value, or -1 and an error on failure.
func (r *Pool) Inc(key string, ttlSeconds int64) (int64, error) {
	var count int
	pp := radix.Pipeline(
		radix.Cmd(&count, "INCR", key),
		radix.FlatCmd(nil, "EXPIRE", key, ttlSeconds),
	)

	if err := r.pool.Do(pp); err != nil {
		return -1, err
	}

	return int64(count), nil
}
