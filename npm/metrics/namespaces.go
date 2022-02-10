package metrics

// RecordControllerNamespaceExecTime adds an observation of namespace exec time (unless the operation is NoOp).
// The execution time is from the timer's start until now.
func RecordControllerNamespaceExecTime(timer *Timer, op OperationKind, hadError bool) {
	timer.stopAndRecordCRUDExecTime(controllerNamespaceExecTime, op, hadError)
}

// GetControllerNamespaceExecCount returns the number of observations for namespace exec time for the specified operation.
// This function is slow.
func GetControllerNamespaceExecCount(op OperationKind, hadError bool) (int, error) {
	return getCountVecValue(controllerNamespaceExecTime, getCRUDExecTimeLabels(op, hadError))
}
