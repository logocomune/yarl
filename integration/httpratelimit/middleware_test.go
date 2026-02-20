package httpratelimit

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/logocomune/yarl/v2"
	"github.com/stretchr/testify/assert"
)

func TestMiddlewareWithIp(t *testing.T) {
	max := 10
	configuration := Configuration{
		y:       yarl.New("prefix", NewMockLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   true,
		Headers: nil,
	}

	rateLimit := New(&configuration, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	count := 0

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
		})

		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
		assert.Equal(t, strconv.Itoa(max-count), w.Header().Get(xRateLimitRemaining))
	}

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
		})

		count++

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
		assert.Equal(t, "0", w.Header().Get(xRateLimitRemaining))

		assert.NotEmpty(t, w.Header().Get(xRateLimitReset))
	}
}

func TestMiddlewareWithIpAndHeader(t *testing.T) {
	max := 10
	configuration := Configuration{
		y:       yarl.New("prefix", NewMockLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   true,
		Headers: []string{"HEADER1"},
	}

	rateLimit := New(&configuration, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	count := 0

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header1",
		})
		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
		assert.Equal(t, strconv.Itoa(max-count), w.Header().Get(xRateLimitRemaining))
	}

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header1",
		})

		count++

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
		assert.Equal(t, "0", w.Header().Get(xRateLimitRemaining))

		assert.NotEmpty(t, w.Header().Get(xRateLimitReset))
	}

	count = 0

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header2",
		})

		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
		assert.Equal(t, strconv.Itoa(max-count), w.Header().Get(xRateLimitRemaining))
	}

	count = 0

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.2",
			"Header1":         "header2",
		})
		count++

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
		assert.Equal(t, strconv.Itoa(max-count), w.Header().Get(xRateLimitRemaining))
	}

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{
			"X-Forwarded-For": "127.0.0.1",
			"Header1":         "header2",
		})

		count++

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, strconv.Itoa(max), w.Header().Get(xRateLimitLimit))
		assert.Equal(t, "0", w.Header().Get(xRateLimitRemaining))

		assert.NotEmpty(t, w.Header().Get(xRateLimitReset))
	}
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

// errMockLimiter always returns an error.
type errMockLimiter struct{}

func (m *errMockLimiter) Inc(_ string, _ int64) (int64, error) {
	return 0, errors.New("limiter error")
}

func NewMockLimiter(current int, err error, t *testing.T) *MockLimiter {
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

// TestGetIP tests the getIP helper function.
func TestGetIP(t *testing.T) {
	tests := []struct {
		name         string
		forwardedFor string
		remoteAddr   string
		expectedIP   string
	}{
		{
			name:         "X-Forwarded-For takes precedence over RemoteAddr",
			forwardedFor: "203.0.113.1",
			remoteAddr:   "10.0.0.1:1234",
			expectedIP:   "203.0.113.1",
		},
		{
			name:         "RemoteAddr IPv4 fallback",
			forwardedFor: "",
			remoteAddr:   "192.168.1.1:5678",
			expectedIP:   "192.168.1.1",
		},
		{
			name:         "RemoteAddr IPv6 fallback",
			forwardedFor: "",
			remoteAddr:   "[::1]:8080",
			expectedIP:   "[::1]",
		},
		{
			name:         "RemoteAddr hostname fallback",
			forwardedFor: "",
			remoteAddr:   "localhost:8080",
			expectedIP:   "localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)
			if tt.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardedFor)
			}
			req.RemoteAddr = tt.remoteAddr
			got := getIP(req)
			assert.Equal(t, tt.expectedIP, got)
		})
	}
}

// TestMiddleware_LimiterError verifies that a limiter error produces HTTP 500.
func TestMiddleware_LimiterError(t *testing.T) {
	configuration := Configuration{
		y:     yarl.New("prefix", &errMockLimiter{}, 10, time.Hour),
		UseIP: true,
	}

	rateLimit := New(&configuration, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := getRequest(rateLimit, "/", map[string]string{
		"X-Forwarded-For": "127.0.0.1",
	})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestMiddleware_WithoutIPAndHeaders verifies that all requests share a single bucket
// when neither IP nor custom headers are used as rate-limit keys.
func TestMiddleware_WithoutIPAndHeaders(t *testing.T) {
	max := 5
	configuration := Configuration{
		y:       yarl.New("prefix", NewMockLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   false,
		Headers: nil,
	}

	rateLimit := New(&configuration, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{})
		assert.Equal(t, http.StatusOK, w.Code)
	}

	w := getRequest(rateLimit, "/", map[string]string{})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get(xRateRetryAfter))
}

// TestMiddleware_WithHeaderOnly verifies rate limiting by a custom header (no IP).
func TestMiddleware_WithHeaderOnly(t *testing.T) {
	max := 5
	configuration := Configuration{
		y:       yarl.New("prefix", NewMockLimiter(0, nil, t), int64(max), time.Hour),
		UseIP:   false,
		Headers: []string{"X-User-ID"},
	}

	rateLimit := New(&configuration, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// First user: 5 allowed, then rate limited
	for i := 0; i < max; i++ {
		w := getRequest(rateLimit, "/", map[string]string{"X-User-Id": "user1"})
		assert.Equal(t, http.StatusOK, w.Code)
	}
	w := getRequest(rateLimit, "/", map[string]string{"X-User-Id": "user1"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Second user: has its own bucket, not rate limited
	w = getRequest(rateLimit, "/", map[string]string{"X-User-Id": "user2"})
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestMiddleware_RetryAfterHeader verifies the Retry-After header is set when rate limited.
func TestMiddleware_RetryAfterHeader(t *testing.T) {
	max := 2
	configuration := Configuration{
		y:     yarl.New("prefix", NewMockLimiter(0, nil, t), int64(max), time.Minute),
		UseIP: true,
	}

	rateLimit := New(&configuration, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exhaust the limit
	for i := 0; i < max; i++ {
		getRequest(rateLimit, "/", map[string]string{"X-Forwarded-For": "1.2.3.4"})
	}

	// Next request should be rate limited with Retry-After header
	w := getRequest(rateLimit, "/", map[string]string{"X-Forwarded-For": "1.2.3.4"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get(xRateRetryAfter))

	retryAfter, err := strconv.ParseInt(w.Header().Get(xRateRetryAfter), 10, 64)
	assert.NoError(t, err)
	assert.Greater(t, retryAfter, int64(0))
}

// TestNewConfigurationWithLru verifies the LRU configuration factory returns a valid config.
func TestNewConfigurationWithLru(t *testing.T) {
	conf := NewConfigurationWithLru("prefix", 100, 10, time.Minute)
	assert.NotNil(t, conf)

	// Verify it works end-to-end
	rateLimit := New(conf, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	w := getRequest(rateLimit, "/", map[string]string{"X-Forwarded-For": "10.0.0.1"})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "10", w.Header().Get(xRateLimitLimit))
}
