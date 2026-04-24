package yarl

import (
	"context"
	"errors"
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBackend struct {
	counts    map[string]int64
	remaining time.Duration
	err       error
}

func newMockBackend(remaining time.Duration, err error) *mockBackend {
	return &mockBackend{counts: make(map[string]int64), remaining: remaining, err: err}
}

func (m *mockBackend) IncAndGetTTL(_ context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	if m.err != nil {
		return 0, 0, m.err
	}
	m.counts[key]++
	rem := m.remaining
	if rem == 0 {
		rem = ttl
	}
	return m.counts[key], rem, nil
}

func TestNew(t *testing.T) {
	b := newMockBackend(time.Minute, nil)
	rule := Rule{ID: "r1", TTL: time.Minute, MaxRequests: 10}
	l := New(b, rule)
	assert.NotNil(t, l)
	assert.Len(t, l.rules, 1)
}

func TestLimiter_Check(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		rules         []Rule
		backend       *mockBackend
		calls         int
		wantAllowed   []bool
		wantCurrent   []int64
		wantErr       bool
	}{
		{
			name:        "single rule under limit",
			rules:       []Rule{{ID: "r1", TTL: time.Minute, MaxRequests: 5}},
			backend:     newMockBackend(time.Minute, nil),
			calls:       1,
			wantAllowed: []bool{true},
			wantCurrent: []int64{1},
		},
		{
			name:        "single rule at limit",
			rules:       []Rule{{ID: "r1", TTL: time.Minute, MaxRequests: 1}},
			backend:     newMockBackend(time.Minute, nil),
			calls:       1,
			wantAllowed: []bool{true},
			wantCurrent: []int64{1},
		},
		{
			name:        "single rule over limit",
			rules:       []Rule{{ID: "r1", TTL: time.Minute, MaxRequests: 1}},
			backend:     newMockBackend(30*time.Second, nil),
			calls:       2,
			wantAllowed: []bool{false},
			wantCurrent: []int64{2},
		},
		{
			name: "multiple rules both allowed",
			rules: []Rule{
				{ID: "per-min", TTL: time.Minute, MaxRequests: 10},
				{ID: "per-hour", TTL: time.Hour, MaxRequests: 100},
			},
			backend:     newMockBackend(time.Minute, nil),
			calls:       1,
			wantAllowed: []bool{true, true},
			wantCurrent: []int64{1, 1},
		},
		{
			name:    "backend error propagates",
			rules:   []Rule{{ID: "r1", TTL: time.Minute, MaxRequests: 5}},
			backend: newMockBackend(0, errors.New("storage failure")),
			calls:   1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(tt.backend, tt.rules...)

			var results []RuleResult
			var err error
			for i := 0; i < tt.calls; i++ {
				results, err = l.Check(ctx, "user:1")
			}

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, results, len(tt.rules))

			for i, res := range results {
				assert.Equal(t, tt.wantAllowed[i], res.Allowed, "rule %d Allowed", i)
				assert.Equal(t, tt.wantCurrent[i], res.Current, "rule %d Current", i)
				assert.Equal(t, tt.rules[i].MaxRequests, res.Max, "rule %d Max", i)
				assert.Equal(t, tt.rules[i].ID, res.ID, "rule %d ID", i)
				assert.False(t, res.ExpiresAt.IsZero(), "rule %d ExpiresAt should be set", i)
				if !res.Allowed {
					assert.Greater(t, res.RetryAfter, time.Duration(0), "rule %d RetryAfter should be > 0 when blocked", i)
				}
			}
		})
	}
}

func TestLimiter_Check_KeyFormat(t *testing.T) {
	ctx := context.Background()
	captured := &keyCapturingBackend{}
	rule := Rule{ID: "my-rule", TTL: time.Minute, MaxRequests: 10}
	l := New(captured, rule)

	_, err := l.Check(ctx, "user:42")
	require.NoError(t, err)
	assert.Equal(t, "my-rule:user:42", captured.lastKey)
}

type keyCapturingBackend struct {
	lastKey string
}

func (k *keyCapturingBackend) IncAndGetTTL(_ context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	k.lastKey = key
	return 1, ttl, nil
}

