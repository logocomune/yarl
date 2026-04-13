// Package httpratelimit provides rate-limiting middleware for the standard net/http package.
//
// Wrap any http.HandlerFunc with [New] to automatically enforce rate limits based on
// the client IP address, arbitrary request headers, or a combination of both.
// When a request exceeds the limit the middleware responds with HTTP 429 Too Many Requests
// and sets the standard X-RateLimit-* and Retry-After response headers.
package httpratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	yarl "github.com/logocomune/yarl/v3"
	"github.com/logocomune/yarl/v3/integration/limiter/goredisyarl"
	"github.com/logocomune/yarl/v3/integration/limiter/lruyarl"
	"github.com/redis/go-redis/v9"
)

const (
	xRateLimitLimit     = "X-RateLimit-Limit"
	xRateLimitRemaining = "X-RateLimit-Remaining"
	xRateLimitReset     = "X-RateLimit-Reset"
	xRateRetryAfter     = "Retry-After"
)

// Configuration holds the settings for the rate-limit middleware.
// Create one with [NewConfiguration], [NewConfigurationWithGoRedis], or
// [NewConfigurationWithLru].
type Configuration struct {
	// y is the underlying YARL rate limiter.
	y yarl.Yarl
	// closeFn releases resources owned by the configuration, when present.
	closeFn func() error
	// UseIP causes the client IP address to be included in the rate-limit key
	// so that each client gets its own counter.
	UseIP bool
	// Headers is a list of request header names whose values are appended to the
	// rate-limit key. Useful for per-user or per-tenant limiting (e.g. "X-User-ID").
	Headers []string
}

// NewConfiguration creates a [Configuration] from any [yarl.Limiter] backend.
func NewConfiguration(prefix string, l yarl.Limiter, limit int64, tWindow time.Duration) *Configuration {
	return &Configuration{
		y: yarl.New(prefix, l, limit, tWindow),
	}
}

// NewConfigurationWithRedisClient creates a [Configuration] from an existing
// go-redis client. The caller retains ownership of client lifecycle.
func NewConfigurationWithRedisClient(prefix string, client *redis.Client, limit int64, tWindow time.Duration) *Configuration {
	return NewConfiguration(prefix, goredisyarl.NewPool(client), limit, tWindow)
}

// NewConfigurationWithGoRedis creates a [Configuration] backed by a go-redis client.
// The client is created internally; use this helper when you do not already have a
// configured *redis.Client.
//
//   - prefix namespaces the Redis keys.
//   - redisAddr identifies the Redis instance, e.g. "localhost:6379".
//   - redisDb is the Redis database index.
//   - limit is the maximum number of requests per tWindow.
//   - tWindow is the duration of the rate-limit window (e.g. time.Minute).
func NewConfigurationWithGoRedis(prefix string, redisAddr string, redisDb int, limit int64, tWindow time.Duration) *Configuration {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   redisDb,
	})

	return &Configuration{
		y:       yarl.New(prefix, goredisyarl.NewPool(client), limit, tWindow),
		closeFn: client.Close,
	}
}

// NewConfigurationWithLru creates a [Configuration] backed by an in-memory LRU cache.
// This is the simplest option and requires no external dependencies, but rate-limit
// state is local to the process and is lost on restart.
//
//   - prefix namespaces the cache keys.
//   - size is the maximum number of distinct keys tracked simultaneously.
//   - limit is the maximum number of requests per tWindow.
//   - tWindow is the duration of the rate-limit window (e.g. time.Minute).
//
// Panics if size is less than 1.
func NewConfigurationWithLru(prefix string, size int, limit int64, tWindow time.Duration) *Configuration {
	r, err := lruyarl.New(size)
	if err != nil {
		panic(err)
	}

	return NewConfiguration(prefix, r, limit, tWindow)
}

// New wraps handler h with rate-limiting logic defined by conf.
// The returned HandlerFunc checks every incoming request against the rate limiter
// and either forwards it to h or rejects it with HTTP 429.
func New(conf *Configuration, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := ""
		if conf.UseIP {
			key += getIP(r)
		}

		if conf.Headers != nil {
			for _, h := range conf.Headers {
				key += ":" + strings.ToLower(r.Header.Get(h)) + ":"
			}
		}

		yResp, err := conf.y.IsAllow(key)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal error."))

			return
		}

		w.Header().Set(xRateLimitLimit, strconv.FormatInt(yResp.Max, 10))
		w.Header().Set(xRateLimitRemaining, strconv.FormatInt(yResp.Remain, 10))
		w.Header().Set(xRateLimitReset, strconv.FormatInt(yResp.NextReset, 10))

		if !yResp.IsAllowed {
			w.Header().Set(xRateRetryAfter, strconv.FormatInt(yResp.RetryAfter, 10))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Too many requests."))

			return
		}

		h.ServeHTTP(w, r)
	}
}

// Close releases any resources owned by the configuration.
func (c *Configuration) Close() error {
	if c == nil || c.closeFn == nil {
		return nil
	}

	return c.closeFn()
}

// getIP extracts the client IP from the request. It prefers the X-Forwarded-For header
// (set by reverse proxies) and falls back to RemoteAddr.
func getIP(r *http.Request) string {
	forwarded := r.Header.Get("X-FORWARDED-FOR")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}
