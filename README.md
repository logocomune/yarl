# YARL — Yet Another Rate Limiter

[![Build Status](https://travis-ci.org/logocomune/yarl.svg?branch=master)](https://travis-ci.org/logocomune/yarl)
[![Go Report Card](https://goreportcard.com/badge/github.com/logocomune/yarl/v2)](https://goreportcard.com/report/github.com/logocomune/yarl/v2)
[![codecov](https://codecov.io/gh/logocomune/yarl/branch/master/graph/badge.svg)](https://codecov.io/gh/logocomune/yarl)
[![Go Reference](https://pkg.go.dev/badge/github.com/logocomune/yarl/v2.svg)](https://pkg.go.dev/github.com/logocomune/yarl/v2)

YARL is a Go library that implements **time-window rate limiting** with pluggable storage backends. It can limit any operation — HTTP requests, API calls, database writes — by counting how many times a given key has been used within a configurable time window.

---

## Features

- **Time-window based** — limits reset automatically at the end of each window (e.g. 100 req/minute)
- **Pluggable backends** — in-memory LRU cache or Redis (via Radix, Redigo, or go-redis)
- **Distributed-ready** — use Redis backends to share rate-limit counters across multiple instances
- **Flexible key** — limit by IP address, request headers, user ID, or any combination
- **Standard HTTP headers** — middleware sets `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `Retry-After`
- **Framework integrations** — drop-in middleware for the standard `net/http` package and the [Gin](https://github.com/gin-gonic/gin) framework

---

## Installation

```bash
go get github.com/logocomune/yarl/v2
```

---

## Quick Start

### Core library

```go
package main

import (
    "fmt"
    "time"

    "github.com/logocomune/yarl/v2"
    "github.com/logocomune/yarl/v2/integration/limiter/lruyarl"
)

func main() {
    // Create an in-memory LRU backend (1 000 tracked keys max)
    backend, err := lruyarl.New(1000)
    if err != nil {
        panic(err)
    }

    // Allow 5 operations per 10 seconds, namespaced under "myapp"
    limiter := yarl.New("myapp", backend, 5, 10*time.Second)

    for i := 1; i <= 7; i++ {
        resp, err := limiter.IsAllow("user:42")
        if err != nil {
            panic(err)
        }
        if resp.IsAllowed {
            fmt.Printf("Request %d: ALLOWED  (remaining: %d)\n", i, resp.Remain)
        } else {
            fmt.Printf("Request %d: DENIED   (retry after %ds)\n", i, resp.RetryAfter)
        }
    }
}
```

**Output** (first 5 requests are allowed, the next 2 are denied):

```
Request 1: ALLOWED  (remaining: 4)
Request 2: ALLOWED  (remaining: 3)
Request 3: ALLOWED  (remaining: 2)
Request 4: ALLOWED  (remaining: 1)
Request 5: ALLOWED  (remaining: 0)
Request 6: DENIED   (retry after 8s)
Request 7: DENIED   (retry after 7s)
```

---

## Backends

### In-Memory LRU (single process)

```go
import "github.com/logocomune/yarl/v2/integration/limiter/lruyarl"

backend, err := lruyarl.New(1000) // track up to 1 000 keys
```

The LRU cache is bounded: when full it evicts the least recently used key. Suitable for single-process deployments where counters can be lost on restart.

---

### Redis — go-redis (recommended)

```go
import (
    "github.com/redis/go-redis/v9"
    goredisynarl "github.com/logocomune/yarl/v2/integration/limiter/goredisyarl"
)

client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
backend := goredisynarl.NewPool(client)
```

---

### Redis — Radix

```go
import (
    "github.com/mediocregopher/radix/v3"
    "github.com/logocomune/yarl/v2/integration/limiter/radixyarl"
)

pool, err := radix.NewPool("tcp", "localhost:6379", 10)
backend := radixyarl.New(pool)
```

---

### Redis — Redigo

```go
import (
    "github.com/gomodule/redigo/redis"
    redigoyarl "github.com/logocomune/yarl/v2/integration/limiter/redigoyarl"
)

pool := &redis.Pool{
    Dial: func() (redis.Conn, error) {
        return redis.Dial("tcp", "localhost:6379")
    },
}
backend := redigoyarl.NewPool(pool, 0) // 0 = database index
```

---

## HTTP Middleware (`net/http`)

The `httpratelimit` package wraps any `http.HandlerFunc`:

```go
package main

import (
    "net/http"
    "time"

    "github.com/logocomune/yarl/v2/integration/httpratelimit"
)

func main() {
    // 100 requests per minute, in-memory LRU, limit by client IP
    conf := httpratelimit.NewConfigurationWithLru("api", 10000, 100, time.Minute)
    conf.UseIP = true

    handler := httpratelimit.New(conf, func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello!"))
    })

    http.ListenAndServe(":8080", handler)
}
```

### Rate limit by IP + custom header

```go
conf := httpratelimit.NewConfigurationWithLru("api", 10000, 100, time.Minute)
conf.UseIP = true
conf.Headers = []string{"X-Tenant-ID"} // separate bucket per tenant
```

### Using a Redis backend (Radix)

```go
conf := httpratelimit.NewConfigurationWithRadix(
    "api",         // prefix
    10,            // pool size
    "localhost",   // Redis host
    "6379",        // Redis port
    0,             // Redis DB
    100,           // max requests
    time.Minute,   // window
)
conf.UseIP = true
```

### Response headers set by the middleware

| Header | Description |
|---|---|
| `X-RateLimit-Limit` | Maximum requests allowed in the window |
| `X-RateLimit-Remaining` | Requests remaining in the current window |
| `X-RateLimit-Reset` | Unix timestamp when the current window resets |
| `Retry-After` | Seconds until the next window (only on HTTP 429) |

---

## Gin Middleware

```go
package main

import (
    "time"

    "github.com/gin-gonic/gin"
    "github.com/logocomune/yarl/v2"
    "github.com/logocomune/yarl/v2/integration/ginratelimit"
    "github.com/logocomune/yarl/v2/integration/limiter/lruyarl"
)

func main() {
    backend, _ := lruyarl.New(10000)
    limiter := yarl.New("api", backend, 100, time.Minute)

    conf := &ginratelimit.Configuration{
        UseIP:   true,
        Headers: []string{"X-Tenant-ID"},
    }
    // Inject the limiter via the embedded Yarl field (or use NewConfigurationWithRadix)
    _ = limiter // assign to conf.y via NewConfigurationWithRadix or direct struct

    r := gin.Default()
    r.Use(ginratelimit.New(conf))

    r.GET("/ping", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "pong"})
    })

    r.Run(":8080")
}
```

Or use the built-in Redis helper:

```go
conf := ginratelimit.NewConfigurationWithRadix(
    "api",       // prefix
    "localhost", // Redis host
    10,          // pool size
    0,           // Redis DB
    100,         // max requests
    time.Minute, // window
)
conf.UseIP = true
r.Use(ginratelimit.New(conf))
```

---

## API Reference

### `yarl.New`

```go
func New(prefix string, l Limiter, max int64, timeWindow time.Duration) Yarl
```

Creates a new rate limiter.

| Parameter | Description |
|---|---|
| `prefix` | Namespace for cache keys (avoids collisions when sharing a backend) |
| `l` | Storage backend implementing the `Limiter` interface |
| `max` | Maximum number of operations allowed per `timeWindow` |
| `timeWindow` | Duration of each rate-limit window |

### `Yarl.IsAllow`

```go
func (y *Yarl) IsAllow(key string) (*Resp, error)
```

Checks whether the operation identified by `key` is within the configured limit. Returns `*Resp` on success.

### `Yarl.IsAllowWithLimit`

```go
func (y *Yarl) IsAllowWithLimit(key string, max int64, tWindow time.Duration) (*Resp, error)
```

Same as `IsAllow` but overrides `max` and `tWindow` for this specific call. Useful when different callers need different limits on a shared `Yarl` instance.

### `Resp` fields

| Field | Type | Description |
|---|---|---|
| `IsAllowed` | `bool` | `true` if the request is within the limit |
| `Current` | `int64` | Counter value after this request |
| `Max` | `int64` | Configured limit |
| `Remain` | `int64` | Requests remaining in the current window |
| `NextReset` | `int64` | Unix timestamp of the next window reset |
| `RetryAfter` | `int64` | Seconds until the next reset |

### `Limiter` interface

```go
type Limiter interface {
    Inc(key string, ttlSeconds int64) (int64, error)
}
```

Implement this interface to plug in a custom backend (e.g. Memcached, DynamoDB).

---

## Custom Backend Example

```go
type MyBackend struct{ /* ... */ }

func (b *MyBackend) Inc(key string, ttlSeconds int64) (int64, error) {
    // atomically increment counter for key, set TTL, return new value
    return newValue, nil
}

limiter := yarl.New("prefix", &MyBackend{}, 100, time.Minute)
```

---

## Test Coverage

| Package | Coverage |
|---|---|
| `yarl` (core) | **100%** |
| `lruyarl` | **100%** |
| `radixyarl` | 83% |
| `goredisyarl` | 82% |
| `redigoyarl` | 80% |
| `ginratelimit` | 79% |
| `httpratelimit` | 77% |

> Redis-backend packages have lower coverage because the `NewConfigurationWithRadix` factory functions require a live Redis server and are not exercised in unit tests.

---

## License

MIT
