package lrubackend

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/quick"
	"time"

	yarl "github.com/logocomune/yarl/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rules(ttl time.Duration) []yarl.Rule {
	return []yarl.Rule{{ID: "r1", TTL: ttl, MaxRequests: 10}}
}

func TestNew(t *testing.T) {
	b := New(rules(time.Minute), 100)
	assert.NotNil(t, b)
	assert.Len(t, b.lrus, 1)
}

func TestLRUBackend_IncAndGetTTL_Increments(t *testing.T) {
	ctx := context.Background()
	b := New(rules(time.Minute), 100)

	count, _, err := b.IncAndGetTTL(ctx, "r1:user1", time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	count, _, err = b.IncAndGetTTL(ctx, "r1:user1", time.Minute)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestLRUBackend_IncAndGetTTL_IsolatedByUser(t *testing.T) {
	ctx := context.Background()
	b := New(rules(time.Minute), 100)

	count1, _, _ := b.IncAndGetTTL(ctx, "r1:user1", time.Minute)
	count2, _, _ := b.IncAndGetTTL(ctx, "r1:user2", time.Minute)

	assert.Equal(t, int64(1), count1)
	assert.Equal(t, int64(1), count2, "different users must have independent counters")
}

func TestLRUBackend_IncAndGetTTL_RemainingDecreases(t *testing.T) {
	ctx := context.Background()
	ttl := 200 * time.Millisecond
	b := New(rules(ttl), 100)

	_, rem1, _ := b.IncAndGetTTL(ctx, "r1:user1", ttl)
	time.Sleep(50 * time.Millisecond)
	_, rem2, _ := b.IncAndGetTTL(ctx, "r1:user1", ttl)

	assert.Less(t, rem2, rem1, "remaining TTL must decrease over time")
}

func TestLRUBackend_IncAndGetTTL_ExpiryResetsCounter(t *testing.T) {
	ctx := context.Background()
	ttl := 100 * time.Millisecond
	b := New(rules(ttl), 100)

	count, _, _ := b.IncAndGetTTL(ctx, "r1:user1", ttl)
	assert.Equal(t, int64(1), count)

	count, _, _ = b.IncAndGetTTL(ctx, "r1:user1", ttl)
	assert.Equal(t, int64(2), count)

	time.Sleep(150 * time.Millisecond)

	count, rem, _ := b.IncAndGetTTL(ctx, "r1:user1", ttl)
	assert.Equal(t, int64(1), count, "counter must reset after TTL expires")
	assert.InDelta(t, ttl.Milliseconds(), rem.Milliseconds(), 20, "remaining should be close to full ttl after reset")
}

func TestLRUBackend_IncAndGetTTL_Concurrent(t *testing.T) {
	ctx := context.Background()
	b := New(rules(time.Minute), 1000)

	const goroutines = 50
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			_, _, err := b.IncAndGetTTL(ctx, "r1:shared", time.Minute)
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	count, _, _ := b.IncAndGetTTL(ctx, "r1:shared", time.Minute)
	assert.Equal(t, int64(goroutines+1), count)
}

func TestLRUBackend_IncAndGetTTL_MultipleRulesDifferentTTLs(t *testing.T) {
	ctx := context.Background()
	ruleSet := []yarl.Rule{
		{ID: "fast", TTL: 100 * time.Millisecond, MaxRequests: 5},
		{ID: "slow", TTL: time.Hour, MaxRequests: 100},
	}
	b := New(ruleSet, 100)

	// Increment both rules for same user
	b.IncAndGetTTL(ctx, "fast:user1", 100*time.Millisecond)
	b.IncAndGetTTL(ctx, "slow:user1", time.Hour)

	time.Sleep(150 * time.Millisecond)

	// fast window expired; slow window still active
	fastCount, _, _ := b.IncAndGetTTL(ctx, "fast:user1", 100*time.Millisecond)
	slowCount, _, _ := b.IncAndGetTTL(ctx, "slow:user1", time.Hour)

	assert.Equal(t, int64(1), fastCount, "fast rule must reset after its short TTL")
	assert.Equal(t, int64(2), slowCount, "slow rule must keep its counter across fast window boundaries")
}

func TestSplitKey(t *testing.T) {
	tests := []struct {
		key         string
		wantRuleID  string
		wantUserKey string
	}{
		{"rule1:user1", "rule1", "user1"},
		{"rule1:user:with:colons", "rule1", "user:with:colons"},
		{"nocodon", "nocodon", ""},
	}
	for _, tt := range tests {
		ruleID, userKey := splitKey(tt.key)
		assert.Equal(t, tt.wantRuleID, ruleID)
		assert.Equal(t, tt.wantUserKey, userKey)
	}
}

// TestSplitKey_RoundTrip verifies that splitKey is the inverse of "{ruleID}:{userKey}"
// for any ruleID that does not itself contain a colon.
func TestSplitKey_RoundTrip(t *testing.T) {
	prop := func(ruleID, userKey string) bool {
		ruleID = strings.ReplaceAll(ruleID, ":", "_") // ruleID must not contain ":"
		gotRule, gotUser := splitKey(ruleID + ":" + userKey)
		return gotRule == ruleID && gotUser == userKey
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// FuzzSplitKey ensures splitKey never panics on arbitrary input and that
// the output can always reconstruct the original string.
func FuzzSplitKey(f *testing.F) {
	f.Add("rule:user")
	f.Add("rule:user:with:colons")
	f.Add("noruleidsep")
	f.Add("")
	f.Add(":")
	f.Add("::")
	f.Add("a:b:c:d")

	f.Fuzz(func(t *testing.T, key string) {
		ruleID, userKey := splitKey(key)

		// Reconstruction invariant: ruleID + sep + userKey == original key
		if strings.Contains(key, ":") {
			if ruleID+":"+userKey != key {
				t.Errorf("splitKey(%q) = (%q, %q): reconstruction mismatch", key, ruleID, userKey)
			}
		} else {
			if ruleID != key || userKey != "" {
				t.Errorf("splitKey(%q) = (%q, %q): expected (%q, \"\")", key, ruleID, userKey, key)
			}
		}
	})
}
