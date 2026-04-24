# YARL — Yet Another Rate Limiter

[![Go Report Card](https://goreportcard.com/badge/github.com/logocomune/yarl/v4)](https://goreportcard.com/report/github.com/logocomune/yarl/v4)
[![Go Reference](https://pkg.go.dev/badge/github.com/logocomune/yarl/v4.svg)](https://pkg.go.dev/github.com/logocomune/yarl/v4)

YARL is a Go rate-limiting library with pluggable storage backends. Define one or more **rules** (max requests + window duration), call `Check` on each request with an identity key, and inspect per-rule results. All rules are evaluated in a **single Redis pipeline round-trip** when using the Redis backend.

---

## Features

- **Multi-rule evaluation** — define burst, sustained, and daily limits as separate rules; all are checked in one call
- **Single Redis round-trip** — all rules share one pipeline via `BatchBackend`; N rules do not mean N network calls
- **Stable Redis keys** — key format is `{ruleID}:{userKey}`; TTL is the window; no clock math, no time-bucket suffix
- **Redis Standalone + Sentinel** — `redis.UniversalClient` covers both; requires Redis ≥ 7.0
- **LRU with real TTL** — one `expirable.LRU` per rule; each rule's window is enforced independently
- **Flexible identity key** — limit by IP, request headers, user ID, or any combination
- **Framework integrations** — drop-in middleware for `net/http` and [Gin](https://github.com/gin-gonic/gin)

---

## Installation

```bash
go get github.com/logocomune/yarl/v4
```

---

## Core concepts

```
Rule          — one rate-limit policy: ID, TTL (window duration), MaxRequests
Limiter       — holds a fixed set of Rules; call Check(ctx, userKey) per request
RuleResult    — outcome for one Rule: Allowed, Current, Max, ExpiresAt, RetryAfter
Backend       — storage interface; implement to plug in any store
BatchBackend  — optional extension of Backend for single-round-trip multi-key evaluation
```

A **userKey** is any string that identifies who is being limited — a client IP, a user ID, a tenant, or a combination. The backend key is `{rule.ID}:{userKey}`.

---

## Quick start — in-memory LRU

The simplest setup: no external services, state is local to the process.

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/logocomune/yarl/v4"
    "github.com/logocomune/yarl/v4/integration/backend/lrubackend"
)

func main() {
    rules := []yarl.Rule{
        {ID: "burst",     TTL: 10 * time.Second, MaxRequests: 3},
        {ID: "sustained", TTL: time.Minute,      MaxRequests: 10},
    }

    backend := lrubackend.New(rules, 1000) // track up to 1 000 distinct users per rule
    limiter  := yarl.New(backend, rules...)

    ctx := context.Background()

    for i := 1; i <= 5; i++ {
        results, err := limiter.Check(ctx, "user:42")
        if err != nil {
            panic(err)
        }
        for _, r := range results {
            if r.Allowed {
                fmt.Printf("[%s] req %d: allowed  (count %d/%d)\n",
                    r.ID, i, r.Current, r.Max)
            } else {
                fmt.Printf("[%s] req %d: BLOCKED  (retry in %s, resets at %s)\n",
                    r.ID, i, r.RetryAfter.Round(time.Second), r.ExpiresAt.Format("15:04:05"))
            }
        }
    }
}
```

**Output** (first 3 pass burst, 4th blocked on burst but not sustained):

```
[burst] req 1: allowed  (count 1/3)
[sustained] req 1: allowed  (count 1/10)
[burst] req 2: allowed  (count 2/3)
[sustained] req 2: allowed  (count 2/10)
[burst] req 3: allowed  (count 3/3)
[sustained] req 3: allowed  (count 3/10)
[burst] req 4: BLOCKED  (retry in 9s, resets at 10:15:09)
[sustained] req 4: allowed  (count 4/10)
[burst] req 5: BLOCKED  (retry in 8s, resets at 10:15:09)
[sustained] req 5: allowed  (count 5/10)
```

---

## Backends

### In-memory LRU

```go
import "github.com/logocomune/yarl/v4/integration/backend/lrubackend"

rules := []yarl.Rule{
    {ID: "per-ip-minute", TTL: time.Minute, MaxRequests: 60},
    {ID: "per-ip-hour",   TTL: time.Hour,   MaxRequests: 1000},
}

// One expirable.LRU is created per rule, each with that rule's TTL.
// sizePerRule = max distinct user keys tracked per rule (e.g. concurrent IPs).
backend := lrubackend.New(rules, 10_000)
```

Different window durations coexist correctly: each rule has its own cache, sized and timed independently. No shared global expiry.

---

### Redis — standalone

```go
import (
    "github.com/redis/go-redis/v9"
    "github.com/logocomune/yarl/v4/integration/backend/redisbackend"
)

client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
defer client.Close()

backend := redisbackend.NewFromClient(client)
// caller owns the client lifecycle
```

Or let YARL manage the connection:

```go
backend := redisbackend.NewStandalone("localhost:6379", 0)
defer backend.Close()
```

---

### Redis — Sentinel (automatic failover)

```go
backend := redisbackend.NewSentinel(
    "mymaster",
    []string{"sentinel1:26379", "sentinel2:26379"},
    0,
)
defer backend.Close()
```

`NewSentinel` uses `redis.FailoverClient` under the hood. If the master fails, Sentinel promotes a replica and the client reconnects automatically.

---

### Single pipeline round-trip (BatchBackend)

`RedisBackend` implements `BatchBackend`. When `Limiter.Check` detects this, it packs all rules into **one pipeline** — N rules cost one network round-trip, not N.

```
Request with 3 rules → 1 pipeline: [INCR r1, ExpireNX r1, TTL r1, INCR r2, ExpireNX r2, TTL r2, INCR r3, ExpireNX r3, TTL r3]
                                                        └─────────────────── single Exec ──────────────────────────────┘
```

This is automatic — no configuration needed. The `LRU` backend does not implement `BatchBackend` (in-process loop is negligible).

---

### Custom backend

```go
type MyBackend struct{}

func (b *MyBackend) IncAndGetTTL(
    ctx context.Context, key string, ttl time.Duration,
) (count int64, remaining time.Duration, err error) {
    // atomically: INCR key, set expiry only on creation, return (count, remaining)
    return count, remaining, nil
}

limiter := yarl.New(&MyBackend{}, rules...)
```

To opt into the single-round-trip path, also implement `BatchBackend`:

```go
func (b *MyBackend) IncAndGetTTLBatch(
    ctx context.Context, entries []yarl.BatchEntry,
) ([]yarl.BatchResult, error) {
    // process all entries in one operation
}
```

`Limiter.Check` will use the batch path automatically.

---

## HTTP Middleware (`net/http`)

```go
import (
    "net/http"
    "time"

    "github.com/logocomune/yarl/v4"
    "github.com/logocomune/yarl/v4/integration/backend/lrubackend"
    "github.com/logocomune/yarl/v4/integration/middleware/httpratelimit"
)

func main() {
    rules := []yarl.Rule{
        {ID: "burst",     TTL: 10 * time.Second, MaxRequests: 10},
        {ID: "sustained", TTL: time.Minute,      MaxRequests: 100},
    }
    limiter := yarl.New(lrubackend.New(rules, 10_000), rules...)

    conf := httpratelimit.NewConfiguration(limiter)
    conf.UseIP = true                            // limit by client IP
    // conf.Headers = []string{"X-Tenant-ID"}   // add header value to identity key

    http.ListenAndServe(":8080", httpratelimit.New(conf, myHandler))
}
```

### Identity key composition

The middleware builds `userKey` by concatenating the enabled components:

| `UseIP` | `Headers` | `userKey` example |
|---------|-----------|-------------------|
| `true`  | —         | `203.0.113.5` |
| `false` | `["X-User-ID"]` | `:alice:` |
| `true`  | `["X-Tenant-ID"]` | `203.0.113.5:acme:` |

### HTTP 429 response

When any rule is violated the middleware returns `429 Too Many Requests` with a JSON body listing every violated rule:

```json
{
  "violations": [
    {
      "id": "burst",
      "retry_after_seconds": 8,
      "resets_at": "2026-04-24T10:15:09Z"
    }
  ]
}
```

All rules are always evaluated — a response may contain multiple violations.

---

## Gin Middleware

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/logocomune/yarl/v4/integration/middleware/ginratelimit"
)

conf := ginratelimit.NewConfiguration(limiter)
conf.UseIP = true

r := gin.Default()
r.Use(ginratelimit.New(conf))
r.GET("/api", myHandler)
r.Run(":8080")
```

---

## API Reference

### `yarl.Rule`

| Field | Type | Description |
|---|---|---|
| `ID` | `string` | Key namespace; unique per `Limiter`. Backend key: `{ID}:{userKey}` |
| `TTL` | `time.Duration` | Window duration and Redis key expiry |
| `MaxRequests` | `int64` | Allowed requests per window |

### `yarl.New`

```go
func New(b Backend, rules ...Rule) *Limiter
```

Rules are fixed for the lifetime of the `Limiter`.

### `Limiter.Check`

```go
func (l *Limiter) Check(ctx context.Context, userKey string) ([]RuleResult, error)
```

Evaluates every rule. All rules are always checked — no short-circuit on first violation. Uses a single pipeline round-trip when the backend implements `BatchBackend`.

### `yarl.Summarize`

```go
func Summarize(results []RuleResult) (allowed bool, worst *RuleResult)
```

Convenience helper: returns whether all rules passed and, if not, the violated rule with the **furthest `ExpiresAt`** — the worst case for the caller to retry. Returns `(true, nil)` when all rules are allowed.

```go
results, err := limiter.Check(ctx, userKey)
if err != nil { /* ... */ }

allowed, worst := yarl.Summarize(results)
if !allowed {
    log.Printf("blocked by rule %q, retry in %s", worst.ID, worst.RetryAfter.Round(time.Second))
}
```

Use `Check` directly when you need per-rule detail (e.g. to set multiple response headers). Use `Summarize` when you only need a single go/no-go decision and the worst-case retry window.

### `yarl.RuleResult`

| Field | Type | Description |
|---|---|---|
| `ID` | `string` | Rule ID |
| `Allowed` | `bool` | `true` when `Current ≤ Max` |
| `Current` | `int64` | Counter value after this increment |
| `Max` | `int64` | Copy of `Rule.MaxRequests` |
| `ExpiresAt` | `time.Time` | When the current window resets |
| `RetryAfter` | `time.Duration` | > 0 only when `Allowed == false` |

### `yarl.Backend`

```go
type Backend interface {
    IncAndGetTTL(ctx context.Context, key string, ttl time.Duration) (int64, time.Duration, error)
}
```

### `yarl.BatchBackend`

```go
type BatchBackend interface {
    Backend
    IncAndGetTTLBatch(ctx context.Context, entries []BatchEntry) ([]BatchResult, error)
}
```

Implement to process multiple keys in a single round-trip. `Limiter.Check` detects and uses it automatically.

---

## Redis key schema

```
{Rule.ID}:{userKey}
```

| Rule.ID | userKey | Redis key |
|---|---|---|
| `burst` | `203.0.113.5` | `burst:203.0.113.5` |
| `per-user-hour` | `user:42` | `per-user-hour:user:42` |
| `per-tenant-day` | `acme` | `per-tenant-day:acme` |

No time component in the key. One key per `(rule, identity)`. When Redis expires the key the next request recreates it with a fresh TTL.

---

## Examples

Runnable examples are in [`_example/`](./_example):

| Directory | Backend | Description |
|---|---|---|
| [`lru/`](./_example/lru) | In-memory LRU | HTTP server with middleware + manual `Check`/`Summarize` |
| [`standalone/`](./_example/standalone) | Redis standalone | Multi-limiter chaining (per-IP + per-user) |
| [`sentinel/`](./_example/sentinel) | Redis Sentinel | Automatic failover setup |

---

## Requirements

- Go ≥ 1.25
- Redis ≥ 7.0 (for `EXPIRE NX`)

---

## License

MIT
