// Package yarl provides a multi-rule rate limiter with pluggable storage backends.
//
// Create a [Limiter] with [New], passing one or more [Rule] values that define
// the rate-limit policies. Call [Limiter.Check] on each incoming request with
// the identity key (e.g. client IP, user ID) to get a [RuleResult] per rule.
package yarl

import (
	"context"
	"time"
)

// Rule defines one rate-limit policy.
// Each Rule gets its own key in the backend: "{ID}:{userKey}".
// ID must be unique within the rules passed to [New].
type Rule struct {
	ID          string        // key namespace; must be unique per Limiter
	TTL         time.Duration // window duration and key expiry
	MaxRequests int64         // allowed requests per window
}

// RuleResult is the outcome for one [Rule] after a [Limiter.Check] call.
type RuleResult struct {
	ID         string
	Allowed    bool
	Current    int64         // counter value after this increment
	Max        int64         // copy of Rule.MaxRequests
	ExpiresAt  time.Time     // when the current window resets
	RetryAfter time.Duration // > 0 only when Allowed == false
}

// Backend is the storage interface for [Limiter].
// Implementations must be safe for concurrent use.
type Backend interface {
	// IncAndGetTTL atomically increments the counter for key by 1, sets its expiry
	// to ttl only on first creation (no-op if already set), and returns the new
	// counter value and the remaining TTL. If the key did not exist, remaining == ttl.
	IncAndGetTTL(ctx context.Context, key string, ttl time.Duration) (count int64, remaining time.Duration, err error)
}

// BatchEntry is one entry in a [BatchBackend.IncAndGetTTLBatch] call.
type BatchEntry struct {
	Key string
	TTL time.Duration
}

// BatchResult is the outcome for one [BatchEntry].
type BatchResult struct {
	Count     int64
	Remaining time.Duration
}

// BatchBackend is an optional extension of [Backend] for backends that can process
// multiple keys in a single round-trip (e.g. a Redis pipeline covering all rules).
// [Limiter.Check] uses this path automatically when the backend implements it,
// reducing N round-trips to 1 for N rules.
type BatchBackend interface {
	Backend
	IncAndGetTTLBatch(ctx context.Context, entries []BatchEntry) ([]BatchResult, error)
}

// Limiter evaluates a fixed set of [Rule] values on every [Limiter.Check] call.
type Limiter struct {
	backend Backend
	rules   []Rule
}

// New creates a Limiter backed by b. Rules are fixed for the lifetime of the Limiter.
// Each rule must have a unique ID.
func New(b Backend, rules ...Rule) *Limiter {
	return &Limiter{backend: b, rules: rules}
}

// Check evaluates every Rule against userKey and returns one [RuleResult] per Rule.
// All rules are always evaluated; Check does not short-circuit on first violation.
// If the backend implements [BatchBackend], all rules are evaluated in a single round-trip.
func (l *Limiter) Check(ctx context.Context, userKey string) ([]RuleResult, error) {
	if bb, ok := l.backend.(BatchBackend); ok {
		return l.checkBatch(ctx, userKey, bb)
	}
	return l.checkSerial(ctx, userKey)
}

func (l *Limiter) checkSerial(ctx context.Context, userKey string) ([]RuleResult, error) {
	results := make([]RuleResult, 0, len(l.rules))
	for _, rule := range l.rules {
		count, remaining, err := l.backend.IncAndGetTTL(ctx, rule.ID+":"+userKey, rule.TTL)
		if err != nil {
			return nil, err
		}
		results = append(results, toResult(rule, count, remaining))
	}
	return results, nil
}

func (l *Limiter) checkBatch(ctx context.Context, userKey string, bb BatchBackend) ([]RuleResult, error) {
	entries := make([]BatchEntry, len(l.rules))
	for i, rule := range l.rules {
		entries[i] = BatchEntry{Key: rule.ID + ":" + userKey, TTL: rule.TTL}
	}

	batchResults, err := bb.IncAndGetTTLBatch(ctx, entries)
	if err != nil {
		return nil, err
	}

	results := make([]RuleResult, len(l.rules))
	for i, rule := range l.rules {
		results[i] = toResult(rule, batchResults[i].Count, batchResults[i].Remaining)
	}
	return results, nil
}

// Summarize scans results and reports whether all rules passed.
// If any rule was violated it also returns the violated rule with the furthest
// ExpiresAt — i.e. the one the caller must wait the longest to retry.
// Returns (true, nil) when all rules are allowed.
func Summarize(results []RuleResult) (allowed bool, worst *RuleResult) {
	for i := range results {
		if !results[i].Allowed {
			if worst == nil || results[i].ExpiresAt.After(worst.ExpiresAt) {
				worst = &results[i]
			}
		}
	}
	return worst == nil, worst
}

func toResult(rule Rule, count int64, remaining time.Duration) RuleResult {
	allowed := count <= rule.MaxRequests
	r := RuleResult{
		ID:        rule.ID,
		Allowed:   allowed,
		Current:   count,
		Max:       rule.MaxRequests,
		ExpiresAt: time.Now().Add(remaining),
	}
	if !allowed {
		r.RetryAfter = remaining
	}
	return r
}
