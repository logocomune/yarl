package redigoyarl

import (
	"errors"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/context"
	"time"
)

const timeout = 10 * time.Second

type GoRedis struct {
	client  *redis.Client
	redisDb int
}

func NewPool(client *redis.Client) *GoRedis {
	return &GoRedis{
		client: client,
	}
}

func (g *GoRedis) Inc(key string, ttlSeconds int64) (int64, error) {
	pipeline := g.client.Pipeline()
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	incCmd := pipeline.IncrBy(ctx, key, 1)
	pipeline.Expire(ctx, key, time.Duration(ttlSeconds)*time.Second)
	_, err := pipeline.Exec(ctx)

	if err != nil {
		return -1, err
	}

	if incCmd != nil {
		return incCmd.Val(), nil
	}

	return 0, errors.New("bad counter value")
}
