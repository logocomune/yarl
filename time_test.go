package yarl

import (
	"testing"
	"time"
)

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
