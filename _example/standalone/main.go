// Example: multi-rule rate limiting with a Redis standalone backend.
//
// Runs an HTTP server on :8080 backed by Redis at localhost:6379.
// Rules:
//   - 10 req / 10 s  per IP  (burst)
//   - 100 req / min  per IP  (sustained)
//   - 500 req / hour per user (identified by X-User-ID header)
//
// Try:
//
//	curl -i -H "X-User-ID: alice" http://localhost:8080/api
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/logocomune/yarl/v4"
	"github.com/logocomune/yarl/v4/integration/backend/redisbackend"
	"github.com/logocomune/yarl/v4/integration/middleware/httpratelimit"
)

func main() {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	rules := []yarl.Rule{
		{ID: "ip-burst",    TTL: 10 * time.Second, MaxRequests: 10},
		{ID: "ip-sustained", TTL: time.Minute,      MaxRequests: 100},
		{ID: "user-hour",   TTL: time.Hour,         MaxRequests: 500},
	}

	backend := redisbackend.NewFromClient(client)
	// All rules are evaluated in a single Redis pipeline round-trip (BatchBackend).

	ipLimiter := yarl.New(backend,
		rules[0], // burst
		rules[1], // sustained
	)
	userLimiter := yarl.New(backend, rules[2])

	ipConf := httpratelimit.NewConfiguration(ipLimiter)
	ipConf.UseIP = true

	userConf := httpratelimit.NewConfiguration(userLimiter)
	userConf.Headers = []string{"X-User-ID"}

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		// Chain: check IP limits first, then per-user limit.
		httpratelimit.New(ipConf,
			httpratelimit.New(userConf, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintln(w, "ok")
			}),
		).ServeHTTP(w, r)
	})

	log.Println("listening on :8080 (Redis standalone)")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
