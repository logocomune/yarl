// Package httpratelimit provides rate-limiting middleware for the standard net/http package.
//
// Wrap any http.HandlerFunc with [New] to automatically enforce rate limits based on
// the client IP address, arbitrary request headers, or a combination of both.
// When a request exceeds the limit the middleware responds with HTTP 429 Too Many Requests
// and sets the standard X-RateLimit-* and Retry-After response headers.
package httpratelimit

import (
	yarl "github.com/logocomune/yarl/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/logocomune/yarl/v2/integration/limiter/lruyarl"
	"github.com/logocomune/yarl/v2/integration/limiter/radixyarl"
	"github.com/mediocregopher/radix/v3"
)

const (
	xRateLimitLimit     = "X-RateLimit-Limit"
	xRateLimitRemaining = "X-RateLimit-Remaining"
	xRateLimitReset     = "X-RateLimit-Reset"
	xRateRetryAfter     = "Retry-After"
)

// Configuration holds the settings for the rate-limit middleware.
// Create one with [NewConfigurationWithRadix] or [NewConfigurationWithLru],
// or assemble it manually if you need a custom [yarl.Limiter] backend.
type Configuration struct {
	// y is the underlying YARL rate limiter.
	y yarl.Yarl
	// UseIP causes the client IP address to be included in the rate-limit key
	// so that each client gets its own counter.
	UseIP bool
	// Headers is a list of request header names whose values are appended to the
	// rate-limit key. Useful for per-user or per-tenant limiting (e.g. "X-User-ID").
	Headers []string
}

// NewConfigurationWithRadix creates a [Configuration] backed by a Redis connection
// pool managed by the Radix client library. The pool is created internally; use this
// helper when you do not already have a Radix pool.
//
//   - prefix namespaces the Redis keys.
//   - poolsize is the number of connections in the Radix pool.
//   - redisHost / redisPort / redisDb identify the Redis instance.
//   - limit is the maximum number of requests per tWindow.
//   - tWindow is the duration of the sliding window (e.g. time.Minute).
//
// Panics if the pool cannot be created.
func NewConfigurationWithRadix(prefix string, poolsize int, redisHost string, redisPort string, redisDb int, limit int, tWindow time.Duration) *Configuration {
	customConnFunc := func(network, addr string) (radix.Conn, error) {
		return radix.Dial(network, addr,
			radix.DialTimeout(10*time.Second),
			radix.DialSelectDB(redisDb),
		)
	}

	pool, err := radix.NewPool("tcp", redisHost+":"+redisPort, poolsize, radix.PoolConnFunc(customConnFunc))
	if err != nil {
		panic(err)
	}

	r := radixyarl.New(pool)

	return &Configuration{
		y: yarl.New(prefix, r, int64(limit), tWindow),
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

	return &Configuration{
		y: yarl.New(prefix, r, limit, tWindow),
	}
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
			w.Write([]byte("Too many  requests."))

			return
		}

		h.ServeHTTP(w, r)
	}
}

// getIP extracts the client IP from the request. It prefers the X-Forwarded-For header
// (set by reverse proxies) and falls back to RemoteAddr.
func getIP(r *http.Request) string {
	forwarded := r.Header.Get("X-FORWARDED-FOR")
	if forwarded != "" {
		return forwarded
	}

	ipComponents := strings.Split(r.RemoteAddr, ":")

	if len(ipComponents) == 0 {
		return ipComponents[0]
	}

	return strings.Join(ipComponents[:len(ipComponents)-1], ":")
}
