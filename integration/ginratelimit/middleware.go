// Package ginratelimit provides rate-limiting middleware for the Gin web framework.
//
// Register the handler returned by [New] with router.Use() to enforce rate limits on
// all routes, or on specific route groups. When a request exceeds the limit the
// middleware calls c.AbortWithStatus(429) and sets the standard X-RateLimit-* and
// Retry-After response headers.
package ginratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/logocomune/yarl/v3"
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

// Configuration holds the settings for the Gin rate-limit middleware.
// Create one with [NewConfiguration], [NewConfigurationWithGoRedis], or
// [NewConfigurationWithLru].
type Configuration struct {
	// y is the underlying YARL rate limiter.
	y yarl.Yarl
	// closeFn releases resources owned by the configuration, when present.
	closeFn func() error
	// UseIP causes the client IP (from c.ClientIP()) to be included in the
	// rate-limit key so that each client gets its own counter.
	UseIP bool
	// Headers is a list of request header names whose values are appended to the
	// rate-limit key. Useful for per-user or per-tenant limiting (e.g. "X-Tenant-ID").
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
// This is the simplest option and requires no external services, but state is local
// to the process and is lost on restart.
func NewConfigurationWithLru(prefix string, size int, limit int64, tWindow time.Duration) *Configuration {
	r, err := lruyarl.New(size)
	if err != nil {
		panic(err)
	}

	return NewConfiguration(prefix, r, limit, tWindow)
}

// New returns a gin.HandlerFunc that enforces rate limits defined by conf.
// Register it with router.Use() or on individual route groups.
// On each request the middleware checks the rate limit; if exceeded it aborts
// the Gin context with HTTP 429 and never calls c.Next().
func New(conf *Configuration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := ""
		if conf.UseIP {
			key += c.ClientIP()
		}

		if conf.Headers != nil {
			for _, h := range conf.Headers {
				key += ":" + strings.ToLower(c.GetHeader(h)) + ":"
			}
		}

		yResp, err := conf.y.IsAllow(key)

		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		c.Header(xRateLimitLimit, strconv.FormatInt(yResp.Max, 10))
		c.Header(xRateLimitRemaining, strconv.FormatInt(yResp.Remain, 10))
		c.Header(xRateLimitReset, strconv.FormatInt(yResp.NextReset, 10))

		if !yResp.IsAllowed {
			c.Header(xRateRetryAfter, strconv.FormatInt(yResp.RetryAfter, 10))
			c.AbortWithStatus(http.StatusTooManyRequests)

			return
		}

		c.Next()
	}
}

// Close releases any resources owned by the configuration.
func (c *Configuration) Close() error {
	if c == nil || c.closeFn == nil {
		return nil
	}

	return c.closeFn()
}
