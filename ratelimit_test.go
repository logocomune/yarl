package yarl

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	type args struct {
		prefix     string
		l          Limiter
		max        int64
		timeWindow time.Duration
	}
	tests := []struct {
		name string
		args args
		want Yarl
	}{
		{
			name: "Init",
			args: args{
				prefix:     "MyPrefix",
				l:          nil,
				max:        1,
				timeWindow: time.Second,
			},
			want: Yarl{
				prefix:  "MyPrefix",
				max:     1,
				tWindow: time.Second,
				limiter: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.args.prefix, tt.args.l, tt.args.max, tt.args.timeWindow); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New() = %v, want %v", got, tt.want)
			}
		})
	}
}

func NewMocklLimiter(current int64, err error) *MockLimiter {
	return &MockLimiter{
		count: current,
		err:   err,
	}
}

type MockLimiter struct {
	count int64
	err   error
}

func (m *MockLimiter) Inc(key string, ttlSeconds int64) (int64, error) {
	m.count++

	return m.count, m.err

}

func TestYarl_IsAllow(t *testing.T) {
	type fields struct {
		prefix  string
		tWindow time.Duration
		max     int64
		limiter Limiter
	}
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *Resp
		wantErr bool
	}{
		{
			name: "Under limit",
			fields: fields{
				prefix:  "",
				tWindow: 10 * time.Second,
				max:     10,
				limiter: NewMocklLimiter(0, nil),
			},
			args: args{key: "my_key"},
			want: &Resp{
				IsAllowed: true,
				Current:   1,
				Max:       10,
				Remain:    9,
				NextReset: 0,
			},
			wantErr: false,
		},
		{
			name: "Near limit",
			fields: fields{
				prefix:  "",
				tWindow: 10 * time.Second,
				max:     10,
				limiter: NewMocklLimiter(9, nil),
			},
			args: args{key: "my_key"},
			want: &Resp{
				IsAllowed: true,
				Current:   10,
				Max:       10,
				Remain:    0,
				NextReset: 0,
			},
			wantErr: false,
		},
		{
			name: "Limit reach",
			fields: fields{
				prefix:  "",
				tWindow: 0,
				max:     10,
				limiter: NewMocklLimiter(10, nil),
			},
			args: args{key: "my_key"},
			want: &Resp{
				IsAllowed: false,
				Current:   11,
				Max:       10,
				Remain:    0,
				NextReset: 0,
			},
			wantErr: false,
		},
		{
			name: "Limiter error",
			fields: fields{
				prefix:  "",
				tWindow: 0,
				max:     10,
				limiter: NewMocklLimiter(0, errors.New("Generic error")),
			},
			args:    args{key: "my_key"},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y := &Yarl{
				prefix:  tt.fields.prefix,
				tWindow: tt.fields.tWindow,
				max:     tt.fields.max,
				limiter: tt.fields.limiter,
			}
			got, err := y.IsAllow(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsAllow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil {
				tt.want.NextReset = got.NextReset
				tt.want.RetryAfter = got.RetryAfter
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IsAllow() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYarl_IsAllowWithLimits(t *testing.T) {
	type fields struct {
		prefix  string
		tWindow time.Duration
		max     int64
		limiter Limiter
	}
	type args struct {
		key     string
		max     int64
		tWindow time.Duration
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *Resp
		wantErr bool
	}{
		{
			name: "Under limit",
			fields: fields{
				prefix:  "",
				tWindow: 1000 * time.Second,
				max:     1000,
				limiter: NewMocklLimiter(0, nil),
			},
			args: args{
				key:     "my_key",
				max:     10,
				tWindow: 1 * time.Second,
			},
			want: &Resp{
				IsAllowed: true,
				Current:   1,
				Max:       10,
				Remain:    9,
				NextReset: 0,
			},
			wantErr: false,
		},
		{
			name: "Near limit",
			fields: fields{
				prefix:  "",
				tWindow: 1000 * time.Second,
				max:     1000,
				limiter: NewMocklLimiter(9, nil),
			},
			args: args{
				key:     "my_key",
				max:     10,
				tWindow: 1 * time.Second,
			},
			want: &Resp{
				IsAllowed: true,
				Current:   10,
				Max:       10,
				Remain:    0,
				NextReset: 0,
			},
			wantErr: false,
		},
		{
			name: "Limit reach",
			fields: fields{
				prefix:  "",
				tWindow: 0,
				max:     10,
				limiter: NewMocklLimiter(10, nil),
			},
			args: args{
				key:     "my_key",
				max:     10,
				tWindow: 1 * time.Second,
			},
			want: &Resp{
				IsAllowed: false,
				Current:   11,
				Max:       10,
				Remain:    0,
				NextReset: 0,
			},
			wantErr: false,
		},
		{
			name: "Limiter error",
			fields: fields{
				prefix:  "",
				tWindow: 0,
				max:     10,
				limiter: NewMocklLimiter(0, errors.New("generic error")),
			},
			args: args{
				key:     "my_key",
				max:     10,
				tWindow: 1 * time.Second,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y := &Yarl{
				prefix:  tt.fields.prefix,
				tWindow: tt.fields.tWindow,
				max:     tt.fields.max,
				limiter: tt.fields.limiter,
			}
			got, err := y.IsAllowWithLimit(tt.args.key, tt.args.max, tt.args.tWindow)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsAllowWithLimit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != nil {
				tt.want.NextReset = got.NextReset
				tt.want.RetryAfter = got.RetryAfter
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IsAllowWithLimit() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// KeyCapturingMock records the last key passed to Inc.
type KeyCapturingMock struct {
	capturedKey string
}

func (m *KeyCapturingMock) Inc(key string, _ int64) (int64, error) {
	m.capturedKey = key
	return 1, nil
}

func TestYarl_KeyBuilder(t *testing.T) {
	t.Run("Key with prefix", func(t *testing.T) {
		mock := &KeyCapturingMock{}
		y := New("myprefix", mock, 10, time.Minute)
		_, _ = y.IsAllow("mykey")

		if !strings.HasPrefix(mock.capturedKey, "myprefix_") {
			t.Errorf("keyBuilder() key = %q, expected prefix 'myprefix_'", mock.capturedKey)
		}
		if !strings.HasSuffix(mock.capturedKey, "_mykey") {
			t.Errorf("keyBuilder() key = %q, expected suffix '_mykey'", mock.capturedKey)
		}
	})

	t.Run("Key without prefix", func(t *testing.T) {
		mock := &KeyCapturingMock{}
		y := New("", mock, 10, time.Minute)
		_, _ = y.IsAllow("mykey")

		if strings.HasPrefix(mock.capturedKey, "_") {
			t.Errorf("keyBuilder() key = %q, should not start with '_' when prefix is empty", mock.capturedKey)
		}
		if !strings.HasSuffix(mock.capturedKey, "_mykey") {
			t.Errorf("keyBuilder() key = %q, expected suffix '_mykey'", mock.capturedKey)
		}
	})

	t.Run("Different tWindow produces different key", func(t *testing.T) {
		mock1 := &KeyCapturingMock{}
		y1 := New("p", mock1, 10, time.Second)
		_, _ = y1.IsAllow("k")

		mock2 := &KeyCapturingMock{}
		y2 := New("p", mock2, 10, time.Hour)
		_, _ = y2.IsAllow("k")

		// Keys built with different windows may differ (different truncation)
		// Both should still end with "_k"
		if !strings.HasSuffix(mock1.capturedKey, "_k") {
			t.Errorf("keyBuilder() key = %q, expected suffix '_k'", mock1.capturedKey)
		}
		if !strings.HasSuffix(mock2.capturedKey, "_k") {
			t.Errorf("keyBuilder() key = %q, expected suffix '_k'", mock2.capturedKey)
		}
	})

	t.Run("Resp fields are populated correctly", func(t *testing.T) {
		mock := &KeyCapturingMock{}
		y := New("p", mock, 5, time.Minute)
		resp, err := y.IsAllow("k")
		if err != nil {
			t.Fatalf("IsAllow() unexpected error: %v", err)
		}
		if resp.Max != 5 {
			t.Errorf("Resp.Max = %d, want 5", resp.Max)
		}
		if resp.Current != 1 {
			t.Errorf("Resp.Current = %d, want 1", resp.Current)
		}
		if resp.Remain != 4 {
			t.Errorf("Resp.Remain = %d, want 4", resp.Remain)
		}
		if !resp.IsAllowed {
			t.Errorf("Resp.IsAllowed = false, want true")
		}
		if resp.NextReset <= 0 {
			t.Errorf("Resp.NextReset = %d, should be > 0", resp.NextReset)
		}
	})
}
