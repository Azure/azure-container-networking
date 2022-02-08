package metrics

import (
	"net/http"

	"github.com/Azure/azure-container-networking/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
)

// Constants for metric names and descriptions as well as exported labels for Vector metrics
const (
	namespace        = "npm"
	controllerPrefix = "controller"

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

	// perf metrics added after v1.4.16
	// all these metrics have "npm_controller_" prepended to their name
	operationLabel = "operation"
	hadErrorLabel  = "had_error"

	policyExecTimeName           = "policy_exec_time"
	controllerPolicyExecTimeHelp = "Execution time in milliseconds for updating/deleting a network policy. NOTE: for adding, see npm_add_policy_exec_time"

	podExecTimeName           = "pod_exec_time"
	controllerPodExecTimeHelp = "Execution time in milliseconds for adding/updating/deleting a pod"

	namespaceExecTimeName           = "namespace_exec_time"
	controllerNamespaceExecTimeHelp = "Execution time in milliseconds for adding/updating/deleting a namespace"

	// TODO add health metrics

	quantileMedian float64 = 0.5
	deltaMedian    float64 = 0.05
	quantile90th   float64 = 0.9
	delta90th      float64 = 0.01
	quantil99th    float64 = 0.99
	delta99th      float64 = 0.001
)

// Gauge metrics have the methods Inc(), Dec(), and Set(float64)
// Summary metrics have the method Observe(float64)
// For any Vector metric, you can call With(prometheus.Labels) before the above methods
//   e.g. SomeGaugeVec.With(prometheus.Labels{label1: val1, label2: val2, ...).Dec()
var (
	nodeRegistry    = prometheus.NewRegistry()
	clusterRegistry = prometheus.NewRegistry()
	haveInitialized = false

	// quantiles e.g. the "0.5 quantile" with delta 0.05 will actually be the phi quantile for some phi in [0.5 - 0.05, 0.5 + 0.05]
	execTimeQuantiles = map[float64]float64{quantileMedian: deltaMedian, quantile90th: delta90th, quantil99th: delta99th}

	numPolicies          prometheus.Gauge
	numACLRules          prometheus.Gauge
	addACLRuleExecTime   prometheus.Summary
	numIPSets            prometheus.Gauge
	addIPSetExecTime     prometheus.Summary
	numIPSetEntries      prometheus.Gauge
	ipsetInventory       *prometheus.GaugeVec
	ipsetInventoryLabels = []string{setNameLabel, setHashLabel}

	// controller perf metrics
	// used to be a regular Summary in v1.4.16 and below
	addPolicyExecTime       *prometheus.SummaryVec
	addPolicyExecTimeLabels = []string{hadErrorLabel}
	// metrics added after v1.4.16
	controllerPolicyExecTime    *prometheus.SummaryVec
	controllerPodExecTime       *prometheus.SummaryVec
	controllerNamespaceExecTime *prometheus.SummaryVec
	controllerExecTimeLabels    = []string{operationLabel, hadErrorLabel}

	// TODO add health metrics
)

type RegistryType string

const (
	NodeMetrics    RegistryType = "node-metrics"
	ClusterMetrics RegistryType = "cluster-metrics"
)

type OperationKind string

const (
	CreateOp OperationKind = "create"
	UpdateOp OperationKind = "update"
	DeleteOp OperationKind = "delete"
	NoOp     OperationKind = "noop"
)

func (op OperationKind) isValid() bool {
	switch op {
	case CreateOp, UpdateOp, DeleteOp, NoOp:
		return true
	default:
		return false
	}
}

// InitializeAll creates the Controller and Daemon Prometheus Metrics.
// The metrics will be nil before this method is called.
// TODO consider refactoring the functionality of the metrics package into a "Metrics" struct with methods (this would require code changes throughout npm).
// Would need to consider how it seems like you can't register a metric twice, even in a separate registry, so you couldn't throw away the Metrics struct and create a new one.
func InitializeAll() {
	// TODO introduce isFanOut parameter to determine when to create fan-out controller/daemon metrics
	if haveInitialized {
		klog.Infof("metrics have already been initialized")
	} else {
		initializeDaemonMetrics()
		initializeControllerMetrics()
		// TODO include dataplane health metrics:
		// num failures for apply ipsets, updating policies, deleting policies, and running periodic policy tasks, etc.
		log.Logf("Finished initializing all Prometheus metrics")
		haveInitialized = true
	}
}

