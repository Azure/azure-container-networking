package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Timer is a one-time-use tool for recording time between a start and end point
type Timer struct {
	before int64
	after  int64
}

// StartNewTimer creates a new Timer
func StartNewTimer() *Timer {
	return &Timer{time.Now().Unix(), 0}
}

// StopAndRecord ends a timer and records its delta in a summary
func (timer *Timer) StopAndRecord(observer prometheus.Summary) {
	Observe(observer, timer.timeElapsed())
}

func (timer *Timer) stop() {
	timer.after = time.Now().Unix()
}

func (timer *Timer) timeElapsed() float64 {
	if timer.after == 0 {
		timer.stop()
	}
	millisecondDifference := (timer.after - timer.before) / 1000000
	secondDifference := float64(millisecondDifference) / 1000.0
	return secondDifference
}
