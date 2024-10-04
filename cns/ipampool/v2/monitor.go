package v2

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// DefaultMaxIPs default maximum allocatable IPs on a k8s Node.
	DefaultMaxIPs = 250
	// fieldManager is the field manager used when patching the NodeNetworkConfig.
	fieldManager = "azure-cns"
)

type nodeNetworkConfigSpecUpdater interface {
	PatchSpec(context.Context, *v1alpha.NodeNetworkConfigSpec, string) (*v1alpha.NodeNetworkConfig, error)
}

type ipStateStore interface {
	GetPendingReleaseIPConfigs() []cns.IPConfigurationStatus
	MarkNIPsPendingRelease(n int) (map[string]cns.IPConfigurationStatus, error)
}

type scaler struct {
	batch     int64
	buffer    float64
	exhausted bool
	max       int64
}

type Monitor struct {
	z                     *zap.Logger
	scaler                scaler
	nnccli                nodeNetworkConfigSpecUpdater
	store                 ipStateStore
	demand                int64
	request               int64
	demandSource          <-chan int
	cssSource             <-chan v1alpha1.ClusterSubnetState
	nncSource             <-chan v1alpha.NodeNetworkConfig
	started               chan interface{}
	once                  sync.Once
	legacyMetricsObserver func(context.Context) error
}

func NewMonitor(z *zap.Logger, store ipStateStore, nnccli nodeNetworkConfigSpecUpdater, demandSource <-chan int, nncSource <-chan v1alpha.NodeNetworkConfig, cssSource <-chan v1alpha1.ClusterSubnetState) *Monitor { //nolint:lll // it's fine
	return &Monitor{
		z:                     z.With(zap.String("component", "ipam-pool-monitor")),
		store:                 store,
		nnccli:                nnccli,
		demandSource:          demandSource,
		cssSource:             cssSource,
		nncSource:             nncSource,
		started:               make(chan interface{}),
		legacyMetricsObserver: func(context.Context) error { return nil },
	}
}

// Start begins the Monitor's pool reconcile loop.
// On first run, it will block until a NodeNetworkConfig is received (through a call to Update()).
// Subsequently, it will run run when Events happen or at least once per ReconcileDelay and attempt to re-reconcile the pool.
func (pm *Monitor) Start(ctx context.Context) error {
	pm.z.Debug("starting")
	maxReconcileDelay := time.NewTicker(60 * time.Second) //nolint:gomnd // 60 seconds
	for {
		// proceed when things happen:
		select {
		case <-ctx.Done(): // calling context has closed, we'll exit.
			return errors.Wrap(ctx.Err(), "pool monitor context closed")
		case demand := <-pm.demandSource: // updated demand for IPs, recalculate request
			pm.demand = int64(demand)
			pm.z.Info("demand update", zap.Int64("demand", pm.demand))
		case css := <-pm.cssSource: // received an updated ClusterSubnetState, recalculate request
			pm.scaler.exhausted = css.Status.Exhausted
			pm.z.Info("exhaustion update", zap.Bool("exhausted", pm.scaler.exhausted))
		case nnc := <-pm.nncSource: // received a new NodeNetworkConfig, extract the data from it and recalculate request
			pm.scaler.max = int64(math.Min(float64(nnc.Status.Scaler.MaxIPCount), DefaultMaxIPs))
			pm.scaler.batch = int64(math.Min(math.Max(float64(nnc.Status.Scaler.BatchSize), 1), float64(pm.scaler.max)))
			pm.scaler.buffer = math.Abs(float64(nnc.Status.Scaler.RequestThresholdPercent)) / 100 //nolint:gomnd // it's a percentage
			pm.once.Do(func() {
				pm.request = nnc.Spec.RequestedIPCount
				close(pm.started) // close the init channel the first time we fully receive a NodeNetworkConfig.
				pm.z.Debug("started", zap.Int64("initial request", pm.request))
			})
			pm.z.Info("scaler update", zap.Int64("batch", pm.scaler.batch), zap.Float64("buffer", pm.scaler.buffer), zap.Int64("max", pm.scaler.max), zap.Int64("request", pm.request))
		case <-maxReconcileDelay.C: // try to reconcile the pool every maxReconcileDelay to prevent drift or lockups.
		}
		select {
		case <-pm.started: // this blocks until we have initialized
		default:
			// if we haven't started yet, we need to wait for the first NNC to be received.
			continue // jumps to the next iteration of the outer for-loop
		}
		// if control has flowed through the select(s) to this point, we can now reconcile.
		if err := pm.reconcile(ctx); err != nil {
			pm.z.Error("reconcile failed", zap.Error(err))
		}
		if err := pm.legacyMetricsObserver(ctx); err != nil {
			pm.z.Error("legacy metrics observer failed", zap.Error(err))
		}
	}
}

