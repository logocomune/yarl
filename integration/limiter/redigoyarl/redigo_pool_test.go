package redigoyarl

import (
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
)

func TestPool_Inc(t *testing.T) {
	pool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "localhost:6379")
		},
	}

	conn := pool.Get()
	defer conn.Close()
	if _, err := conn.Do("PING"); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	p := NewPool(pool, 0)

	key := "test_redigo_inc"
	// Clean up
	conn.Do("DEL", key)

	val, err := p.Inc(key, 10)
	assert.NoError(t, err)
	assert.Equal(t, 1, val)

	val, err = p.Inc(key, 10)
	assert.NoError(t, err)
	assert.Equal(t, 2, val)
}
