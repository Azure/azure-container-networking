package logger

import (
	"testing"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type traceRecorder struct {
	aitelemetry.TelemetryHandle
	reports []aitelemetry.Report
}

func (r *traceRecorder) TrackLog(report aitelemetry.Report) {
	r.reports = append(r.reports, report)
}

func TestSetContextDetailsPartitionProperties(t *testing.T) {
	shim, err := New("testlogs", 0, 0, t.TempDir())
	require.NoError(t, err)
	l := shim.(*logger)
	t.Cleanup(func() {
		l.th = nil
		l.Close()
	})
	recorder := &traceRecorder{}
	l.th = recorder

	for _, partition := range []string{"partition1", "partition2", ""} {
		l.SetContextDetails("ServiceFabric", "node1", partition)
		l.Printf("test trace")
		report := recorder.reports[len(recorder.reports)-1]
		assert.Equal(t, "node1", report.Context)
		assert.Equal(t, "node1", report.CustomDimensions[nodeIDKey])
		assert.Equal(t, "ServiceFabric", report.CustomDimensions[orchestratorTypeKey])
		if partition == "" {
			assert.NotContains(t, report.CustomDimensions, dncPartitionKeyKey)
		} else {
			assert.Equal(t, partition, report.CustomDimensions[dncPartitionKeyKey])
		}
	}
	assert.Equal(t, "partition1", recorder.reports[0].CustomDimensions[dncPartitionKeyKey])
}
