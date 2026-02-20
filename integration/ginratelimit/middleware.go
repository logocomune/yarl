// Package ginratelimit provides rate-limiting middleware for the Gin web framework.
//
// Register the handler returned by [New] with router.Use() to enforce rate limits on
// all routes, or on specific route groups. When a request exceeds the limit the
// middleware calls c.AbortWithStatus(429) and sets the standard X-RateLimit-* and
// Retry-After response headers.
package ginratelimit

import (
	"github.com/logocomune/yarl/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/logocomune/yarl/v2/integration/limiter/radixyarl"
	"github.com/mediocregopher/radix/v3"
)

const (
	xRateLimitLimit     = "X-RateLimit-Limit"
	xRateLimitRemaining = "X-RateLimit-Remaining"
	xRateLimitReset     = "X-RateLimit-Reset"
	xRateRetryAfter     = "Retry-After"
)

// Configuration holds the settings for the Gin rate-limit middleware.
// Create one with [NewConfigurationWithRadix], or assemble it manually
// if you need a custom [yarl.Limiter] backend.
type Configuration struct {
	// y is the underlying YARL rate limiter.
	y yarl.Yarl
	// UseIP causes the client IP (from c.ClientIP()) to be included in the
	// rate-limit key so that each client gets its own counter.
	UseIP bool
	// Headers is a list of request header names whose values are appended to the
	// rate-limit key. Useful for per-user or per-tenant limiting (e.g. "X-Tenant-ID").
	Headers []string
}

// NewConfigurationWithRadix creates a [Configuration] backed by a Redis connection
// pool managed by the Radix client library.
//
//   - prefix namespaces the Redis keys.
//   - redisHost is the Redis server hostname.
//   - redisPort is the number of connections in the Radix pool.
//   - redisDb is the Redis database index to select.
//   - limit is the maximum number of requests per tWindow.
//   - tWindow is the duration of the rate-limit window (e.g. time.Minute).
//
// Panics if the pool cannot be created.
func NewConfigurationWithRadix(prefix string, redisHost string, redisPort int, redisDb int, limit int64, tWindow time.Duration) *Configuration {
	pool, err := radix.NewPool("tcp", redisHost, redisPort)
	if err != nil {
		panic(err)
	}

	r := radixyarl.New(pool)

	return &Configuration{
		y: yarl.New(prefix, r, limit, tWindow),
	}
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