func (pm *Monitor) reconcile(ctx context.Context) error {
	// if the subnet is exhausted, locally overwrite the batch/minfree/maxfree in the meta copy for this iteration
	// (until the controlplane owns this and modifies the scaler values for us directly instead of writing "exhausted")
	// TODO(rbtr)
	s := pm.scaler
	if s.exhausted {
		s.batch = 1
		s.buffer = 1
	}

	// calculate the target state from the current pool state and scaler
	target := calculateTargetIPCountOrMax(pm.demand, s.batch, s.max, s.buffer)
	pm.z.Info("calculated new request", zap.Int64("demand", pm.demand), zap.Int64("batch", s.batch), zap.Int64("max", s.max), zap.Float64("buffer", s.buffer), zap.Int64("target", target))
	delta := target - pm.request
	if delta == 0 {
		pm.z.Info("NNC already at target IPs, no scaling required")
		return nil
	}
	pm.z.Info("scaling pool", zap.Int64("delta", delta))
	// try to release -delta IPs. this is no-op if delta is negative.
	if _, err := pm.store.MarkNIPsPendingRelease(int(-delta)); err != nil {
		return errors.Wrapf(err, "failed to mark sufficient IPs as PendingRelease, wanted %d", pm.request-target)
	}
	spec := pm.buildNNCSpec(target)
	if _, err := pm.nnccli.PatchSpec(ctx, &spec, fieldManager); err != nil {
		return errors.Wrap(err, "failed to UpdateSpec with NNC client")
	}
	pm.request = target
	pm.z.Info("scaled pool", zap.Int64("request", pm.request))
	return nil
}

// buildNNCSpec translates CNS's map of IPs to be released and requested IP count into an NNC Spec.
func (pm *Monitor) buildNNCSpec(request int64) v1alpha.NodeNetworkConfigSpec {
	// Get All Pending IPs from CNS and populate it again.
	pendingReleaseIPs := pm.store.GetPendingReleaseIPConfigs()
	spec := v1alpha.NodeNetworkConfigSpec{
		RequestedIPCount: request,
		IPsNotInUse:      make([]string, len(pendingReleaseIPs)),
	}
	for i := range pendingReleaseIPs {
		spec.IPsNotInUse[i] = pendingReleaseIPs[i].ID
	}
	return spec
}

func (pm *Monitor) WithLegacyMetricsObserver(observer func(context.Context) error) {
	pm.legacyMetricsObserver = observer
}

// calculateTargetIPCountOrMax calculates the target IP count request
// using the scaling function and clamps the result at the max IPs.
func calculateTargetIPCountOrMax(demand, batch, max int64, buffer float64) int64 {
	targetRequest := calculateTargetIPCount(demand, batch, buffer)
	if targetRequest > max {
		// clamp request at the max IPs
		targetRequest = max
	}
	return targetRequest
}

// calculateTargetIPCount calculates an IP count request based on the
// current demand, batch size, and buffer.
// ref: https://github.com/Azure/azure-container-networking/blob/master/docs/feature/ipammath/0-background.md
// the idempotent scaling function is:
// Target = Batch \times \lceil buffer + \frac{Demand}{Batch} \rceil
func calculateTargetIPCount(demand, batch int64, buffer float64) int64 {
	return batch * int64(math.Ceil(buffer+float64(demand)/float64(batch)))
}
