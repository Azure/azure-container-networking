package restserver

import (
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	subnetLabel              = "subnet"
	subnetCIDRLabel          = "subnet_cidr"
	podnetARMIDLabel         = "podnet_arm_id"
	cnsReturnCode            = "cns_return_code"
	customerMetricLabel      = "customer_metric"
	customerMetricLabelValue = "customer metric"
)

var (
	HTTPRequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_request_latency_seconds",
			Help: "Request latency in seconds by endpoint, verb, and response code.",
			//nolint:gomnd // default bucket consts
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1 ms to ~16 seconds
		},
		[]string{"url", "verb", "cns_return_code"},
	)
	ipAssignmentLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "ip_assignment_latency_seconds",
			Help: "Pod IP assignment latency in seconds",
			//nolint:gomnd // default bucket consts
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1 ms to ~16 seconds
		},
	)
	ipConfigStatusStateTransitionTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "ipconfigstatus_state_transition_seconds",
			Help: "Time spent by the IP Configuration Status in each state transition",
			//nolint:gomnd // default bucket consts
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1 ms to ~16 seconds
		},
		[]string{"previous_state", "next_state"},
	)
	syncHostNCVersionCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sync_host_nc_version_total",
			Help: "Count of Sync Host NC by success or failure",
		},
		[]string{"ok"},
	)
	syncHostNCVersionLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "sync_host_nc_version_latency_seconds",
			Help: "Sync Host NC Latency",
			//nolint:gomnd // default bucket consts
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1 ms to ~16 seconds
		},
		[]string{"ok"},
	)
	allocatedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_allocated_ips_v2",
			Help:        "Count of IPs CNS has Allocated",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{},
	)
	assignedIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_assigned_ips_v2",
			Help:        "Count of IPs CNS has Assigned to Pods",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{},
	)
	availableIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_available_ips_v2",
			Help:        "Count of IPs Available",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{},
	)
	pendingProgrammingIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_pending_programming_ips_v2",
			Help:        "Count of IPs in Pending Programming State",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{},
	)
	pendingReleaseIPCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "cx_pending_release_ips_v2",
			Help:        "Count of IPs in Pending Release State",
			ConstLabels: prometheus.Labels{customerMetricLabel: customerMetricLabelValue},
		},
		[]string{},
	)
)

func init() {
	metrics.Registry.MustRegister(
		HTTPRequestLatency,
		ipAssignmentLatency,
		ipConfigStatusStateTransitionTime,
		syncHostNCVersionCount,
		syncHostNCVersionLatency,
		allocatedIPCount,
		assignedIPCount,
		availableIPCount,
		pendingProgrammingIPCount,
		pendingReleaseIPCount,
	)
}

// Every http response is 200 so we really want cns  response code.
// Hard tto do with middleware unless we derserialize the responses but making it an explit header works around it.
// if that doesn't work we could have a separate countervec just for response codes.
func NewHandlerFuncWithHistogram(handler http.HandlerFunc, histogram *prometheus.HistogramVec) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		defer func() {
			code := w.Header().Get(cnsReturnCode)
			histogram.WithLabelValues(req.URL.RequestURI(), req.Method, code).Observe(time.Since(start).Seconds())
		}()
		handler(w, req)
	}
}

func stateTransitionMiddleware(i *cns.IPConfigurationStatus, s types.IPState) {
	// if no state transition has been recorded yet, don't collect any metric
	if i.LastStateTransition.IsZero() {
		return
	}
	ipConfigStatusStateTransitionTime.WithLabelValues(string(i.GetState()), string(s)).Observe(time.Since(i.LastStateTransition).Seconds())
}

type ipState struct {
	// allocatedIPs are all the IPs given to CNS by DNC.
	allocatedIPs int64
	// assignedIPs are the IPs CNS gives to Pods.
	assignedIPs int64
	// availableIPs are the IPs in state "Available".
	availableIPs int64
	// programmingIPs are the IPs in state "PendingProgramming".
	programmingIPs int64
	// releasingIPs are the IPs in state "PendingReleasr".
	releasingIPs int64
}

type asyncMetricsRecorder struct {
	podIPConfigSrc func() map[string]cns.IPConfigurationStatus
	sig            chan struct{}
	once           sync.Once
}

// singleton recorder
var recorder asyncMetricsRecorder

// run starts the asyncMetricsRecorder and listens for signals to record the metrics.
func (a *asyncMetricsRecorder) run() {
	for range a.sig {
		a.record()
	}
}

// record records the IP Config state metrics to Prometheus.
func (a *asyncMetricsRecorder) record() {
	var state ipState
	for ipConfig := range maps.Values(a.podIPConfigSrc()) {
		state.allocatedIPs++
		if ipConfig.GetState() == types.Assigned {
			state.assignedIPs++
		}
		if ipConfig.GetState() == types.Available {
			state.availableIPs++
		}
		if ipConfig.GetState() == types.PendingProgramming {
			state.programmingIPs++
		}
		if ipConfig.GetState() == types.PendingRelease {
			state.releasingIPs++
		}
	}

	logger.Printf("Allocated IPs: %d, Assigned IPs: %d, Available IPs: %d, PendingProgramming IPs: %d, PendingRelease IPs: %d",
		state.allocatedIPs,
		state.assignedIPs,
		state.availableIPs,
		state.programmingIPs,
		state.releasingIPs,
	)

	labels := []string{}
	allocatedIPCount.WithLabelValues(labels...).Set(float64(state.allocatedIPs))
	assignedIPCount.WithLabelValues(labels...).Set(float64(state.assignedIPs))
	availableIPCount.WithLabelValues(labels...).Set(float64(state.availableIPs))
	pendingProgrammingIPCount.WithLabelValues(labels...).Set(float64(state.programmingIPs))
	pendingReleaseIPCount.WithLabelValues(labels...).Set(float64(state.releasingIPs))
}

// publishIPStateMetrics logs and publishes the IP Config state metrics to Prometheus.
func (service *HTTPRestService) publishIPStateMetrics() {
	recorder.once.Do(func() {
		recorder.podIPConfigSrc = service.PodIPConfigStates
		recorder.sig = make(chan struct{})
		go recorder.run()
	})
	select {
	case recorder.sig <- struct{}{}: // signal the recorder to record the metrics
	default: // drop the signal if the recorder already has an event queued
	}
}

// PodIPConfigStates returns a clone of the IP Config State map.
func (service *HTTPRestService) PodIPConfigStates() map[string]cns.IPConfigurationStatus {
	// copy state
	service.RLock()
	defer service.RUnlock()
	return maps.Clone(service.PodIPConfigState)
}
