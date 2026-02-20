// Package yarl provides a time-window based rate limiter with pluggable storage backends.
//
// The core concept is simple: for a given key (e.g. a client IP, a user ID, or any
// arbitrary string) YARL counts how many times an operation has been performed within
// the current time window. If the count exceeds the configured maximum, the request is
// denied.
//
// Storage backends are swappable through the [Limiter] interface. Out-of-the-box
// implementations are available for in-memory LRU caches and three Redis client libraries
// (Radix, Redigo, and go-redis).
package yarl

import (
	"time"
)

// Yarl is the main rate-limiter instance. Create one with [New] and then call
// [Yarl.IsAllow] or [Yarl.IsAllowWithLimit] on each incoming request.
type Yarl struct {
	prefix  string
	tWindow time.Duration
	max     int64
	limiter Limiter
}

// Resp is the result returned by [Yarl.IsAllow] and [Yarl.IsAllowWithLimit].
// It carries all the information needed to set standard HTTP rate-limit response headers
// (X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After).
type Resp struct {
	// IsAllowed is true when the request is within the allowed limit.
	IsAllowed bool
	// Current is the counter value after this request (i.e. how many times the
	// operation has been performed in the current window, including this call).
	Current int64
	// Max is the maximum number of requests allowed per window.
	Max int64
	// Remain is the number of requests still allowed in the current window.
	// It is 0 when the limit has been reached or exceeded.
	Remain int64
	// NextReset is the Unix timestamp (seconds) at which the current window ends
	// and the counter resets. Suitable for the X-RateLimit-Reset header.
	NextReset int64
	// RetryAfter is the number of seconds until the current window ends.
	// Suitable for the Retry-After header returned on HTTP 429 responses.
	RetryAfter int64
}

// Limiter is the storage backend interface. Any type that implements Inc can be used
// as a backend for YARL.
//
// Inc atomically increments the counter for key and returns the new value.
// ttlSeconds is a hint for how long the key should be kept alive in the store;
// backends that do not support TTLs (e.g. the in-memory LRU) may ignore it.
type Limiter interface {
	Inc(key string, ttlSeconds int64) (int64, error)
}

// New creates a new Yarl rate limiter.
//
//   - prefix is prepended to every cache key, useful for namespacing when multiple
//     Yarl instances share the same backend.
//   - l is the storage backend (see the integration/limiter sub-packages).
//   - max is the maximum number of requests allowed within timeWindow.
//   - timeWindow is the duration of each rate-limit window (e.g. time.Minute for
//     "100 requests per minute").
func New(prefix string, l Limiter, max int64, timeWindow time.Duration) Yarl {
	return Yarl{
		prefix:  prefix,
		tWindow: timeWindow,
		max:     max,
		limiter: l,
	}
}

// IsAllow checks whether another request for key is allowed under the default limit
// and time window configured in [New].
//
// It returns a non-nil *[Resp] on success, or an error if the backend fails.
func (y *Yarl) IsAllow(key string) (*Resp, error) {
	return y.IsAllowWithLimit(key, y.max, y.tWindow)
}

// IsAllowWithLimit is like [Yarl.IsAllow] but overrides the default limit and time window
// for this specific call. This is useful when different endpoints share the same Yarl
// instance but need different rate limits.
//
// It returns a non-nil *[Resp] on success, or an error if the backend fails.
func (y *Yarl) IsAllowWithLimit(key string, max int64, tWindow time.Duration) (*Resp, error) {
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
