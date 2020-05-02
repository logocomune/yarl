package yarl

import "time"

const (
	format             = "0102_150405"
	ttlSafeWindowInSec = 5
)

func timeKey(t time.Time, d time.Duration) string {
	return t.UTC().Truncate(d).Format(format)
}

func nextResetInSec(t time.Time, d time.Duration) (int64, int64) {
	now := t.Unix()
	resetAt := t.UTC().Truncate(d).Add(d).Unix()

	return resetAt - now, resetAt
}

func ttl(sec int64) int64 {
	return sec + ttlSafeWindowInSec
}

func ttlByDuration(d time.Duration) int64 {
	return int64(d/time.Second) + ttlSafeWindowInSec
}
