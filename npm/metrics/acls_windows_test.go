package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordACLLatency(t *testing.T) {
	timer := StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordACLLatency(timer, UpdateOp)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordACLLatency(timer, CreateOp)

	count, err := TotalACLLatencyCalls(CreateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have recorded create once")

	count, err = TotalACLLatencyCalls(UpdateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have recorded update once")
}

func TestIncACLFailures(t *testing.T) {
	IncACLFailures(CreateOp)
	IncACLFailures(UpdateOp)
	IncACLFailures(CreateOp)

	count, err := TotalACLFailures(CreateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count, "should have failed to create twice")

	count, err = TotalACLFailures(UpdateOp)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count, "should have failed to update once")
}
