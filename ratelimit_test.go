package yarl

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	type args struct {
		prefix     string
		l          Limiter
		max        int
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

func NewMocklLimiter(current int, err error) *MockLimiter {
	return &MockLimiter{
		count: current,
		err:   err,
	}
}

type MockLimiter struct {
	count int
	err   error
}

func (m *MockLimiter) Inc(key string, ttlSeconds int64) (int, error) {
	m.count++

	return m.count, m.err

}

func TestYarl_IsAllow(t *testing.T) {
	type fields struct {
		prefix  string
		tWindow time.Duration
		max     int
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
		max     int
		limiter Limiter
	}
	type args struct {
		key     string
		max     int
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
