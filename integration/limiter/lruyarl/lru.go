package lruyarl

import (
	"sync"

	"github.com/hashicorp/golang-lru"
)

type LRU struct {
	cache *lru.Cache
	sync.RWMutex
}

func New(size int) (*LRU, error) {
	cache, err := lru.New(size)
	if err != nil {
		return nil, err
	}

	return &LRU{cache: cache}, nil
}

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
