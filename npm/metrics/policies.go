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

// RecordPolicyExecTime adds an observation of execution time for adding a policy.
// The execution time is from the timer's start until now.
func RecordPolicyExecTime(timer *Timer) {
	timer.stopAndRecord(addPolicyExecTime)
}

// GetNumPolicies returns the number of policies.
// This function is slow.
func GetNumPolicies() (int, error) {
	return getValue(numPolicies)
}

// GetPolicyExecCount returns the number of observations for execution time of adding policies.
// This function is slow.
func GetPolicyExecCount() (int, error) {
	return getCountValue(addPolicyExecTime)
}
