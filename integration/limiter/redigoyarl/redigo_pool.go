package redigoyarl

import (
	"errors"

	"github.com/gomodule/redigo/redis"
)

type Pool struct {
	pool    *redis.Pool
	redisDb int
}

func NewPool(p *redis.Pool, redisDb int) *Pool {
	return &Pool{
		pool:    p,
		redisDb: redisDb,
	}
}

func (p *Pool) Inc(key string, ttlSeconds int64) (int, error) {
	c := p.pool.Get()
	defer c.Close()
	c.Send("SELECT", p.redisDb)
	c.Send("INCR", key)
	c.Send("EXPIRE", key, ttlSeconds)
	reply, err := c.Do("EXEC")

	values, err := redis.Values(reply, err)
	if err != nil {
		return -1, err
	}

	if i, ok := values[1].(int); ok {
		return i, nil
	}

	return 0, errors.New("bad counter value")
}
