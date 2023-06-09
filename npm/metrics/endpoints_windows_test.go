package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordListEndpointsLatency(t *testing.T) {
	timer := StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordListEndpointsLatency(timer)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordListEndpointsLatency(timer)

	count, err := TotalListEndpointsLatencyCalls()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count, "should have recorded list endpoints twice")
}

func TestIncListEndpointsFailures(t *testing.T) {
	IncListEndpointsFailures()
	IncListEndpointsFailures()
	IncListEndpointsFailures()

	count, err := TotalListEndpointsFailures()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count, "should have failed to list endpoints thrice")
}

func TestRecordGetEndpointLatency(t *testing.T) {
	timer := StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordGetEndpointLatency(timer)
	timer = StartNewTimer()
	time.Sleep(1 * time.Millisecond)
	RecordGetEndpointLatency(timer)

	count, err := TotalGetEndpointLatencyCalls()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 2, count, "should have recorded get endpoint twice")
}

func TestIncGetEndpointFailures(t *testing.T) {
	IncGetEndpointFailures()
	IncGetEndpointFailures()
	IncGetEndpointFailures()

	count, err := TotalGetEndpointFailures()
	require.Nil(t, err, "failed to get metric")
	require.Equal(t, 3, count, "should have failed to get endpoint thrice")
}
