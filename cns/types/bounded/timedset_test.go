package bounded

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTimedSet(t *testing.T) {
	tests := []struct {
		name    string
		cap     int
		in      []string
		out     []string
		dropped []string
	}{
		{
			name:    "size 1",
			cap:     1,
			in:      []string{"a", "b", "c"},
			out:     []string{"c"},
			dropped: []string{"a", "b"},
		},
		{
			name:    "overflow",
			cap:     2,
			in:      []string{"a", "b", "c"},
			out:     []string{"b", "c"},
			dropped: []string{"a"},
		},
		{
			name:    "not present",
			cap:     2,
			in:      []string{"a", "b"},
			out:     []string{"a", "b"},
			dropped: []string{"c", "d"},
		},
		{
			name: "dupe push",
			cap:  2,
			in:   []string{"a", "a", "a", "a"},
			out:  []string{"a"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // this test has timer delay, so parallelize
			ts := NewTimedSet(tt.cap)

			firstTime := map[string]time.Time{}
			for _, item := range tt.in {
				ts.Push(item)
				time.Sleep(5 * time.Millisecond)
				if _, ok := firstTime[item]; !ok {
					firstTime[item] = ts.items.m[item].(*TimedItem).Time
				}
			}

			require.LessOrEqual(t, ts.items.Len(), tt.cap)

			times := []time.Duration{}
			for _, item := range tt.out {
				_, ok := ts.items.Contains(item)
				assert.True(t, ok)
				assert.Equal(t, firstTime[item], ts.items.m[item].(*TimedItem).Time)

				times = append(times, ts.Pop(item))
			}

			for _, item := range tt.dropped {
				_, ok := ts.items.Contains(item)
				assert.False(t, ok)
				assert.Negative(t, ts.Pop(item))
			}

			require.NotContains(t, times, time.Duration(-1))

			for i := 0; i < len(times)-1; i++ {
				assert.Less(t, times[i+1], times[i])
			}
		})
	}
}
