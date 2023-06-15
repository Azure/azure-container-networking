package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const millisecondForgiveness = 25

func TestTimeElapsed(t *testing.T) {
	expectedDuration := 100.0
	timer := StartNewTimer()
	time.Sleep(time.Millisecond * time.Duration(expectedDuration))
	duration := math.Floor(timer.timeElapsedSeconds())
	if duration > expectedDuration+millisecondForgiveness {
		require.FailNowf(t, "", "expected elapsed time for timer to be  %f but got %f", expectedDuration, duration)
	}
}
