package radixyarl

import (
	"github.com/mediocregopher/radix/v3"
)

type Pool struct {
	pool    *radix.Pool
}

func New(p *radix.Pool) *Pool {
	return &Pool{
		pool:    p,
	}
}

func (r *Pool) Inc(key string, ttlSeconds int64) (int, error) {
	var count int
	pp := radix.Pipeline(
		radix.Cmd(&count, "INCR", key),
		radix.FlatCmd(nil, "EXPIRE", key, ttlSeconds),
	)

	if err := r.pool.Do(pp); err != nil {
		return -1, err
	}

	return count, nil
}
