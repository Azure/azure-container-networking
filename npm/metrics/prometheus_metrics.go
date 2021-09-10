package metrics

import (
	"fmt"
	"net/http"

	"github.com/Azure/azure-container-networking/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

const namespace = "npm"

// Prometheus Metrics
// Gauge metrics have the methods Inc(), Dec(), and Set(float64)
// Summary metrics have the method Observe(float64)
// For any Vector metric, you can call With(prometheus.Labels) before the above methods
//   e.g. SomeGaugeVec.With(prometheus.Labels{label1: val1, label2: val2, ...).Dec()
var (
	numPolicies        prometheus.Gauge
	addPolicyExecTime  prometheus.Summary
	numACLRules        prometheus.Gauge
	addACLRuleExecTime prometheus.Summary
	numIPSets          prometheus.Gauge
	addIPSetExecTime   prometheus.Summary
	numIPSetEntries    prometheus.Gauge
	ipsetInventory     *prometheus.GaugeVec
)

// Constants for metric names and descriptions as well as exported labels for Vector metrics
const (
	numPoliciesName = "num_policies"
	numPoliciesHelp = "The number of current network policies for this node"

	addPolicyExecTimeName = "add_policy_exec_time"
	addPolicyExecTimeHelp = "Execution time in milliseconds for adding a network policy"

	numACLRulesName = "num_iptables_rules"
	numACLRulesHelp = "The number of current IPTable rules for this node"

	addACLRuleExecTimeName = "add_iptables_rule_exec_time"
	addACLRuleExecTimeHelp = "Execution time in milliseconds for adding an IPTable rule to a chain"

	numIPSetsName = "num_ipsets"
	numIPSetsHelp = "The number of current IP sets for this node"

	addIPSetExecTimeName = "add_ipset_exec_time"
	addIPSetExecTimeHelp = "Execution time in milliseconds for creating an IP set"

	numIPSetEntriesName = "num_ipset_entries"
	numIPSetEntriesHelp = "The total number of entries in every IPSet"

	ipsetInventoryName = "ipset_counts"
	ipsetInventoryHelp = "The number of entries in each individual IPSet"
	setNameLabel       = "set_name"
	setHashLabel       = "set_hash"
)

var (
	nodeLevelRegistry    = prometheus.NewRegistry()
	clusterLevelRegistry = prometheus.NewRegistry()
	haveInitialized      = false
)

// InitializeAll creates all the Prometheus Metrics. The metrics will be nil before this method is called.
func InitializeAll() {
	if !haveInitialized {
		numPolicies = createGauge(numPoliciesName, numPoliciesHelp, false)
		addPolicyExecTime = createSummary(addPolicyExecTimeName, addPolicyExecTimeHelp, true)
		numACLRules = createGauge(numACLRulesName, numACLRulesHelp, false)
		addACLRuleExecTime = createSummary(addACLRuleExecTimeName, addACLRuleExecTimeHelp, true)
		numIPSets = createGauge(numIPSetsName, numIPSetsHelp, false)
		addIPSetExecTime = createSummary(addIPSetExecTimeName, addIPSetExecTimeHelp, true)
		numIPSetEntries = createGauge(numIPSetEntriesName, numIPSetEntriesHelp, false)
		ipsetInventory = createGaugeVec(ipsetInventoryName, ipsetInventoryHelp, false, setNameLabel, setHashLabel)
		log.Logf("Finished initializing all Prometheus metrics")
		haveInitialized = true
	}
}

// GetHandler returns the HTTP handler for the metrics endpoint
func GetHandler(isNodeLevel bool) http.Handler {
	return promhttp.HandlerFor(getRegistry(isNodeLevel), promhttp.HandlerOpts{})
}

func register(collector prometheus.Collector, name string, isNodeLevel bool) {
	err := getRegistry(isNodeLevel).Register(collector)
	if err != nil {
		log.Errorf("Error creating metric %s", name)
	}
}

func getRegistry(isNodeLevel bool) *prometheus.Registry {
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

// getValue returns a Gauge metric's value.
// This function is slow.
func getValue(gaugeMetric prometheus.Gauge) (int, error) {
	dtoMetric, err := getDTOMetric(gaugeMetric)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Gauge.GetValue()), nil
}

// getVecValue Gauge Vec metric's value, or 0 if the label doesn't exist for the metric.
// This function is slow.
func getVecValue(gaugeVecMetric *prometheus.GaugeVec, labels prometheus.Labels) (int, error) {
	return getValue(gaugeVecMetric.With(labels))
}

// getCountValue the number of times a Summary metric has recorded an observation.
// This function is slow.
func getCountValue(summaryMetric prometheus.Summary) (int, error) {
	dtoMetric, err := getDTOMetric(summaryMetric)
	if err != nil {
		return 0, err
	}
	return int(dtoMetric.Summary.GetSampleCount()), nil
}

func getDTOMetric(collector prometheus.Collector) (*dto.Metric, error) {
	channel := make(chan prometheus.Metric, 1)
	collector.Collect(channel)
	metric := &dto.Metric{}
	err := (<-channel).Write(metric)
	if err != nil {
		err = fmt.Errorf("error while extracting Prometheus metric value: %w", err)
	}
	return metric, err
}
