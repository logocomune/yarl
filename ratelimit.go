package yarl

import (
	"time"
)

type Yarl struct {
	prefix  string
	tWindow time.Duration
	max     int
	limiter Limiter
}

type Resp struct {
	IsAllowed bool
	Current   int
	Max       int
	Remain    int
	NexReset  int64
}

type Limiter interface {
	Inc(key string, ttlSeconds int64) (int, error)
}

// New initialize Yarl
func New(prefix string, l Limiter, max int, timeWindow time.Duration) Yarl {
	return Yarl{
		prefix:  prefix,
		tWindow: timeWindow,
		max:     max,
		limiter: l,
	}
}

// IsAllow evaluate limit for key
func (y *Yarl) IsAllow(key string) (*Resp, error) {
	return y.IsAllowWithLimit(key, y.max, y.tWindow)
}

// IsAllowWithLimit evaluate custom limit for key
func (y *Yarl) IsAllowWithLimit(key string, max int, tWindow time.Duration) (*Resp, error) {
	try, err := y.limiter.Inc(y.keyBuilder(key), ttl(tWindow))

	if err != nil {
		return nil, err
	}

	r := Resp{
		IsAllowed: false,
		Max:       max,
		Remain:    0,
		Current:   try,
		NexReset:  nextResetInSec(time.Now(), tWindow),
	}

	if try > max {
		return &r, nil
	}

	r.Remain = max - try
	r.IsAllowed = true

	return &r, nil
}

func (y *Yarl) keyBuilder(k string) string {
	s := timeKey(time.Now(), y.tWindow) + "_" + k
	if y.prefix != "" {
		s = y.prefix + "_" + s
	}

	return s
}
