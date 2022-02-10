package metrics

import "testing"

func TestRecordControllerPodExecTime(t *testing.T) {
	testStopAndRecordCRUDExecTime(t, &crudExecMetric{
		RecordControllerPodExecTime,
		GetControllerPodExecCount,
	})
}