// ReinitializeAll creates/replaces Prometheus metrics.
// This function is intended for UTs.
func ReinitializeAll() {
	klog.Infof("reinitializing Prometheus metrics. This may cause error messages of the form: 'error creating metric' from trying to re-register each metric")
	haveInitialized = false
	InitializeAll()
}

// GetHandler returns the HTTP handler for the metrics endpoint
func GetHandler(registryType RegistryType) http.Handler {
	if !haveInitialized {
		// not sure if this will ever happen, but just in case
		klog.Infof("in GetHandler, metrics weren't initialized. Initializing now")
		InitializeAll()
	}
	return promhttp.HandlerFor(getRegistry(registryType), promhttp.HandlerOpts{})
}

// initializeDaemonMetrics creates non-controller metrics
func initializeDaemonMetrics() {
	// CLUSTER METRICS
	numACLRules = createClusterGauge(numACLRulesName, numACLRulesHelp)
	numIPSets = createClusterGauge(numIPSetsName, numIPSetsHelp)
	numIPSetEntries = createClusterGauge(numIPSetEntriesName, numIPSetEntriesHelp)
	ipsetInventory = createClusterGaugeVec(ipsetInventoryName, ipsetInventoryHelp, ipsetInventoryLabels)
	ipsetInventoryMap = make(map[string]int)

	// NODE METRICS
	addACLRuleExecTime = createNodeSummary(addACLRuleExecTimeName, addACLRuleExecTimeHelp)
	addIPSetExecTime = createNodeSummary(addIPSetExecTimeName, addIPSetExecTimeHelp)
}

// initializeControllerMetrics creates metrics modified by the controller
func initializeControllerMetrics() {
	// CLUSTER METRICS
	numPolicies = createClusterGauge(numPoliciesName, numPoliciesHelp)
	// TODO include health metrics: num failures for validating policies & ipsets

	// NODE METRICS
	addPolicyExecTime = createNodeSummaryVec(addPolicyExecTimeName, "", addPolicyExecTimeHelp, addPolicyExecTimeLabels)

	// perf metrics added after v1.4.16
	// all these metrics have "npm_controller_" prepended to their name
	controllerPolicyExecTime = createControllerExecTimeSummaryVec(policyExecTimeName, controllerPolicyExecTimeHelp)
	controllerPodExecTime = createControllerExecTimeSummaryVec(podExecTimeName, controllerPodExecTimeHelp)
	controllerNamespaceExecTime = createControllerExecTimeSummaryVec(namespaceExecTimeName, controllerNamespaceExecTimeHelp)
}

func register(collector prometheus.Collector, name string, registryType RegistryType) {
	err := getRegistry(registryType).Register(collector)
	if err != nil {
		log.Errorf("Error creating metric %s", name)
	}
}

func getRegistry(registryType RegistryType) *prometheus.Registry {
	if registryType == NodeMetrics {
		return nodeRegistry
	}
	return clusterRegistry
}

func createClusterGauge(name, helpMessage string) prometheus.Gauge {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
			Help:      helpMessage,
		},
	)
	register(gauge, name, ClusterMetrics)
	return gauge
}

func createClusterGaugeVec(name, helpMessage string, labels []string) *prometheus.GaugeVec {
	gaugeVec := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
			Help:      helpMessage,
		},
		labels,
	)
	register(gaugeVec, name, ClusterMetrics)
	return gaugeVec
}

func createNodeSummary(name, helpMessage string) prometheus.Summary {
	// uses default observation TTL of 10 minutes
	summary := prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       name,
			Help:       helpMessage,
			Objectives: execTimeQuantiles,
		},
	)
	register(summary, name, NodeMetrics)
	return summary
}

func createNodeSummaryVec(name, subsystem, helpMessage string, labels []string) *prometheus.SummaryVec {
	// uses default observation TTL of 10 minutes
	summary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  subsystem,
			Name:       name,
			Help:       helpMessage,
			Objectives: execTimeQuantiles,
		},
		labels,
	)
	register(summary, name, NodeMetrics)
	return summary
}

func createControllerExecTimeSummaryVec(name, helpMessage string) *prometheus.SummaryVec {
	return createNodeSummaryVec(name, controllerPrefix, helpMessage, controllerExecTimeLabels)
}
