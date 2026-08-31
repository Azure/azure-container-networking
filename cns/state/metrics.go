// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type TransactionOperation string

const (
	TransactionView   TransactionOperation = "view"
	TransactionUpdate TransactionOperation = "update"
)

type LifecycleOperation string

const (
	LifecycleStartup  LifecycleOperation = "startup"
	LifecycleImport   LifecycleOperation = "import"
	LifecycleRollback LifecycleOperation = "rollback"
	LifecycleBoot     LifecycleOperation = "boot"
)

type OperationResult string

const (
	ResultSuccess  OperationResult = "success"
	ResultError    OperationResult = "error"
	ResultCanceled OperationResult = "canceled"
	ResultNoop     OperationResult = "noop"
	ResultConflict OperationResult = "conflict"
)

type InvariantName string

const (
	InvariantNone       InvariantName = "none"
	InvariantStructural InvariantName = "structural"
	InvariantSchema     InvariantName = "schema"
	InvariantAuthority  InvariantName = "authority"
)

type RecordType string

const (
	RecordNetworkContainer RecordType = "network_container"
	RecordIP               RecordType = "ip"
	RecordNetwork          RecordType = "network"
	RecordEndpoint         RecordType = "endpoint"
	RecordAssignment       RecordType = "assignment"
	RecordOwner            RecordType = "owner"
	RecordDeleteIntent     RecordType = "delete_intent"
)

const (
	metricLabelOperation  = "operation"
	metricLabelResult     = "result"
	metricLabelInvariant  = "invariant"
	metricLabelBackend    = "backend"
	metricLabelAuthority  = "authority"
	metricLabelSchema     = "schema"
	metricLabelRecordType = "record_type"
)

var (
	errNilMetricsRegisterer          = errors.New("persistent state metrics registerer is nil")
	errUnknownTransactionOperation   = errors.New("unknown persistent state transaction operation")
	errNegativeTransactionDuration   = errors.New("persistent state transaction duration is negative")
	errUnknownLifecycleOperation     = errors.New("unknown persistent state lifecycle operation")
	errNegativeLifecycleDuration     = errors.New("persistent state lifecycle duration is negative")
	errUnknownPersistentInvariant    = errors.New("unknown persistent state invariant")
	errUnknownPersistentStateBackend = errors.New("unknown persistent state backend")
	errUnknownOperationResult        = errors.New("unknown persistent state operation result")

	transactionOperations = map[TransactionOperation]struct{}{
		TransactionView: {}, TransactionUpdate: {},
	}
	lifecycleOperations = map[LifecycleOperation]struct{}{
		LifecycleStartup: {}, LifecycleImport: {}, LifecycleRollback: {}, LifecycleBoot: {},
	}
	operationResults = map[OperationResult]struct{}{
		ResultSuccess: {}, ResultError: {}, ResultCanceled: {}, ResultNoop: {}, ResultConflict: {},
	}
	invariantNames = map[InvariantName]struct{}{
		InvariantStructural: {}, InvariantSchema: {}, InvariantAuthority: {},
	}
	recordTypes = []RecordType{
		RecordNetworkContainer,
		RecordIP,
		RecordNetwork,
		RecordEndpoint,
		RecordAssignment,
		RecordOwner,
		RecordDeleteIntent,
	}
)

type Metrics struct {
	transactions      *prometheus.CounterVec
	transactionTime   *prometheus.HistogramVec
	lifecycle         *prometheus.CounterVec
	lifecycleTime     *prometheus.HistogramVec
	invariantFailures *prometheus.CounterVec
	info              *prometheus.GaugeVec
	generation        prometheus.Gauge
	storagePresent    prometheus.Gauge
	databaseBytes     prometheus.Gauge
	records           *prometheus.GaugeVec
	now               func() time.Time
}

