package ciliumpool

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/ipam"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	metricPrefix   = "azure_vnet_pool_"
	observationTTL = 30 * time.Second
	stageLabel     = "stage"
	kindLabel      = "kind"
)

type (
	HealthState     string
	AdmissionReason string
)

const (
	HealthUnknown               HealthState     = "unknown"
	HealthHealthy               HealthState     = "healthy"
	HealthUnhealthy             HealthState     = "unhealthy"
	AdmissionLeaseExpired       AdmissionReason = "lease_expired"
	AdmissionStorageUnavailable AdmissionReason = "storage_unavailable"
	AdmissionFenced             AdmissionReason = "fenced"
	AdmissionBootChanged        AdmissionReason = "boot_changed"
	AdmissionLeaseValid         AdmissionReason = "lease_valid"
)

type Observation struct {
	State       HealthState `json:"state"`
	ObservedAt  time.Time   `json:"observedAt"`
	LastSuccess time.Time   `json:"lastSuccess"`
	Error       string      `json:"error,omitempty"`
}

type Admission struct {
	State  HealthState     `json:"state"`
	Reason AdmissionReason `json:"reason"`
}

type HealthSnapshot struct {
	Admission        Admission              `json:"admission"`
	Local            ipam.CiliumPoolStatus  `json:"local"`
	Observations     map[string]Observation `json:"observations"`
	Pool             int                    `json:"pool"`
	BootMatches      bool                   `json:"bootMatches"`
	CredentialExpiry time.Time              `json:"credentialExpiry"`
	Policy           HealthState            `json:"policy"`
	Dataplane        HealthState            `json:"dataplane"`
}

// Monitor publishes observations only; it does not change allocation decisions.
type Monitor struct {
	mu        sync.RWMutex
	snapshot  HealthSnapshot
	now       func() time.Time
	reserve   int
	registry  *prometheus.Registry
	durations *prometheus.HistogramVec
	failures  *prometheus.CounterVec
}

var stages = []string{"reconcile", "guest", "node_api", "pool_api", "qualified_pool", "usage", "publication", "storage", "routes", "credentials"}

func NewMonitor(reserve int) *Monitor {
	m := &Monitor{
		now: time.Now, reserve: reserve, registry: prometheus.NewRegistry(),
		snapshot: HealthSnapshot{Observations: make(map[string]Observation), Policy: HealthUnknown, Dataplane: HealthUnknown},
		durations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricPrefix + "operation_duration_seconds",
			Help:    "Bridge operation latency, including failed operations.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation", "outcome"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricPrefix + "errors_total", Help: "Bridge errors by bounded operation and reason.",
		}, []string{"operation", "reason"}),
	}
	for _, stage := range stages {
		m.snapshot.Observations[stage] = Observation{State: HealthUnknown}
	}
	m.registry.MustRegister(m, m.durations, m.failures)
	return m
}

func (m *Monitor) Observe(operation string, duration time.Duration, err error) {
	if m == nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
		reason := "failure"
		switch {
		case apierrors.IsUnauthorized(err):
			reason = "unauthorized"
		case apierrors.IsForbidden(err):
			reason = "forbidden"
		case apierrors.IsTimeout(err), errors.Is(err, http.ErrHandlerTimeout):
			reason = "timeout"
		}
		m.failures.WithLabelValues(operation, reason).Inc()
	}
	m.durations.WithLabelValues(operation, outcome).Observe(duration.Seconds())
	m.mu.Lock()
	defer m.mu.Unlock()
	previous, found := m.snapshot.Observations[operation]
	if !found {
		return
	}
	previous.ObservedAt = m.now()
	previous.State, previous.Error = HealthHealthy, ""
	if err != nil {
		previous.State, previous.Error = HealthUnhealthy, err.Error()
	} else {
		previous.LastSuccess = previous.ObservedAt
	}
	m.snapshot.Observations[operation] = previous
}

func (m *Monitor) setLocal(status ipam.CiliumPoolStatus, bootID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshot.Local = status
	m.snapshot.BootMatches = status.State.BootID == bootID && bootID != ""
}

func (m *Monitor) setPool(pool int) {
	if m != nil {
		m.mu.Lock()
		m.snapshot.Pool = pool
		m.mu.Unlock()
	}
}

