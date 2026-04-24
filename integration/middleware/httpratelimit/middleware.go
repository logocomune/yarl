// Package httpratelimit provides rate-limiting middleware for the standard net/http package.
//
// Wrap any [http.HandlerFunc] with [New] to enforce rate limits based on the client
// IP, arbitrary request headers, or a combination of both.
// When a request violates any rule the middleware responds with HTTP 429 and a JSON
// body listing each violated rule with its retry window.
package httpratelimit

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	yarl "github.com/logocomune/yarl/v4"
)

// Configuration holds middleware settings.
// Create one with [NewConfiguration], then set UseIP and Headers as needed.
type Configuration struct {
	limiter *yarl.Limiter
	// UseIP includes the client IP in the rate-limit key.
	UseIP bool
	// Headers lists request header names appended to the key (e.g. "X-User-ID").
	Headers []string
}

// NewConfiguration creates a Configuration backed by limiter.
func NewConfiguration(limiter *yarl.Limiter) *Configuration {
	return &Configuration{limiter: limiter}
}

// New wraps h with rate-limiting logic defined by conf.
// Requests that violate any rule are rejected with HTTP 429 before h is called.
func New(conf *Configuration, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := buildKey(r, conf)

		results, err := conf.limiter.Check(r.Context(), key)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if violations := collectViolations(results); len(violations) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"violations": violations})
			return
		}

		h.ServeHTTP(w, r)
	}
}

type violationDTO struct {
	ID                string    `json:"id"`
	RetryAfterSeconds int64     `json:"retry_after_seconds"`
	ResetsAt          time.Time `json:"resets_at"`
}

func collectViolations(results []yarl.RuleResult) []violationDTO {
	var out []violationDTO
	for _, res := range results {
		if !res.Allowed {
			out = append(out, violationDTO{
				ID:                res.ID,
				RetryAfterSeconds: int64(res.RetryAfter / time.Second),
				ResetsAt:          res.ExpiresAt,
			})
		}
	}
	return out
}

func buildKey(r *http.Request, conf *Configuration) string {
	var sb strings.Builder
	if conf.UseIP {
		sb.WriteString(getIP(r))
	}
	for _, h := range conf.Headers {
		sb.WriteByte(':')
		sb.WriteString(strings.ToLower(r.Header.Get(h)))
	}
	return sb.String()
}

// getIP extracts the client IP from the request, preferring X-Forwarded-For.
func getIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

