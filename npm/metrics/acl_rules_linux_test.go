package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordIPTablesBackgroundRestoreLatency(t *testing.T) {
	timer := StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordIPTablesBackgroundRestoreLatency(timer, UpdateOp)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordIPTablesBackgroundRestoreLatency(timer, CreateOp)

	count, err := TotalIPTablesBackgroundRestoreLatencyCalls(CreateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have recorded create once")

	count, err = TotalIPTablesBackgroundRestoreLatencyCalls(UpdateOp)
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

func TestIncIPTablesBackgroundRestoreFailures(t *testing.T) {
	IncIPTablesBackgroundRestoreFailures(CreateOp)
	IncIPTablesBackgroundRestoreFailures(UpdateOp)
	IncIPTablesBackgroundRestoreFailures(CreateOp)

	count, err := TotalIPTablesBackgroundRestoreFailures(CreateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count, "should have failed to create twice")

	count, err = TotalIPTablesBackgroundRestoreFailures(UpdateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have failed to update once")
}
