package metrics

import (
	"time"

	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/prometheus/client_golang/prometheus"
)

// Timer is a one-time-use tool for recording time between a start and end point
type Timer struct {
	before int64
	after  int64
}

// StartNewTimer creates a new Timer
func StartNewTimer() *Timer {
	return &Timer{time.Now().UnixNano(), 0}
}

// stopAndRecord ends a timer and records its delta in a summary
func (timer *Timer) stopAndRecord(observer prometheus.Summary) {
	observer.Observe(timer.timeElapsed())
}

// stopAndRecordCRUDExecTime ends a timer and records its delta in a summary (unless the operation is NoOp) with the specified operation as a label.
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

func (timer *Timer) timeElapsed() float64 {
	if timer.after == 0 {
		timer.stop()
	}
	millisecondDifference := float64(timer.after-timer.before) / 1000000.0
	return millisecondDifference
}
