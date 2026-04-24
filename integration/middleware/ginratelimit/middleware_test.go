package ginratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	yarl "github.com/logocomune/yarl/v4"
	"github.com/stretchr/testify/assert"
)

type stubBackend struct {
	mu        sync.Mutex
	counts    map[string]int64
	remaining time.Duration
	err       error
}

func newStubBackend(remaining time.Duration, err error) *stubBackend {
	return &stubBackend{counts: make(map[string]int64), remaining: remaining, err: err}
}

func (s *stubBackend) IncAndGetTTL(_ context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	if s.err != nil {
		return 0, 0, s.err
	}
	s.mu.Lock()
	s.counts[key]++
	count := s.counts[key]
	s.mu.Unlock()
	rem := s.remaining
	if rem == 0 {
		rem = ttl
	}
	return count, rem, nil
}

func init() {
	gin.SetMode(gin.TestMode)
}

func newLimiter(max int64, remaining time.Duration, err error) *yarl.Limiter {
	return yarl.New(
		newStubBackend(remaining, err),
		yarl.Rule{ID: "test", TTL: time.Minute, MaxRequests: max},
	)
}

func doRequest(r http.Handler, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func newRouter(conf *Configuration) *gin.Engine {
	r := gin.New()
	r.Use(New(conf))
	r.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestGinMiddleware_AllowedRequest(t *testing.T) {
	conf := NewConfiguration(newLimiter(10, time.Minute, nil))
	conf.UseIP = true

	w := doRequest(newRouter(conf), map[string]string{"X-Forwarded-For": "1.2.3.4"})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGinMiddleware_BlockedRequest_Returns429(t *testing.T) {
	conf := NewConfiguration(newLimiter(1, 30*time.Second, nil))
	conf.UseIP = true

	r := newRouter(conf)
	doRequest(r, map[string]string{"X-Forwarded-For": "1.2.3.4"}) // first: allowed
	w := doRequest(r, map[string]string{"X-Forwarded-For": "1.2.3.4"}) // second: blocked

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestGinMiddleware_BackendError_Returns500(t *testing.T) {
	conf := NewConfiguration(newLimiter(10, 0, errors.New("storage down")))
	conf.UseIP = true

	w := doRequest(newRouter(conf), nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGinMiddleware_UseHeader(t *testing.T) {
	backend := newStubBackend(time.Minute, nil)
	limiter := yarl.New(backend, yarl.Rule{ID: "api", TTL: time.Minute, MaxRequests: 2})
	conf := &Configuration{limiter: limiter, Headers: []string{"X-User-ID"}}

	r := newRouter(conf)
	doRequest(r, map[string]string{"X-User-Id": "alice"})
	doRequest(r, map[string]string{"X-User-Id": "alice"})
	w := doRequest(r, map[string]string{"X-User-Id": "alice"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	w = doRequest(r, map[string]string{"X-User-Id": "bob"})
	assert.Equal(t, http.StatusOK, w.Code)
}
