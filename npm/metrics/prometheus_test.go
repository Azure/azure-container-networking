package metrics

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	InitializeAll()
	exitCode := m.Run()
	os.Exit(exitCode)
}

type basicMetric struct {
	reset func()
	inc   func()
	dec   func()
	get   func() (int, error)
}

type amountMetric struct {
	*basicMetric
	incBy func(int)
	decBy func(int)
}

func (metric *amountMetric) testIncByMetric(t *testing.T) {
	metric.reset()
	metric.incBy(2)
	assertMetricVal(t, metric.basicMetric, 2)
}

func (metric *amountMetric) testDecByMetric(t *testing.T) {
	metric.reset()
	metric.incBy(5)
	metric.decBy(2)
	assertMetricVal(t, metric.basicMetric, 3)
}

type recordingMetric struct {
	record   func(timer *Timer)
	getCount func() (int, error)
}

type crudExecMetric struct {
	record   func(timer *Timer, op OperationKind, hadError bool)
	getCount func(op OperationKind, hadError bool) (int, error)
}

func assertMetricVal(t *testing.T, metric *basicMetric, expectedVal int) {
	val, err := metric.get()
	promutil.NotifyIfErrors(t, err)
	if val != expectedVal {
		require.FailNowf(t, "", "expected metric count to be %d but got %d", expectedVal, val)
	}
}

func testIncMetric(t *testing.T, metric *basicMetric) {
	metric.reset()
	metric.inc()
	metric.inc()
	assertMetricVal(t, metric, 2)
}

func testDecMetric(t *testing.T, metric *basicMetric) {
	metric.reset()
	metric.inc()
	metric.inc()
	metric.dec()
	assertMetricVal(t, metric, 1)
}

func testResetMetric(t *testing.T, metric *basicMetric) {
	metric.inc()
	metric.inc()
	metric.reset()
	assertMetricVal(t, metric, 0)
}

func testStopAndRecordCRUDExecTime(t *testing.T, m *crudExecMetric) {
	for _, mode := range []OperationKind{CreateOp, UpdateOp, DeleteOp} {
		for _, hadError := range []bool{true, false} {
			testStopAndRecord(t, &recordingMetric{
				func(timer *Timer) {
					m.record(timer, mode, hadError)
				},
				func() (int, error) {
					return m.getCount(mode, hadError)
				},
			})
		}
	}
}

func testStopAndRecord(t *testing.T, metric *recordingMetric) {
	InitializeAll()
	durations := []int{100, 100, 100, 100, 100, 100, 100, 100, 200, 300}
	quantileMap := map[float64]float64{0.5: 100, 0.9: 200, 0.99: 300}
	for _, duration := range durations {
		timer := StartNewTimer()
		time.Sleep(time.Duration(duration) * time.Millisecond)
		timer.stop()
		metric.record(timer)
	}

	quantiles, err := getQuantiles(addACLRuleExecTime)
	require.NoError(t, err)
	for _, quantile := range quantiles {
		percent := quantile.GetQuantile()
		duration := math.Floor(quantile.GetValue())
		expectedDuration := quantileMap[percent]
		if duration > expectedDuration+millisecondForgiveness {
			require.FailNowf(t, "", "expected quantile %f for timer to be  %f but got %f", percent, expectedDuration, duration)
		}
	}

	val, err := metric.getCount()
	require.NoError(t, err)
	expectedVal := len(durations)
	if val != expectedVal {
		require.FailNowf(t, "", "expected exec count to be %d but got %d", expectedVal, val)
	}
}
