package radixyarl

import (
	"strconv"

	"github.com/mediocregopher/radix/v3"
)

type Pool struct {
	pool    *radix.Pool
	redisDb string
}

func New(p *radix.Pool, redisDb int) *Pool {
	return &Pool{
		pool:    p,
		redisDb: strconv.Itoa(redisDb),
	}
}

func (r *Pool) Inc(key string, ttlSeconds int64) (int, error) {
	var count int
	pp := radix.Pipeline(
		radix.FlatCmd(nil, "SELECT", r.redisDb),
		radix.Cmd(&count, "INCR", key),
		radix.FlatCmd(nil, "EXPIRE", key, ttlSeconds),
	)

	if err := r.pool.Do(pp); err != nil {
		return -1, err
	}

	return count, nil
}
