package yarl

import "time"

const (
	format             = "0102_150405"
	ttlSafeWindowInSec = 5
)

func timeKey(t time.Time, d time.Duration) string {
	return t.UTC().Truncate(d).Format(format)
}

func nextResetInSec(t time.Time, d time.Duration) int64 {
	return t.UTC().Truncate(d).Add(d).Unix()
}

func ttl(d time.Duration) int64 {
	return int64(d/time.Second) + ttlSafeWindowInSec
}
