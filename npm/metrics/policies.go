package metrics

// IncNumPolicies increments the number of policies.
func IncNumPolicies() {
	numPolicies.Inc()
}

// DecNumPolicies decrements the number of policies.
func DecNumPolicies() {
	numPolicies.Dec()
}

// ResetNumPolicies sets the number of policies to 0.
func ResetNumPolicies() {
	numPolicies.Set(0)
}

// RecordControllerPolicyExecTime adds an observation of policy exec time  (unless the operation is NoOp).
// The execution time is from the timer's start until now.
func RecordControllerPolicyExecTime(timer *Timer, op OperationKind, hadError bool) {
	if op == CreateOp {
		timer.stopAndRecordExecTimeWithError(addPolicyExecTime, hadError)
	} else {
		timer.stopAndRecordCRUDExecTime(controllerPolicyExecTime, op, hadError)
	}
}

// GetNumPolicies returns the number of policies.
// This function is slow.
func GetNumPolicies() (int, error) {
	return getValue(numPolicies)
}

// GetControllerPolicyExecCount returns the number of observations for policy exec time for the specified operation.
// This function is slow.
func GetControllerPolicyExecCount(op OperationKind, hadError bool) (int, error) {
	if op == CreateOp {
		return getCountVecValue(addPolicyExecTime, getErrorLabels(hadError))
	}
	return getCountVecValue(controllerPolicyExecTime, getCRUDExecTimeLabels(op, hadError))
}
