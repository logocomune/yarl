package ginratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/logocomune/yarl"
	"github.com/logocomune/yarl/integration/limiter/radixyarl"
	"github.com/mediocregopher/radix/v3"
)

const (
	xRateLimitLimit     = "X-RateLimit-Limit"
	xRateLimitRemaining = "X-RateLimit-Remaining"
	xRateLimitReset     = "X-RateLimit-Reset"
	xRateRetryAfter     = "Retry-After"
)

type Configuration struct {
	y       yarl.Yarl
	UseIP   bool
	Headers []string
}

func NewConfigurationWithRadix(prefix string, redisHost string, redisPort int, redisDb int, limit int, tWindow time.Duration) *Configuration {
	pool, err := radix.NewPool("tcp", redisHost, redisPort)
	if err != nil {
		panic(err)
	}

	r := radixyarl.New(pool, redisDb)

	return &Configuration{
		y: yarl.New(prefix, r, limit, tWindow),
	}
}

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

		c.Header(xRateLimitLimit, strconv.Itoa(yResp.Max))
		c.Header(xRateLimitRemaining, strconv.Itoa(yResp.Remain))
		c.Header(xRateLimitReset, strconv.FormatInt(yResp.NexReset, 10))

		if !yResp.IsAllowed {
			c.Header(xRateRetryAfter, strconv.FormatInt(yResp.NexReset, 10))
			c.AbortWithStatus(http.StatusTooManyRequests)

			return
		}

		c.Next()
	}
}
