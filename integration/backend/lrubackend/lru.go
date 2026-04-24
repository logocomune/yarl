// Package lrubackend provides an in-memory LRU backend for YARL with per-entry TTL.
//
// Because [expirable.LRU] is constructed with a single global TTL, one LRU
// instance is created per [yarl.Rule]. Rules with different window durations
// therefore each get their own correctly-configured cache.
package lrubackend

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	yarl "github.com/logocomune/yarl/v4"
)

type entry struct {
	count     int64
	expiresAt time.Time
}

// LRUBackend is a thread-safe in-memory rate-limit backend.
// Create one with [New].
type LRUBackend struct {
	mu   sync.Mutex
	lrus map[string]*expirable.LRU[string, *entry]
}

// New creates an LRUBackend.
// rules must match those passed to [yarl.New] so that one [expirable.LRU] is
// pre-created per rule with the correct TTL.
// sizePerRule is the maximum number of distinct user keys tracked per rule;
// total memory is roughly numRules × sizePerRule × entrySize.
func New(rules []yarl.Rule, sizePerRule int) *LRUBackend {
	lrus := make(map[string]*expirable.LRU[string, *entry], len(rules))
	for _, r := range rules {
		lrus[r.ID] = expirable.NewLRU[string, *entry](sizePerRule, nil, r.TTL)
	}
	return &LRUBackend{lrus: lrus}
}

// IncAndGetTTL increments the counter for key and returns the new value and
// remaining window duration. key must have the format "{ruleID}:{userKey}".
func (l *LRUBackend) IncAndGetTTL(_ context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	ruleID, userKey := splitKey(key)
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	cache := l.lrus[ruleID]

	e, ok := cache.Get(userKey)
	if !ok {
		e = &entry{count: 1, expiresAt: now.Add(ttl)}
		cache.Add(userKey, e)
		return 1, ttl, nil
	}

	e.count++
	return e.count, e.expiresAt.Sub(now), nil
}

// splitKey splits "{ruleID}:{userKey}" on the first colon.
func splitKey(key string) (ruleID, userKey string) {
	ruleID, userKey, _ = strings.Cut(key, ":")
	return
}
