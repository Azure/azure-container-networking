package metrics

import (
	"net/http"

	"github.com/Azure/azure-container-networking/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "npm"

// Prometheus Metrics
// Gauge metrics have the methods Inc(), Dec(), and Set(float64)
// Summary metrics has the method Observe(float64)
// For any Vector metric, you can call With(prometheus.Labels) before the above methods
//   e.g. SomeGaugeVec.With(prometheus.Labels{label1: val1, label2: val2, ...).Dec()
var (
	NumPolicies            prometheus.Gauge
	AddPolicyExecTime      prometheus.Summary
	NumIPTableRules        prometheus.Gauge
	AddIPTableRuleExecTime prometheus.Summary
	NumIPSets              prometheus.Gauge
	AddIPSetExecTime       prometheus.Summary
	NumIPSetEntries        prometheus.Gauge

	// IPSetInventory should not be referenced directly. Use the functions in ipset-inventory.go
	IPSetInventory *prometheus.GaugeVec
)

// Constants for metric names and descriptions as well as exported labels for Vector metrics
const (
	numPoliciesName = "num_policies"
	numPoliciesHelp = "The number of current network policies for this node"

	addPolicyExecTimeName = "add_policy_exec_time"
	addPolicyExecTimeHelp = "Execution time in milliseconds for adding a network policy"

	numIPTableRulesName = "num_iptables_rules"
	numIPTableRulesHelp = "The number of current IPTable rules for this node"

	addIPTableRuleExecTimeName = "add_iptables_rule_exec_time"
	addIPTableRuleExecTimeHelp = "Execution time in milliseconds for adding an IPTable rule to a chain"

	numIPSetsName = "num_ipsets"
	numIPSetsHelp = "The number of current IP sets for this node"

	addIPSetExecTimeName = "add_ipset_exec_time"
	addIPSetExecTimeHelp = "Execution time in milliseconds for creating an IP set"

	numIPSetEntriesName = "num_ipset_entries"
	numIPSetEntriesHelp = "The total number of entries in every IPSet"

	ipsetInventoryName = "ipset_counts"
	ipsetInventoryHelp = "The number of entries in each individual IPSet"
	SetNameLabel       = "set_name"
	SetHashLabel       = "set_hash"
)

var nodeLevelRegistry = prometheus.NewRegistry()
var clusterLevelRegistry = prometheus.NewRegistry()
var haveInitialized = false

func ReInitializeAllMetrics() {
	haveInitialized = false
	InitializeAll()
}

// InitializeAll creates all the Prometheus Metrics. The metrics will be nil before this method is called.
func InitializeAll() {
	if !haveInitialized {
		NumPolicies = createGauge(numPoliciesName, numPoliciesHelp, false)
		AddPolicyExecTime = createSummary(addPolicyExecTimeName, addPolicyExecTimeHelp, true)
		NumIPTableRules = createGauge(numIPTableRulesName, numIPTableRulesHelp, false)
		AddIPTableRuleExecTime = createSummary(addIPTableRuleExecTimeName, addIPTableRuleExecTimeHelp, true)
		NumIPSets = createGauge(numIPSetsName, numIPSetsHelp, false)
		AddIPSetExecTime = createSummary(addIPSetExecTimeName, addIPSetExecTimeHelp, true)
		NumIPSetEntries = createGauge(numIPSetEntriesName, numIPSetEntriesHelp, false)
		IPSetInventory = createGaugeVec(ipsetInventoryName, ipsetInventoryHelp, false, SetNameLabel, SetHashLabel)
		log.Logf("Finished initializing all Prometheus metrics")
		haveInitialized = true
	}
}

// getHandler returns the HTTP handler for the metrics endpoint
func GetHandler(isNodeLevel bool) http.Handler {
	return promhttp.HandlerFor(GetRegistry(isNodeLevel), promhttp.HandlerOpts{})
}

func register(collector prometheus.Collector, name string, isNodeLevel bool) {
	err := GetRegistry(isNodeLevel).Register(collector)
	if err != nil {
		log.Errorf("Error creating metric %s", name)
	}
}

func GetRegistry(isNodeLevel bool) *prometheus.Registry {
	registry := clusterLevelRegistry
	if isNodeLevel {
		registry = nodeLevelRegistry
	}
	return registry
}

func createGauge(name string, helpMessage string, isNodeLevel bool) prometheus.Gauge {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
			Help:      helpMessage,
		},
	)
	register(gauge, name, isNodeLevel)
	return gauge
}

func createGaugeVec(name string, helpMessage string, isNodeLevel bool, labels ...string) *prometheus.GaugeVec {
	gaugeVec := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
			Help:      helpMessage,
		},
		labels,
	)
	register(gaugeVec, name, isNodeLevel)
	return gaugeVec
}

func createSummary(name string, helpMessage string, isNodeLevel bool) prometheus.Summary {
	summary := prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       name,
			Help:       helpMessage,
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			// quantiles e.g. the "0.5 quantile" will actually be the phi quantile for some phi in [0.5 - 0.05, 0.5 + 0.05]
		},
	)
	register(summary, name, isNodeLevel)
	return summary
}
