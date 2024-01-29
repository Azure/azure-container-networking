package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateTargetIPCount(t *testing.T) {
	tests := []struct {
		name   string
		demand int64
		batch  int64
		buffer float64
		want   int64
	}{
		{
			name:   "base case",
			demand: 0,
			batch:  16,
			buffer: .5,
			want:   16,
		},
		{
			name:   "1/2 demand",
			demand: 8,
			batch:  16,
			buffer: .5,
			want:   16,
		},
		{
			name:   "1x demand",
			demand: 16,
			batch:  16,
			buffer: .5,
			want:   32,
		},
		{
			name:   "2x demand",
			demand: 32,
			batch:  16,
			buffer: .5,
			want:   48,
		},
		{
			name:   "3x demand",
			demand: 48,
			batch:  16,
			buffer: .5,
			want:   64,
		},
		{
			name:   "batch of one",
			demand: 10,
			batch:  1,
			buffer: .5,
			want:   11,
		},
		{
			name:   "zero buffer",
			demand: 10,
			batch:  16,
			buffer: 0,
			want:   16,
		},
		{
			name:   "zero buffer batch of one",
			demand: 13,
			batch:  1,
			buffer: 0,
			want:   13,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, calculateTargetIPCount(tt.demand, tt.batch, tt.buffer))
		})
	}
}

func TestCalculateTargetIPCountOrMax(t *testing.T) {
	tests := []struct {
		name   string
		demand int64
		batch  int64
		buffer float64
		max    int64
		want   int64
	}{
		{
			name:   "base case",
			demand: 0,
			batch:  16,
			buffer: .5,
			max:    100,
			want:   16,
		},
		{
			name:   "clamp to max",
			demand: 500,
			batch:  16,
			buffer: .5,
			max:    250,
			want:   250,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, calculateTargetIPCountOrMax(tt.demand, tt.batch, tt.max, tt.buffer))
		})
	}
}
