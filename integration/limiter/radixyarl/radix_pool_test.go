package radixyarl

import (
	"testing"

	"github.com/mediocregopher/radix/v3"
	"github.com/stretchr/testify/assert"
)

func TestPool_Inc(t *testing.T) {
	// Try to connect to Redis
	client, err := radix.NewPool("tcp", "localhost:6379", 1)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	defer client.Close()

	// Check if we can actually ping
	if err := client.Do(radix.Cmd(nil, "PING")); err != nil {
		t.Skipf("Redis not reachable: %v", err)
	}

	p := New(client)

	key := "test_radix_inc"
	// Clean up before test
	client.Do(radix.Cmd(nil, "DEL", key))

	val, err := p.Inc(key, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)

	val, err = p.Inc(key, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), val)
}
