// Package goredisyarl provides a Redis backend for YARL using the go-redis/v9 client library.
//
// It uses a pipeline with IncrBy and ExpireNX commands, taking advantage of
// go-redis' context-aware API for reliable timeouts.
package goredisyarl

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const timeout = 10 * time.Second

// GoRedis is a YARL backend that stores rate-limit counters in Redis using
// the go-redis client. Create one with [NewPool].
type GoRedis struct {
	client *redis.Client
}

// NewPool wraps an existing go-redis Client into a YARL-compatible backend.
func NewPool(client *redis.Client) *GoRedis {
	return &GoRedis{
		client: client,
	}
}

// Inc atomically increments the counter for key by 1 and sets its TTL only when
// the key is first created, using a go-redis pipeline. A 10-second context
// timeout is applied. Returns the new counter value, or -1 and an error on
// failure.
func (g *GoRedis) Inc(key string, ttlSeconds int64) (int64, error) {
	pipeline := g.client.Pipeline()
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	incCmd := pipeline.IncrBy(ctx, key, 1)
	pipeline.ExpireNX(ctx, key, time.Duration(ttlSeconds)*time.Second)
	_, err := pipeline.Exec(ctx)

	if err != nil {
		return -1, err
	}

	if incCmd != nil {
		return incCmd.Val(), nil
	}

	return 0, errors.New("bad counter value")
}
