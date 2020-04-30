package httpratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"

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

func NewConfigurationWithRadix(prefix string, poolsize int, redisHost string, redisPort string, redisDb int, limit int, tWindow time.Duration) *Configuration {
	pool, err := radix.NewPool("tcp", redisHost+":"+redisPort, poolsize)
	if err != nil {
		panic(err)
	}

	r := radixyarl.New(pool, redisDb)

	return &Configuration{
		y: yarl.New(prefix, r, limit, tWindow),
	}
}

func New(conf *Configuration, h http.Handler) http.HandlerFunc {

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

		w.Header().Set(xRateLimitLimit, strconv.Itoa(yResp.Max))
		w.Header().Set(xRateLimitRemaining, strconv.Itoa(yResp.Remain))
		w.Header().Set(xRateLimitReset, strconv.FormatInt(yResp.NexReset, 10))

		if !yResp.IsAllowed {
			w.Header().Set(xRateRetryAfter, strconv.FormatInt(yResp.RetryAfter, 10))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Too many  requests."))

			return
		}

		h.ServeHTTP(w, r)
	}
}

func getIP(r *http.Request) string {
	forwarded := r.Header.Get("X-FORWARDED-FOR")
	if forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}
