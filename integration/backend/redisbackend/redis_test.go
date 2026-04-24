package redisbackend

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	yarl "github.com/logocomune/yarl/v4"
)

func redisClient(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		t.Skipf("Redis not available: %v", err)
	}
	return client
}

func TestRedisBackend_IncAndGetTTL_Increments(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	ctx := context.Background()
	key := fmt.Sprintf("test:incr:%d", time.Now().UnixNano())
	client.Del(ctx, key)

	b := NewFromClient(client)

	count, remaining, err := b.IncAndGetTTL(ctx, key, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Greater(t, remaining, time.Duration(0))

	count, _, err = b.IncAndGetTTL(ctx, key, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestRedisBackend_IncAndGetTTL_TTLNotRefreshed(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	ctx := context.Background()
	key := fmt.Sprintf("test:ttl:%d", time.Now().UnixNano())
	client.Del(ctx, key)

	b := NewFromClient(client)

	_, first, err := b.IncAndGetTTL(ctx, key, 10*time.Second)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	_, second, err := b.IncAndGetTTL(ctx, key, 10*time.Second)
	require.NoError(t, err)

	// TTL must decrease, not refresh
	assert.Less(t, second, first, "ExpireNX must not reset TTL on subsequent calls")
}

func TestRedisBackend_IncAndGetTTL_RemainingApproximatesWindowDuration(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	ctx := context.Background()
	key := fmt.Sprintf("test:rem:%d", time.Now().UnixNano())
	client.Del(ctx, key)

	b := NewFromClient(client)
	ttl := 30 * time.Second

	_, remaining, err := b.IncAndGetTTL(ctx, key, ttl)
	require.NoError(t, err)
	assert.InDelta(t, ttl.Seconds(), remaining.Seconds(), 2, "remaining should be close to ttl on first call")
}

func TestNewStandalone_Close(t *testing.T) {
	b := NewStandalone("localhost:6379", 0)
	assert.NotNil(t, b)
	assert.NoError(t, b.Close())
}

func TestNewFromClient_CloseIsNoOp(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer client.Close()
	b := NewFromClient(client)
	assert.NoError(t, b.Close()) // must not close the caller-owned client
}

func TestRedisBackend_IncAndGetTTLBatch_SingleRoundTrip(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	ctx := context.Background()
	nano := time.Now().UnixNano()
	keys := []string{
		fmt.Sprintf("test:batch:r1:%d", nano),
		fmt.Sprintf("test:batch:r2:%d", nano),
		fmt.Sprintf("test:batch:r3:%d", nano),
	}
	client.Del(ctx, keys...)

	b := NewFromClient(client)
	entries := []yarl.BatchEntry{
		{Key: keys[0], TTL: 10 * time.Second},
		{Key: keys[1], TTL: time.Minute},
		{Key: keys[2], TTL: time.Hour},
	}

	results, err := b.IncAndGetTTLBatch(ctx, entries)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i, res := range results {
		assert.Equal(t, int64(1), res.Count, "key %d count", i)
		assert.Greater(t, res.Remaining, time.Duration(0), "key %d remaining", i)
		assert.LessOrEqual(t, res.Remaining, entries[i].TTL, "key %d remaining <= ttl", i)
	}
}

func TestRedisBackend_IncAndGetTTLBatch_TTLNotRefreshed(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	ctx := context.Background()
	key := fmt.Sprintf("test:batch:ttl:%d", time.Now().UnixNano())
	client.Del(ctx, key)

	b := NewFromClient(client)
	entry := []yarl.BatchEntry{{Key: key, TTL: 10 * time.Second}}

	first, err := b.IncAndGetTTLBatch(ctx, entry)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	second, err := b.IncAndGetTTLBatch(ctx, entry)
	require.NoError(t, err)

	assert.Less(t, second[0].Remaining, first[0].Remaining, "batch ExpireNX must not refresh TTL")
}
