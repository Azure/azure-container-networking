package metric

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// IP allocations and deallocations go through the NodeNetworkConfig
// CRD, meaning they are asynchronous and eventually consistent, and have
// no guarantees on the order they are processed in. It's currently
// impractical to include an identifier with these requests that could
// be used to correlate the request with the result.
//
// The public functions in this file are a way to get some (dirty, incomplete)
// data on the amount of time it takes from when we request more or less IPs
// to when we receive an updated NNC CRD with more or less IPs.
//
// We get this data by recording a start time when we push an alloc/dealloc
// request via the NNC, and then popping any alloc/dealloc start times that
// we have saved as soon we get an updated NNC.
// If we have not received a new NNC, multiple requests to start an
// alloc/dealloc timer will noop - we only record the longest span between
// pushing an NNC spec and receiving an updated NNC status.
//
// This data will be left-skewed: if there is no in-flight alloc or dealloc,
// and we queue one right before receiving an NNC status update, we have no
// way to decorrelate that update with the in-flight requests and will record
// a very short response latency.

var incLatency prometheus.ObserverVec = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		//nolint:gomnd
		Buckets: prometheus.ExponentialBuckets(0.05, 2, 15), // 50 ms to ~800 seconds
		Help:    "IP pool size increase latency in seconds by batch size",
		Name:    "ip_pool_inc_latency_seconds",
	},
	[]string{"batch"},
)

var decLatency prometheus.ObserverVec = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		//nolint:gomnd
		Buckets: prometheus.ExponentialBuckets(0.05, 2, 15), // 50 ms to ~800 seconds
		Help:    "IP pool size decrease latency in seconds by batch size",
		Name:    "ip_pool_dec_latency_seconds",
	},
	[]string{"batch"},
)

type scaleEvent struct {
	start time.Time
	batch int
}

var (
	incEvents = make(chan scaleEvent, 1)
	decEvents = make(chan scaleEvent, 1)
)

// StartPoolIncreaseTimer records the start of an IP allocation request.
// If an IP allocation request is already in flight, this method noops.
func StartPoolIncreaseTimer(batch int) {
	e := scaleEvent{
		start: time.Now(),
		batch: batch,
	}
	select {
	case incEvents <- e:
	default:
	}
}

// StartPoolDecreaseTimer records the start of an IP deallocation request.
// If an IP deallocation request is already in flight, this method noops.
func StartPoolDecreaseTimer(batch int) {
	e := scaleEvent{
		start: time.Now(),
		batch: batch,
	}
	select {
	case decEvents <- e:
	default:
	}
}

// ObserverPoolScaleLatency records the elapsed interval since the oldest
// unobserved allocation and deallocation requests. If there are no recorded
// request starts, this method noops.
func ObserverPoolScaleLatency() {
	select {
	case e := <-incEvents:
		incLatency.WithLabelValues(strconv.Itoa(e.batch)).Observe(time.Since(e.start).Seconds())
	default:
	}

	select {
	case e := <-decEvents:
		decLatency.WithLabelValues(strconv.Itoa(e.batch)).Observe(time.Since(e.start).Seconds())
	default:
	}
}

func init() {
	metrics.Registry.MustRegister(
		incLatency,
		decLatency,
	)
}
