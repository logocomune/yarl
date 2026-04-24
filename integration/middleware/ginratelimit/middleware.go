// Package ginratelimit provides rate-limiting middleware for the Gin web framework.
//
// Register the handler returned by [New] with router.Use() to enforce rate limits.
// When a request violates any rule the middleware aborts with HTTP 429 and a JSON
// body listing each violated rule with its retry window.
package ginratelimit

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	yarl "github.com/logocomune/yarl/v4"
)

// Configuration holds middleware settings.
// Create one with [NewConfiguration], then set UseIP and Headers as needed.
type Configuration struct {
	limiter *yarl.Limiter
	// UseIP includes the client IP (from c.ClientIP()) in the rate-limit key.
	UseIP bool
	// Headers lists request header names appended to the key (e.g. "X-Tenant-ID").
	Headers []string
}

// NewConfiguration creates a Configuration backed by limiter.
func NewConfiguration(limiter *yarl.Limiter) *Configuration {
	return &Configuration{limiter: limiter}
}

// New returns a gin.HandlerFunc that enforces rate limits defined by conf.
// Requests that violate any rule are aborted with HTTP 429 before c.Next() is called.
func New(conf *Configuration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := buildKey(c, conf)

		results, err := conf.limiter.Check(c.Request.Context(), key)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		if violations := collectViolations(results); len(violations) > 0 {
			c.Header("Content-Type", "application/json")
			body, _ := json.Marshal(map[string]any{"violations": violations})
			c.AbortWithStatusJSON(http.StatusTooManyRequests, json.RawMessage(body))
			return
		}

		c.Next()
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

func buildKey(c *gin.Context, conf *Configuration) string {
	var sb strings.Builder
	if conf.UseIP {
		sb.WriteString(c.ClientIP())
	}
	for _, h := range conf.Headers {
		sb.WriteByte(':')
		sb.WriteString(strings.ToLower(c.GetHeader(h)))
	}
	return sb.String()
}
