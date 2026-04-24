// Example: rate limiting with a Redis Sentinel backend.
//
// Runs an HTTP server on :8080 backed by a Redis Sentinel cluster.
// Sentinel provides automatic failover: if the master goes down,
// Sentinel promotes a replica and YARL reconnects transparently.
//
// Rules:
//   - 60 req / min per IP
//   - 1000 req / hour per IP
//
// Try:
//
//	curl -i http://localhost:8080/ping
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/logocomune/yarl/v4"
	"github.com/logocomune/yarl/v4/integration/backend/redisbackend"
	"github.com/logocomune/yarl/v4/integration/middleware/httpratelimit"
)

func main() {
	backend := redisbackend.NewSentinel(
		"mymaster",                                       // Sentinel master name
		[]string{"sentinel1:26379", "sentinel2:26379"},  // Sentinel node addresses
		0,                                                // Redis DB
	)
	defer backend.Close()

	rules := []yarl.Rule{
		{ID: "per-ip-minute", TTL: time.Minute, MaxRequests: 60},
		{ID: "per-ip-hour",   TTL: time.Hour,   MaxRequests: 1000},
	}

	limiter := yarl.New(backend, rules...)
	// Both rules are evaluated in one Redis pipeline round-trip.

	conf := httpratelimit.NewConfiguration(limiter)
	conf.UseIP = true

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", httpratelimit.New(conf, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "pong")
	}))

	log.Println("listening on :8080 (Redis Sentinel)")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
