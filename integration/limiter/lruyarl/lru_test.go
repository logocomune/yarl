package lruyarl

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	l, err := New(10)
	assert.NotNil(t, l)
	assert.NoError(t, err)

	l, err = New(0)
	assert.Nil(t, l)
	assert.Error(t, err)
}

func TestLRU_Inc(t *testing.T) {
	size := 10
	l, err := New(size)
	assert.NoError(t, err)

	// Test Increment for key1
	val, err := l.Inc("key1", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	val, err = l.Inc("key1", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// Test Increment for key2
	val, err = l.Inc("key2", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// Verify key1 is still tracked
	val, err = l.Inc("key1", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), val)
}

// TestLRU_Inc_Concurrent verifies that concurrent increments are thread-safe.
func TestLRU_Inc_Concurrent(t *testing.T) {
	l, err := New(100)
	assert.NoError(t, err)

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, incErr := l.Inc("concurrent_key", 10)
			assert.NoError(t, incErr)
		}()
	}

	wg.Wait()

	// One final increment should give exactly goroutines+1
	val, err := l.Inc("concurrent_key", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(goroutines+1), val)
}

// TestLRU_Inc_Eviction verifies that the LRU evicts least-recently-used entries
// when the cache is full.
func TestLRU_Inc_Eviction(t *testing.T) {
	// Size of 2: only 2 entries fit
	l, err := New(2)
	assert.NoError(t, err)

	val, err := l.Inc("key1", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	val, err = l.Inc("key2", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// Adding key3 evicts key1 (the least recently used)
	val, err = l.Inc("key3", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// key1 was evicted: its counter resets to 1
	val, err = l.Inc("key1", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)
}

// TestLRU_Inc_TTLIgnored verifies that the TTL parameter is silently ignored
// (the LRU backend manages expiry via cache size, not TTL).
func TestLRU_Inc_TTLIgnored(t *testing.T) {
	l, err := New(10)
	assert.NoError(t, err)

	// Different TTL values should not affect the counter logic
	val1, err := l.Inc("key", 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val1)

	val2, err := l.Inc("key", 9999)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), val2)
}
