package metrics

import (
	"time"

	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/prometheus/client_golang/prometheus"
)

const billion float64 = 1e+09

// Timer is a one-time-use tool for recording time between a start and end point
type Timer struct {
	before int64
	after  int64
}

// StartNewTimer creates a new Timer
func StartNewTimer() *Timer {
	return &Timer{time.Now().UnixNano(), 0}
}

// stopAndRecord uses milliseconds.
// It ends a timer and records its delta in a summary
func (timer *Timer) stopAndRecord(observer prometheus.Summary) {
	observer.Observe(timer.timeElapsed())
}

// stopAndRecordCRUDExecTime uses milliseconds.
// It ends a timer and records its delta in a summary (unless the operation is NoOp) with the specified operation as a label.
func (timer *Timer) stopAndRecordCRUDExecTime(observer *prometheus.SummaryVec, op OperationKind, hadError bool) {
	timer.stop()
	if !op.isValid() {
		SendErrorLogAndMetric(util.UtilID, "Unknown operation [%v] when recording exec time", op)
		return
	}
	if op != NoOp {
		labels := getCRUDExecTimeLabels(op, hadError)
		observer.With(labels).Observe(timer.timeElapsed())
	}
}

func (timer *Timer) stopAndRecordExecTimeWithError(observer *prometheus.SummaryVec, hadError bool) {
	timer.stop()
	labels := getErrorLabels(hadError)
	observer.With(labels).Observe(timer.timeElapsed())
}

func (timer *Timer) stop() {
	timer.after = time.Now().UnixNano()
}

// timeElapsed returns milliseconds
func (timer *Timer) timeElapsed() float64 {
	if timer.after == 0 {
		timer.stop()
	}
	millisecondDifference := float64(timer.after-timer.before) / 1000000.0
	return millisecondDifference
}

// timeElapsedSeconds returns seconds
func (timer *Timer) timeElapsedSeconds() float64 {
	if timer.after == 0 {
		timer.stop()
	}
	return float64(timer.after-timer.before) / billion
}
