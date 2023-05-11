package metrics

import (
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/prometheus/client_golang/prometheus"
)

// IncNumACLRules increments the number of ACL rules.
func IncNumACLRules() {
	numACLRules.Inc()
}

// IncNumACLRulesBy increments the number of ACL rules by the amount.
func IncNumACLRulesBy(amount int) {
	numACLRules.Add(float64(amount))
}

// DecNumACLRules decrements the number of ACL rules.
func DecNumACLRules() {
	numACLRules.Dec()
}

// DecNumACLRulesBy decrements the number of ACL rules by the amount.
func DecNumACLRulesBy(amount int) {
	numACLRules.Add(float64(-amount))
}

// ResetNumACLRules sets the number of ACL rules to 0.
func ResetNumACLRules() {
	numACLRules.Set(0)
}

// RecordACLRuleExecTime adds an observation of execution time for adding an ACL rule.
// The execution time is from the timer's start until now.
// NOTE: RecordACLLatency is more precise in Windows.
func RecordACLRuleExecTime(timer *Timer) {
	timer.stopAndRecord(addACLRuleExecTime)
}

// GetNumACLRules returns the number of ACL rules.
// This function is slow.
func GetNumACLRules() (int, error) {
	return getValue(numACLRules)
}

// GetACLRuleExecCount returns the number of observations for execution time of adding ACL rules.
// This function is slow.
func GetACLRuleExecCount() (int, error) {
	return getCountValue(addACLRuleExecTime)
}

// RecordACLLatency should be used in Windows DP to record the latency of individual ACL operations.
func RecordACLLatency(timer *Timer, op OperationKind) {
	if util.IsWindowsDP() {
		labels := prometheus.Labels{
			operationLabel: string(op),
		}
		aclLatency.With(labels).Observe(timer.timeElapsed())
	}
}

// IncACLFailures should be used in Windows DP to record the number of failures for individual ACL operations.
func IncACLFailures(op OperationKind) {
	if util.IsWindowsDP() {
		labels := prometheus.Labels{
			operationLabel: string(op),
		}
		aclFailures.With(labels).Inc()
	}
}
