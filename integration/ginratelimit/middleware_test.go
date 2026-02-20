package ginratelimit

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/logocomune/yarl/v2"
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
	// RemoteAddr must be set so that gin can evaluate trusted-proxy rules and
	// honour the X-Forwarded-For header via c.ClientIP().
	req.RemoteAddr = "127.0.0.1:1234"

	for k := range headers {
		req.Header.Add(k, headers[k])
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

// ginErrMockLimiter always returns an error.
type ginErrMockLimiter struct{}

func (m *ginErrMockLimiter) Inc(_ string, _ int64) (int64, error) {
	return 0, errors.New("limiter error")
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

// TestGinMiddleware_LimiterError verifies that a limiter error aborts with HTTP 500.
func TestGinMiddleware_LimiterError(t *testing.T) {
	configuration := Configuration{
		y:     yarl.New("prefix", &ginErrMockLimiter{}, 10, time.Hour),
		UseIP: false,
	}

	rateLimit := New(&configuration)
	router := getRouter(rateLimit)

	w := getRequest(router, "/", map[string]string{})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGinMiddleware_WithoutHeaders verifies that all requests share one bucket
// when neither IP nor custom headers are configured.
func TestGinMiddleware_WithoutHeaders(t *testing.T) {
	max := 5
	configuration := Configuration{
		y:       yarl.New("prefix", NewMoclLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   false,
		Headers: nil,
	}

	rateLimit := New(&configuration)
	router := getRouter(rateLimit)

	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{})
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
	}

	w := getRequest(router, "/", map[string]string{})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get(xRateRetryAfter))
}

// TestGinMiddleware_WithHeaderOnly verifies rate limiting by a custom header without IP.
func TestGinMiddleware_WithHeaderOnly(t *testing.T) {
	max := 3
	configuration := Configuration{
		y:       yarl.New("prefix", NewMoclLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   false,
		Headers: []string{"X-Tenant-ID"},
	}

	rateLimit := New(&configuration)
	router := getRouter(rateLimit)

	// tenant-A: 3 allowed, then rate limited
	for i := 0; i < max; i++ {
		w := getRequest(router, "/", map[string]string{"X-Tenant-Id": "tenant-A"})
		assert.Equal(t, http.StatusOK, w.Code)
	}
	w := getRequest(router, "/", map[string]string{"X-Tenant-Id": "tenant-A"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// tenant-B: own bucket, not rate limited
	w = getRequest(router, "/", map[string]string{"X-Tenant-Id": "tenant-B"})
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGinMiddleware_RetryAfterHeader verifies Retry-After is set when rate limited.
func TestGinMiddleware_RetryAfterHeader(t *testing.T) {
	max := 2
	configuration := Configuration{
		y:       yarl.New("prefix", NewMoclLimiter(0, nil, t), int64(max), time.Minute),
		UseIP:   false,
		Headers: []string{"X-Client"},
	}

	rateLimit := New(&configuration)
	router := getRouter(rateLimit)

	for i := 0; i < max; i++ {
		getRequest(router, "/", map[string]string{"X-Client": "c1"})
	}

	w := getRequest(router, "/", map[string]string{"X-Client": "c1"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get(xRateRetryAfter))

	retryAfter, err := strconv.ParseInt(w.Header().Get(xRateRetryAfter), 10, 64)
	assert.NoError(t, err)
	assert.Greater(t, retryAfter, int64(0))
}
