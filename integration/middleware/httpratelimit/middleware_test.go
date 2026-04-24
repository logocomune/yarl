package httpratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	yarl "github.com/logocomune/yarl/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBackend tracks per-key counters for middleware tests.
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

func newLimiter(max int64, remaining time.Duration, err error) *yarl.Limiter {
	return yarl.New(
		newStubBackend(remaining, err),
		yarl.Rule{ID: "test", TTL: time.Minute, MaxRequests: max},
	)
}

func doRequest(h http.Handler, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestMiddleware_AllowedRequest(t *testing.T) {
	conf := NewConfiguration(newLimiter(10, time.Minute, nil))
	conf.UseIP = true

	h := New(conf, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := doRequest(h, map[string]string{"X-Forwarded-For": "1.2.3.4"})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddleware_BlockedRequest_Returns429(t *testing.T) {
	conf := NewConfiguration(newLimiter(1, 30*time.Second, nil))
	conf.UseIP = true

	h := New(conf, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First call allowed; second exceeds max=1
	doRequest(h, map[string]string{"X-Forwarded-For": "1.2.3.4"})
	w := doRequest(h, map[string]string{"X-Forwarded-For": "1.2.3.4"})

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	violations, ok := body["violations"].([]any)
	require.True(t, ok)
	assert.Len(t, violations, 1)

	v := violations[0].(map[string]any)
	assert.Equal(t, "test", v["id"])
	assert.Greater(t, v["retry_after_seconds"], float64(0))
}

func TestMiddleware_BackendError_Returns500(t *testing.T) {
	conf := NewConfiguration(newLimiter(10, 0, errors.New("storage down")))
	conf.UseIP = true

	h := New(conf, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := doRequest(h, map[string]string{"X-Forwarded-For": "1.2.3.4"})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestMiddleware_UseHeader(t *testing.T) {
	// User A and User B share no bucket when keyed by header
	backend := newStubBackend(time.Minute, nil)
	limiter := yarl.New(backend, yarl.Rule{ID: "api", TTL: time.Minute, MaxRequests: 2})
	conf := &Configuration{limiter: limiter, Headers: []string{"X-User-ID"}}

	h := New(conf, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// User A — 2 allowed, 3rd blocked
	doRequest(h, map[string]string{"X-User-Id": "alice"})
	doRequest(h, map[string]string{"X-User-Id": "alice"})
	w := doRequest(h, map[string]string{"X-User-Id": "alice"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// User B — own bucket, first call allowed
	w = doRequest(h, map[string]string{"X-User-Id": "bob"})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMiddleware_NoIPNoHeaders_SharedBucket(t *testing.T) {
	conf := NewConfiguration(newLimiter(2, time.Minute, nil))
	// UseIP=false, no headers → all requests share one bucket

	h := New(conf, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	doRequest(h, nil)
	doRequest(h, nil)
	w := doRequest(h, nil)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestGetIP(t *testing.T) {
	tests := []struct {
		name       string
		xForwarded string
		remoteAddr string
		want       string
	}{
		{"XFF takes precedence", "203.0.113.1", "10.0.0.1:1234", "203.0.113.1"},
		{"XFF first element", "203.0.113.1, 10.0.0.1", "10.0.0.1:1234", "203.0.113.1"},
		{"RemoteAddr IPv4", "", "192.168.1.1:5678", "192.168.1.1"},
		{"RemoteAddr IPv6", "", "[::1]:8080", "::1"},
		{"RemoteAddr no port", "", "192.168.1.1", "192.168.1.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.xForwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwarded)
			}
			req.RemoteAddr = tt.remoteAddr
			assert.Equal(t, tt.want, getIP(req))
		})
	}
}
