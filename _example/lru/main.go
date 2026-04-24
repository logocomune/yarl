// Example: multi-rule rate limiting with an in-memory LRU backend.
//
// Runs an HTTP server on :8080 with two endpoints:
//
//	/hello  — middleware handles 429 automatically
//	/check  — manual Check + Summarize for custom response logic
//
// Every request is evaluated against two rules:
//   - 5 requests per 10 seconds (burst protection)
//   - 20 requests per minute    (sustained rate)
//
// Try:
//
//	for i in $(seq 1 8); do curl -si http://localhost:8080/check | head -1; done
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/logocomune/yarl/v4"
	"github.com/logocomune/yarl/v4/integration/backend/lrubackend"
	"github.com/logocomune/yarl/v4/integration/middleware/httpratelimit"
)

func main() {
	rules := []yarl.Rule{
		{ID: "burst",     TTL: 10 * time.Second, MaxRequests: 5},
		{ID: "sustained", TTL: time.Minute,       MaxRequests: 20},
	}

	backend := lrubackend.New(rules, 10_000)
	limiter  := yarl.New(backend, rules...)

	mux := http.NewServeMux()

	// /hello — middleware handles 429 automatically (JSON violations body).
	conf := httpratelimit.NewConfiguration(limiter)
	conf.UseIP = true
	mux.HandleFunc("/hello", httpratelimit.New(conf, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello!")
	}))

	// /check — manual Check + Summarize for custom response logic.
	mux.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
		results, err := limiter.Check(r.Context(), r.RemoteAddr)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		allowed, worst := yarl.Summarize(results)
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"blocked_by":   worst.ID,
				"retry_after":  worst.RetryAfter.Round(time.Second).String(),
				"resets_at":    worst.ExpiresAt.UTC().Format(time.RFC3339),
			})
			return
		}

		fmt.Fprintln(w, "ok")
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
