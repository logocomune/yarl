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
	IsAllowed  bool
	Current    int
	Max        int
	Remain     int
	NextReset  int64
	RetryAfter int64
}

type Limiter interface {
	Inc(key string, ttlSeconds int64) (int, error)
}

// New initialize Yarl.
func New(prefix string, l Limiter, max int, timeWindow time.Duration) Yarl {
	return Yarl{
		prefix:  prefix,
		tWindow: timeWindow,
		max:     max,
		limiter: l,
	}
}

// IsAllow evaluate limit for key.
func (y *Yarl) IsAllow(key string) (*Resp, error) {
	return y.IsAllowWithLimit(key, y.max, y.tWindow)
}

// IsAllowWithLimit evaluate custom limit for key.
func (y *Yarl) IsAllowWithLimit(key string, max int, tWindow time.Duration) (*Resp, error) {
	sec, resetAt := nextResetInSec(time.Now(), tWindow)

	try, err := y.limiter.Inc(y.keyBuilder(key, tWindow), ttl(sec+ttlSafeWindowInSec))

	if err != nil {
		return nil, err
	}

	r := Resp{
		IsAllowed:  false,
		Max:        max,
		Remain:     0,
		Current:    try,
		NextReset:  resetAt,
		RetryAfter: sec,
	}

	if try > max {
		return &r, nil
	}

	r.Remain = max - try
	r.IsAllowed = true

	return &r, nil
}

func (y *Yarl) keyBuilder(k string, tWindow time.Duration) string {
	s := timeKey(time.Now(), tWindow) + "_" + k
	if y.prefix != "" {
		s = y.prefix + "_" + s
	}

	return s
}