func (m *Monitor) Snapshot() HealthSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.snapshot
	s.Local.Addresses = slices.Clone(m.snapshot.Local.Addresses)
	s.Observations = make(map[string]Observation, len(m.snapshot.Observations))
	now := m.now()
	for name, observation := range m.snapshot.Observations {
		if observation.ObservedAt.IsZero() || now.Sub(observation.ObservedAt) > observationTTL {
			observation.State = HealthUnknown
		}
		s.Observations[name] = observation
	}
	s.Admission = Admission{State: HealthUnhealthy, Reason: AdmissionLeaseExpired}
	switch {
	case s.Observations["storage"].State != HealthHealthy:
		s.Admission = Admission{State: HealthUnknown, Reason: AdmissionStorageUnavailable}
	case s.Local.State.FenceReason != "":
		s.Admission.Reason = AdmissionFenced
	case !s.BootMatches:
		s.Admission.Reason = AdmissionBootChanged
	case !now.After(s.Local.State.ReadyUntil):
		s.Admission = Admission{State: HealthHealthy, Reason: AdmissionLeaseValid}
	}
	return s
}

func (m *Monitor) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		status := m.Snapshot().Admission
		w.Header().Set("Content-Type", "application/json")
		if status.State != HealthHealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m.Snapshot())
	})
	return mux
}

func (m *Monitor) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
}

func (m *Monitor) Collect(ch chan<- prometheus.Metric) {
	s := m.Snapshot()
	gauge := func(name, help string, value float64, labels []string, values ...string) {
		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(metricPrefix+name, help, labels, nil), prometheus.GaugeValue, value, values...)
	}
	flag := func(value bool) float64 {
		if value {
			return 1
		}
		return 0
	}
	timestamp := func(t time.Time) float64 {
		if t.IsZero() {
			return 0
		}
		return float64(t.Unix()) + float64(t.Nanosecond())/float64(time.Second)
	}
	gauge("admission_ready", "Whether the observed local admission lease is valid; not dataplane health.", flag(s.Admission.State == HealthHealthy), nil)
	gauge("lease_expires_timestamp_seconds", "Committed admission lease expiry.", timestamp(s.Local.State.ReadyUntil), nil)
	gauge("boot_matches", "Whether the committed lease belongs to this boot.", flag(s.BootMatches), nil)
	gauge("fenced", "Whether the last observed committed pool state has a persistent fence.", flag(s.Local.State.FenceReason != ""), nil)
	reason := "none"
	switch s.Local.State.FenceReason {
	case "":
	case "identity changed", "node identity or consumer changed", "cilium node UID changed":
		reason = "identity_changed"
	case "cilium node deleted":
		reason = "node_deleted"
	case "unexpected cloud release handshake":
		reason = "unsupported_release"
	case "network state missing":
		reason = "network_state_missing"
	default:
		reason = "other"
	}
	gauge("fence_reason", "Bounded persistent fence reason; detailed reason is in status.", 1, []string{"reason"}, reason)
	for stage, observation := range s.Observations {
		gauge("observation_healthy", "Health of the latest observation; unknown is not healthy.", flag(observation.State == HealthHealthy), []string{stageLabel}, stage)
		gauge("observation_known", "Whether the observation is recent enough to interpret.", flag(observation.State != HealthUnknown), []string{stageLabel}, stage)
		gauge("last_success_timestamp_seconds", "Last successful observation; zero means never observed.", timestamp(observation.LastSuccess), []string{stageLabel}, stage)
	}
	for name, count := range map[string]int{
		"pool": s.Pool, "eligible": s.Local.Eligible, "free": s.Local.Free,
		"used": s.Local.Used, "quarantined": s.Local.Quarantined,
		"reserve_deficit": max(0, m.reserve-s.Local.Free),
	} {
		gauge("addresses", "Last observed address counts; consult storage and qualified_pool freshness.", float64(count), []string{kindLabel}, name)
	}
	gauge("credential_expiry_known", "Whether the current credential expiry is observable.", flag(!s.CredentialExpiry.IsZero()), nil)
	gauge("credential_expires_timestamp_seconds", "Current credential expiry; zero means unknown, not non-expiring.", timestamp(s.CredentialExpiry), nil)
	gauge("policy_known", "Policy convergence is supplied by the controller, not inferred by the bridge.", 0, nil)
	gauge("dataplane_known", "Reachability is supplied by pod-originated probes, not inferred by the bridge.", 0, nil)
}