func TestLimiter_Check_AllRulesEvaluated_OnFirstViolation(t *testing.T) {
	ctx := context.Background()
	counts := &countingBackend{}
	rules := []Rule{
		{ID: "r1", TTL: time.Minute, MaxRequests: 0}, // always violated
		{ID: "r2", TTL: time.Minute, MaxRequests: 10},
		{ID: "r3", TTL: time.Minute, MaxRequests: 10},
	}
	l := New(counts, rules...)

	results, err := l.Check(ctx, "ip")
	require.NoError(t, err)
	assert.Equal(t, 3, counts.calls, "all rules must be evaluated even when first is violated")
	assert.False(t, results[0].Allowed)
	assert.True(t, results[1].Allowed)
	assert.True(t, results[2].Allowed)
}

type countingBackend struct {
	calls int
}

func (c *countingBackend) IncAndGetTTL(_ context.Context, _ string, ttl time.Duration) (int64, time.Duration, error) {
	c.calls++
	return int64(c.calls), ttl, nil
}

func TestSummarize(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		results     []RuleResult
		wantAllowed bool
		wantWorstID string
	}{
		{
			name: "all allowed",
			results: []RuleResult{
				{ID: "r1", Allowed: true},
				{ID: "r2", Allowed: true},
			},
			wantAllowed: true,
			wantWorstID: "",
		},
		{
			name: "one violated",
			results: []RuleResult{
				{ID: "r1", Allowed: false, ExpiresAt: now.Add(30 * time.Second)},
				{ID: "r2", Allowed: true},
			},
			wantAllowed: false,
			wantWorstID: "r1",
		},
		{
			name: "multiple violated — worst is furthest ExpiresAt",
			results: []RuleResult{
				{ID: "burst", Allowed: false, ExpiresAt: now.Add(10 * time.Second)},
				{ID: "hour",  Allowed: false, ExpiresAt: now.Add(45 * time.Minute)},
				{ID: "day",   Allowed: false, ExpiresAt: now.Add(3 * time.Hour)},
			},
			wantAllowed: false,
			wantWorstID: "day",
		},
		{
			name:        "empty results",
			results:     []RuleResult{},
			wantAllowed: true,
			wantWorstID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, worst := Summarize(tt.results)
			assert.Equal(t, tt.wantAllowed, allowed)
			if tt.wantWorstID == "" {
				assert.Nil(t, worst)
			} else {
				require.NotNil(t, worst)
				assert.Equal(t, tt.wantWorstID, worst.ID)
			}
		})
	}
}

func TestSummarize_WorstIsPointerIntoOriginalSlice(t *testing.T) {
	results := []RuleResult{
		{ID: "r1", Allowed: false, ExpiresAt: time.Now().Add(time.Hour)},
	}
	_, worst := Summarize(results)
	require.NotNil(t, worst)
	assert.Same(t, worst, &results[0])
}

// TestToResult_Invariants verifies the core invariants of toResult for any input:
//   - Allowed == (count <= maxRequests)
//   - RetryAfter == 0 when Allowed, > 0 when !Allowed
//   - Current and Max reflect the inputs exactly
func TestToResult_Invariants(t *testing.T) {
	prop := func(maxReq int64, count int64, remainingSec uint32) bool {
		if maxReq < 0 {
			maxReq = -maxReq
		}
		remaining := time.Duration(remainingSec) * time.Second
		rule := Rule{ID: "r", TTL: time.Minute, MaxRequests: maxReq}

		r := toResult(rule, count, remaining)

		if r.Allowed != (count <= maxReq) {
			return false
		}
		if r.Allowed && r.RetryAfter != 0 {
			return false
		}
		if !r.Allowed && r.RetryAfter == 0 {
			return false
		}
		if r.Current != count || r.Max != maxReq || r.ID != rule.ID {
			return false
		}
		return true
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Error(err)
	}
}

