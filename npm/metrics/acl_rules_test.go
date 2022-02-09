package metrics

import "testing"

var (
	numRulesMetric       = &basicMetric{ResetNumACLRules, IncNumACLRules, DecNumACLRules, GetNumACLRules}
	numRulesAmountMetric = &amountMetric{basicMetric: numRulesMetric, incBy: IncNumACLRulesBy, decBy: DecNumACLRulesBy}
	ruleExecMetric       = &recordingMetric{RecordACLRuleExecTime, GetACLRuleExecCount}
)

func TestRecordACLRuleExecTime(t *testing.T) {
	testStopAndRecord(t, ruleExecMetric)
}

func TestIncNumACLRules(t *testing.T) {
	testIncMetric(t, numRulesMetric)
}

func TestDecNumACLRules(t *testing.T) {
	testDecMetric(t, numRulesMetric)
}

func TestResetNumACLRules(t *testing.T) {
	testResetMetric(t, numRulesMetric)
}

func TestIncNumACLRulesBy(t *testing.T) {
	numRulesAmountMetric.testIncByMetric(t)
}

func TestDecNumACLRulesBy(t *testing.T) {
	numRulesAmountMetric.testDecByMetric(t)
}
