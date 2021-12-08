package ipampool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldScaleUp(t *testing.T) {
	tests := []struct {
		name         string
		max, minFree int64
		in           ipPool
		want         bool
	}{
		{
			name:    "yes",
			max:     32,
			minFree: 8,
			in: ipPool{
				requested: 16,
				assigned:  9,
			},
			want: true,
		},
		{
			name:    "no: enough free",
			max:     32,
			minFree: 8,
			in: ipPool{
				requested: 16,
				assigned:  7,
			},
			want: false,
		},
		{
			name:    "no: at max",
			max:     16,
			minFree: 16,
			in: ipPool{
				requested: 16,
				assigned:  16,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.shouldScaleUp(tt.minFree, tt.max))
		})
	}
}

func TestShouldScaleDown(t *testing.T) {
	tests := []struct {
		name    string
		maxFree int64
		in      ipPool
		want    bool
	}{
		{
			name:    "yes",
			maxFree: 24,
			in: ipPool{
				allocated: 32,
				assigned:  1,
			},
			want: true,
		},
		{
			name:    "no",
			maxFree: 24,
			in: ipPool{
				allocated: 32,
				assigned:  16,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.shouldScaleDown(tt.maxFree))
		})
	}
}

func TestShouldCleanPendingRelease(t *testing.T) {
	tests := []struct {
		name     string
		notInUse int64
		in       ipPool
		want     bool
	}{
		{
			name:     "yes",
			notInUse: 24,
			in: ipPool{
				pendingRelease: 32,
			},
			want: true,
		},
		{
			name:     "no",
			notInUse: 24,
			in: ipPool{
				pendingRelease: 24,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.shouldCleanPendingRelease(tt.notInUse))
		})
	}
}

func TestScale(t *testing.T) {
	tests := []struct {
		name             string
		minFree, maxFree float64
		batch            int64
		in               ipPool
		want             int64
	}{
		{
			name:    "noop",
			minFree: 0.5,
			maxFree: 1.5,
			batch:   16,
			in: ipPool{
				requested: 16,
				allocated: 16,
				assigned:  8,
			},
			want: 16,
		},
		{
			name:    "up",
			minFree: 0.5,
			maxFree: 1.5,
			batch:   16,
			in: ipPool{
				requested: 16,
				allocated: 16,
				assigned:  9,
			},
			want: 32,
		},
		{
			name:    "down",
			minFree: 0.5,
			maxFree: 1.5,
			batch:   16,
			in: ipPool{
				requested: 32,
				allocated: 32,
				assigned:  7,
			},
			want: 16,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.scale(tt.batch, tt.minFree, tt.maxFree).requested)
		})
	}
}