func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	if registerer == nil {
		return nil, errNilMetricsRegisterer
	}

	durationBuckets := []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5}
	metrics := &Metrics{
		transactions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cns_persistent_state_transactions_total",
			Help: "Total number of persistent state database transactions by operation and result.",
		}, []string{metricLabelOperation, metricLabelResult}),
		transactionTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cns_persistent_state_transaction_duration_seconds",
			Help:    "Persistent state database transaction duration in seconds by operation and result.",
			Buckets: durationBuckets,
		}, []string{metricLabelOperation, metricLabelResult}),
		lifecycle: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cns_persistent_state_lifecycle_total",
			Help: "Total number of persistent state lifecycle operations by operation and result.",
		}, []string{metricLabelOperation, metricLabelResult}),
		lifecycleTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cns_persistent_state_lifecycle_duration_seconds",
			Help:    "Persistent state lifecycle operation duration in seconds by operation and result.",
			Buckets: durationBuckets,
		}, []string{metricLabelOperation, metricLabelResult}),
		invariantFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cns_persistent_state_invariant_failures_total",
			Help: "Total number of bounded persistent state invariant failures.",
		}, []string{metricLabelInvariant}),
		info: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cns_persistent_state_info",
			Help: "Persistent state backend, authority, and schema information.",
		}, []string{metricLabelBackend, metricLabelAuthority, metricLabelSchema}),
		generation: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cns_persistent_state_generation",
			Help: "Current committed persistent state generation.",
		}),
		storagePresent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cns_persistent_state_storage_present",
			Help: "Whether persistent state storage is present and readable.",
		}),
		databaseBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cns_persistent_state_database_bytes",
			Help: "Current persistent state database size in bytes.",
		}),
		records: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "cns_persistent_state_records",
			Help: "Current persistent state record count by bounded record type.",
		}, []string{metricLabelRecordType}),
		now: time.Now,
	}

	collectors := []prometheus.Collector{
		metrics.transactions,
		metrics.transactionTime,
		metrics.lifecycle,
		metrics.lifecycleTime,
		metrics.invariantFailures,
		metrics.info,
		metrics.generation,
		metrics.storagePresent,
		metrics.databaseBytes,
		metrics.records,
	}
	registered := make([]prometheus.Collector, 0, len(collectors))
	for _, collector := range collectors {
		if err := registerer.Register(collector); err != nil {
			rollbackComplete := true
			for _, prior := range registered {
				rollbackComplete = registerer.Unregister(prior) && rollbackComplete
			}
			if !rollbackComplete {
				return nil, fmt.Errorf(
					"registering persistent state metrics: %w (collector rollback incomplete)",
					err,
				)
			}
			return nil, fmt.Errorf("registering persistent state metrics: %w", err)
		}
		registered = append(registered, collector)
	}
	return metrics, nil
}

func (m *Metrics) ObserveTransaction(operation TransactionOperation, result OperationResult, duration time.Duration) error {
	if m == nil {
		return nil
	}
	if _, ok := transactionOperations[operation]; !ok {
		return fmt.Errorf("%w: %q", errUnknownTransactionOperation, operation)
	}
	if err := validateOperationResult(result); err != nil {
		return err
	}
	if duration < 0 {
		return errNegativeTransactionDuration
	}
	m.transactions.WithLabelValues(string(operation), string(result)).Inc()
	m.transactionTime.WithLabelValues(string(operation), string(result)).Observe(duration.Seconds())
	return nil
}

func (m *Metrics) ObserveLifecycle(operation LifecycleOperation, result OperationResult, duration time.Duration) error {
	if m == nil {
		return nil
	}
	if _, ok := lifecycleOperations[operation]; !ok {
		return fmt.Errorf("%w: %q", errUnknownLifecycleOperation, operation)
	}
	if err := validateOperationResult(result); err != nil {
		return err
	}
	if duration < 0 {
		return errNegativeLifecycleDuration
	}
	m.lifecycle.WithLabelValues(string(operation), string(result)).Inc()
	m.lifecycleTime.WithLabelValues(string(operation), string(result)).Observe(duration.Seconds())
	return nil
}

func (m *Metrics) ObserveInvariant(name InvariantName) error {
	if m == nil {
		return nil
	}
	if _, ok := invariantNames[name]; !ok {
		return fmt.Errorf("%w: %q", errUnknownPersistentInvariant, name)
	}
	m.invariantFailures.WithLabelValues(string(name)).Inc()
	return nil
}

func (m *Metrics) Refresh(status Status) error {
	if m == nil {
		return nil
	}
	if status.InvariantStatus != InvariantFailed {
		if status.Backend != BackendBolt {
			return fmt.Errorf("%w: %q", errUnknownPersistentStateBackend, status.Backend)
		}
		if err := validateAuthority(status.Authority); err != nil {
			return fmt.Errorf("refreshing persistent state metrics: %w", err)
		}
		if status.SchemaVersion != SchemaVersion {
			return fmt.Errorf(
				"refreshing persistent state metrics: %w: status=%d code=%d",
				ErrSchemaMismatch,
				status.SchemaVersion,
				SchemaVersion,
			)
		}
	}
	m.info.Reset()
	if status.InvariantStatus != InvariantFailed {
		m.info.WithLabelValues(
			status.Backend,
			string(status.Authority),
			strconv.FormatUint(uint64(status.SchemaVersion), 10),
		).Set(1)
	}
	m.generation.Set(float64(status.Generation))
	if status.StoragePresent {
		m.storagePresent.Set(1)
	} else {
		m.storagePresent.Set(0)
	}
	m.databaseBytes.Set(float64(status.DatabaseBytes))
	for _, recordType := range recordTypes {
		m.records.WithLabelValues(string(recordType)).Set(float64(status.RecordCount(recordType)))
	}
	return nil
}

func validateOperationResult(result OperationResult) error {
	if _, ok := operationResults[result]; !ok {
		return fmt.Errorf("%w: %q", errUnknownOperationResult, result)
	}
	return nil
}

func classifyResult(changed bool, err error) OperationResult {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ResultCanceled
	case errors.Is(err, ErrStaleGeneration):
		return ResultConflict
	case err != nil:
		return ResultError
	case !changed:
		return ResultNoop
	default:
		return ResultSuccess
	}
}

func metricNow(metrics *Metrics) time.Time {
	if metrics == nil {
		return time.Now()
	}
	return metrics.now()
}

func metricDuration(metrics *Metrics, started time.Time) time.Duration {
	return metricNow(metrics).Sub(started)
}
