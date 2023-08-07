package ginratelimit

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/logocomune/yarl"
	"github.com/stretchr/testify/assert"
)

func TestMiddlewareWithIp(t *testing.T) {
	max := 10
	configuration := Configuration{
		y:       yarl.New("prefix", NewMoclLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   true,
		Headers: nil,
	}

	rateLimit := New(&configuration)
	router := getRouter(rateLimit)
	router.Use(rateLimit)

	count := 0

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
		})

		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, w.Header().Get(xRateLimitLimit), strconv.Itoa(max))
		assert.Equal(t, w.Header().Get(xRateLimitRemaining), strconv.Itoa(max-count))
	}

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
		})

		count++

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, w.Header().Get(xRateLimitLimit), strconv.Itoa(max))
		assert.Equal(t, w.Header().Get(xRateLimitRemaining), "0")

		assert.NotEmpty(t, w.Header().Get(xRateLimitReset))
	}
}

func TestMiddlewareWithIpAndHeader(t *testing.T) {
	max := 10
	configuration := Configuration{
		y:       yarl.New("prefix", NewMoclLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   true,
		Headers: []string{"HEADER1"},
	}

	rateLimit := New(&configuration)
	router := getRouter(rateLimit)
	router.Use(rateLimit)

	count := 0

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header1",
		})
		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, w.Header().Get(xRateLimitLimit), strconv.Itoa(max))
		assert.Equal(t, w.Header().Get(xRateLimitRemaining), strconv.Itoa(max-count))
	}

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header1",
		})

		count++

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, w.Header().Get(xRateLimitLimit), strconv.Itoa(max))
		assert.Equal(t, w.Header().Get(xRateLimitRemaining), "0")

		assert.NotEmpty(t, w.Header().Get(xRateLimitReset))
	}

	count = 0

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header2",
		})

		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, w.Header().Get(xRateLimitLimit), strconv.Itoa(max))
		assert.Equal(t, w.Header().Get(xRateLimitRemaining), strconv.Itoa(max-count))
	}

	count = 0

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.2",
			"Header1":         "header2",
		})
		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, w.Header().Get(xRateLimitLimit), strconv.Itoa(max))
		assert.Equal(t, w.Header().Get(xRateLimitRemaining), strconv.Itoa(max-count))
	}

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header2",
		})

		count++

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, w.Header().Get(xRateLimitLimit), strconv.Itoa(max))
		assert.Equal(t, w.Header().Get(xRateLimitRemaining), "0")

		assert.NotEmpty(t, w.Header().Get(xRateLimitReset))
	}
}

func getRouter(middleware gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	router.Use(middleware)
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	return router
}

func getRequest(r http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", path, nil)

	for k := range headers {
		req.Header.Add(k, headers[k])
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func NewMoclLimiter(current int, err error, t *testing.T) *MockLimiter {
	return &MockLimiter{
		counters: make(map[string]int),
		err:      err,
		t:        t,
	}
}

type MockLimiter struct {
	counters map[string]int
	err      error
	t        *testing.T
}

func (m *MockLimiter) Inc(key string, ttlSeconds int64) (int64, error) {
	m.counters[key]++
	m.t.Logf("Key: '%s' , count: '%d'\n", key, m.counters[key])

	return int64(m.counters[key]), m.err
}
