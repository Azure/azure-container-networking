package metrics

import "testing"

func TestRecordControllerNamespaceExecTime(t *testing.T) {
	testStopAndRecordCRUDExecTime(t, &crudExecMetric{
		RecordControllerNamespaceExecTime,
		GetControllerNamespaceExecCount,
	})
}
