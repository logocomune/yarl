package yarl

import (
	"testing"
	"time"
)

func TestNextResetInSec(t *testing.T) {
	// now = Wed Dec 18 11:55:36 UTC 2019
	now := time.Unix(1576670136, 0)

	tests := []struct {
		name        string
		d           time.Duration
		wantSec     int64
		wantResetAt int64
	}{
		{
			name:        "One second window",
			d:           time.Second,
			wantSec:     1,          // 11:55:36 -> 11:55:37
			wantResetAt: 1576670137, // 1576670136 + 1
		},
		{
			name:        "One minute window",
			d:           time.Minute,
			wantSec:     24,         // 11:55:36 -> 11:56:00 = 24s
			wantResetAt: 1576670160, // 1576670100 + 60
		},
		{
			name:        "One hour window",
			d:           time.Hour,
			wantSec:     264,        // 11:55:36 -> 12:00:00 = 264s
			wantResetAt: 1576670400, // 1576666800 + 3600
		},
		{
			name:        "One day window",
			d:           24 * time.Hour,
			wantSec:     43464,      // 11:55:36 -> midnight next day = 43200+264 = 43464
			wantResetAt: 1576713600, // 1576627200 + 86400
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sec, resetAt := nextResetInSec(now, tt.d)
			if sec != tt.wantSec {
				t.Errorf("nextResetInSec() sec = %v, want %v", sec, tt.wantSec)
			}
			if resetAt != tt.wantResetAt {
				t.Errorf("nextResetInSec() resetAt = %v, want %v", resetAt, tt.wantResetAt)
			}
		})
	}
}

func TestTimeKey(t *testing.T) {
	now := time.Unix(1576670136, 0)

	type args struct {
		t time.Time
		d time.Duration
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Seconds",
			args: args{
				t: now,
				d: time.Second,
			},
			want: "1218_115536",
		},
		{
			name: "Minute",
			args: args{
				t: now,
				d: time.Minute,
			},
			want: "1218_115500",
		},
		{
			name: "Hour",
			args: args{
				t: now,
				d: time.Hour,
			},
			want: "1218_110000",
		},
		{
			name: "Day",
			args: args{
				t: now,
				d: 24 * time.Hour,
			},
			want: "1218_000000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timeKey(tt.args.t, tt.args.d); got != tt.want {
				t.Errorf("timeKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ttlByDuration(t *testing.T) {
	type args struct {
		d time.Duration
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "Ten seconds",
			args: args{
				d: 10 * time.Second,
			},
			want: 10 + ttlSafeWindowInSec,
		},
		{
			name: "An Hour",
			args: args{
				d: 1 * time.Hour,
			},
			want: 3600 + ttlSafeWindowInSec,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ttlByDuration(tt.args.d); got != tt.want {
				t.Errorf("ttl() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ttl(t *testing.T) {
	type args struct {
		sec int64
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "Ten seconds",
			args: args{
				sec: 10,
			},
			want: 10 + ttlSafeWindowInSec,
		},
		{
			name: "An Hour",
			args: args{
				sec: 3600,
			},
			want: 3600 + ttlSafeWindowInSec,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ttl(tt.args.sec); got != tt.want {
				t.Errorf("ttl() = %v, want %v", got, tt.want)
			}
		})
	}
}
