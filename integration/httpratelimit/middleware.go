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

type Configuration struct {
	y       yarl.Yarl
	UseIP   bool
	Headers []string
}

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

func NewConfigurationWithLru(prefix string, size int, limit int64, tWindow time.Duration) *Configuration {
	r, err := lruyarl.New(size)
	if err != nil {
		panic(err)
	}

	return &Configuration{
		y: yarl.New(prefix, r, limit, tWindow),
	}
}

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
