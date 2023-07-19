package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordIPTablesRestoreLatency(t *testing.T) {
	timer := StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordIPTablesRestoreLatency(timer, UpdateOp)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordIPTablesRestoreLatency(timer, CreateOp)

	count, err := TotalIPTablesRestoreLatencyCalls(CreateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have recorded create once")

	count, err = TotalIPTablesRestoreLatencyCalls(UpdateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have recorded update once")
}

func TestRecordIPTablesDeleteLatency(t *testing.T) {
	timer := StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordIPTablesDeleteLatency(timer)

	count, err := TotalIPTablesDeleteLatencyCalls()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have recorded create once")
}

func TestIncIPTablesRestoreFailures(t *testing.T) {
	IncIPTablesRestoreFailures(CreateOp)
	IncIPTablesRestoreFailures(UpdateOp)
	IncIPTablesRestoreFailures(CreateOp)

	count, err := TotalIPTablesRestoreFailures(CreateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count, "should have failed to create twice")

	count, err = TotalIPTablesRestoreFailures(UpdateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have failed to update once")
}
