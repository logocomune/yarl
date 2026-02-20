// Package redigoyarl provides a Redis backend for YARL using the Redigo client library.
//
// It uses SELECT + INCR + EXPIRE wrapped in an EXEC call so that all commands are
// sent to Redis in a single round-trip, and supports selecting a specific Redis
// database.
package redigoyarl

import (
	"errors"

	"github.com/gomodule/redigo/redis"
)

// Pool is a YARL backend that stores rate-limit counters in Redis using a Redigo
// connection pool. Create one with [NewPool].
type Pool struct {
	pool    *redis.Pool
	redisDb int
}

// NewPool wraps an existing Redigo connection pool into a YARL-compatible backend.
// redisDb is the Redis database index to SELECT before executing commands.
func NewPool(p *redis.Pool, redisDb int) *Pool {
	return &Pool{
		pool:    p,
		redisDb: redisDb,
	}
}

// Inc atomically increments the counter for key in Redis and sets its TTL to
// ttlSeconds. The SELECT, INCR, and EXPIRE commands run inside a MULTI/EXEC
// transaction. Returns the new counter value, or -1 and an error on failure.
func (p *Pool) Inc(key string, ttlSeconds int64) (int, error) {
	c := p.pool.Get()
	defer c.Close()

	if _, err := c.Do("MULTI"); err != nil {
		return -1, err
	}

	c.Send("SELECT", p.redisDb)
	c.Send("INCR", key)
	c.Send("EXPIRE", key, ttlSeconds)
	reply, err := c.Do("EXEC")

	values, err := redis.Values(reply, err)
	if err != nil {
		return -1, err
	}

	if i, ok := values[1].(int64); ok {
		return int(i), nil
	}

	return 0, errors.New("bad counter value")
}
