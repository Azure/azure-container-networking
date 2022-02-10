package metrics

// RecordControllerPodExecTime adds an observation of pod exec time for the specified operation (unless the operation is NoOp).
// The execution time is from the timer's start until now.
func RecordControllerPodExecTime(timer *Timer, op OperationKind, hadError bool) {
	timer.stopAndRecordCRUDExecTime(controllerPodExecTime, op, hadError)
}

// GetControllerPodExecCount returns the number of observations for pod exec time for the specified operation.
// This function is slow.
func GetControllerPodExecCount(op OperationKind, hadError bool) (int, error) {
	return getCountVecValue(controllerPodExecTime, getCRUDExecTimeLabels(op, hadError))
}