// TestCheck_BatchEqualsSerial verifies that batch and serial paths produce identical
// Allowed/Current/Max/ID results for arbitrary rule sets and counters.
func TestCheck_BatchEqualsSerial(t *testing.T) {
	ctx := context.Background()

	prop := func(maxR1, maxR2 uint32, hits uint8) bool {
		rules := []Rule{
			{ID: "r1", TTL: time.Minute, MaxRequests: int64(maxR1)},
			{ID: "r2", TTL: time.Hour, MaxRequests: int64(maxR2)},
		}

		batchB := newBatchCapturingBackend()
		serialB := newMockBackend(time.Minute, nil)

		batchL := New(batchB, rules...)
		serialL := New(serialB, rules...)

		var batchRes, serialRes []RuleResult
		var err error
		for range int(hits) + 1 {
			batchRes, err = batchL.Check(ctx, "u")
			if err != nil {
				return false
			}
			serialRes, err = serialL.Check(ctx, "u")
			if err != nil {
				return false
			}
		}

		for i := range rules {
			if batchRes[i].ID != serialRes[i].ID ||
				batchRes[i].Allowed != serialRes[i].Allowed ||
				batchRes[i].Current != serialRes[i].Current ||
				batchRes[i].Max != serialRes[i].Max {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Error(err)
	}
}

// batchCapturingBackend records whether the batch path was taken.
type batchCapturingBackend struct {
	counts     map[string]int64
	batchCalls int
	serialCalls int
}

func newBatchCapturingBackend() *batchCapturingBackend {
	return &batchCapturingBackend{counts: make(map[string]int64)}
}

func (b *batchCapturingBackend) IncAndGetTTL(_ context.Context, key string, ttl time.Duration) (int64, time.Duration, error) {
	b.serialCalls++
	b.counts[key]++
	return b.counts[key], ttl, nil
}

func (b *batchCapturingBackend) IncAndGetTTLBatch(_ context.Context, entries []BatchEntry) ([]BatchResult, error) {
	b.batchCalls++
	results := make([]BatchResult, len(entries))
	for i, e := range entries {
		b.counts[e.Key]++
		results[i] = BatchResult{Count: b.counts[e.Key], Remaining: e.TTL}
	}
	return results, nil
}

func TestLimiter_Check_UsesBatchPath_WhenAvailable(t *testing.T) {
	ctx := context.Background()
	b := newBatchCapturingBackend()
	rules := []Rule{
		{ID: "r1", TTL: time.Minute, MaxRequests: 10},
		{ID: "r2", TTL: time.Hour, MaxRequests: 100},
		{ID: "r3", TTL: 24 * time.Hour, MaxRequests: 1000},
	}
	l := New(b, rules...)

	results, err := l.Check(ctx, "user:1")
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, 1, b.batchCalls, "batch path must be used")
	assert.Equal(t, 0, b.serialCalls, "serial path must not be called when batch is available")
}

func TestLimiter_Check_FallsBackToSerial_WhenNoBatch(t *testing.T) {
	ctx := context.Background()
	b := newMockBackend(time.Minute, nil)
	rules := []Rule{
		{ID: "r1", TTL: time.Minute, MaxRequests: 10},
		{ID: "r2", TTL: time.Hour, MaxRequests: 100},
	}
	l := New(b, rules...)

	results, err := l.Check(ctx, "user:1")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Len(t, b.counts, 2, "serial path increments each key individually")
}

func TestLimiter_Check_BatchResultsMatchSerialResults(t *testing.T) {
	ctx := context.Background()
	rules := []Rule{
		{ID: "r1", TTL: time.Minute, MaxRequests: 5},
		{ID: "r2", TTL: time.Hour, MaxRequests: 50},
	}

	batchB := newBatchCapturingBackend()
	serialB := newMockBackend(time.Minute, nil)

	batchResults, err := New(batchB, rules...).Check(ctx, "u")
	require.NoError(t, err)
	serialResults, err := New(serialB, rules...).Check(ctx, "u")
	require.NoError(t, err)

	for i := range rules {
		assert.Equal(t, batchResults[i].ID, serialResults[i].ID)
		assert.Equal(t, batchResults[i].Allowed, serialResults[i].Allowed)
		assert.Equal(t, batchResults[i].Current, serialResults[i].Current)
		assert.Equal(t, batchResults[i].Max, serialResults[i].Max)
	}
}
