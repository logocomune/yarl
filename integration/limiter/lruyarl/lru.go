// Package lruyarl provides an in-memory LRU cache backend for YARL.
//
// It is suitable for single-process deployments where a distributed store is not
// available or needed. The cache is bounded by a fixed number of entries; when full
// it evicts the least recently used key.
package lruyarl

import (
	"sync"

	"github.com/hashicorp/golang-lru"
)

// LRU is a thread-safe in-memory rate-limit backend backed by a Least-Recently-Used
// (LRU) cache. Create one with [New].
type LRU struct {
	cache *lru.Cache
	sync.RWMutex
}

// New creates a new LRU backend with the given maximum number of cached entries.
// Returns an error if size is less than 1.
func New(size int) (*LRU, error) {
	cache, err := lru.New(size)
	if err != nil {
		return nil, err
	}

	return &LRU{cache: cache}, nil
}

// Inc atomically increments the counter for key and returns the new value.
// The ttlSeconds parameter is accepted for interface compatibility but is ignored;
// expiry is handled implicitly by LRU eviction when the cache is full.
func (l *LRU) Inc(key string, _ int64) (int64, error) {
	curr := 0

	l.Lock()
	defer l.Unlock()

	val, ok := l.cache.Get(key)

	if ok {
		curr, _ = val.(int)
	}
	curr++
	l.cache.Add(key, curr)

	return int64(curr), nil
}
