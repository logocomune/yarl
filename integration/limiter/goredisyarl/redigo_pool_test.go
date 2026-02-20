package redigoyarl

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestGoRedis_Inc(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	defer client.Close()

	p := NewPool(client)

	key := "test_goredis_inc"
	client.Del(ctx, key)

	val, err := p.Inc(key, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	val, err = p.Inc(key, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), val)
}
