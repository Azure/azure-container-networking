package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordSetPolicyLatency(t *testing.T) {
	timer := StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordSetPolicyLatency(timer, CreateOp, false)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordSetPolicyLatency(timer, CreateOp, true)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordSetPolicyLatency(timer, UpdateOp, false)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordSetPolicyLatency(timer, DeleteOp, true)

	count, err := TotalSetPolicyLatencyCalls(CreateOp, false)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyLatencyCalls(CreateOp, true)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyLatencyCalls(UpdateOp, false)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyLatencyCalls(DeleteOp, true)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyLatencyCalls(UpdateOp, true)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 0, count)

	count, err = TotalSetPolicyLatencyCalls(DeleteOp, false)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 0, count)
}

func TestIncSetPolicyFailures(t *testing.T) {
	IncSetPolicyFailures(CreateOp, false)
	IncSetPolicyFailures(CreateOp, true)
	IncSetPolicyFailures(UpdateOp, false)
	IncSetPolicyFailures(DeleteOp, true)

	count, err := TotalSetPolicyFailures(CreateOp, false)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyFailures(CreateOp, true)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyFailures(UpdateOp, false)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyFailures(DeleteOp, true)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 1, count)

	count, err = TotalSetPolicyFailures(UpdateOp, true)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 0, count)

	count, err = TotalSetPolicyFailures(DeleteOp, false)
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 0, count)
}
