package ipampool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/metric"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/pkg/errors"
)

const (
	// DefaultRefreshDelay pool monitor poll delay default in seconds.
	DefaultRefreshDelay = 1 * time.Second
)

type nodeNetworkConfigSpecUpdater interface {
	UpdateSpec(context.Context, *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error)
}

type Options struct {
	RefreshDelay time.Duration
}

type Monitor struct {
	opts                     *Options
	nnc                      *v1alpha.NodeNetworkConfig
	nnccli                   nodeNetworkConfigSpecUpdater
	httpService              cns.HTTPService
	updatingIpsNotInUseCount int
	initialized              chan interface{}
	nncSource                chan v1alpha.NodeNetworkConfig
	once                     sync.Once
}

func NewMonitor(httpService cns.HTTPService, nnccli nodeNetworkConfigSpecUpdater, opts *Options) *Monitor {
	if opts.RefreshDelay < 1 {
		opts.RefreshDelay = DefaultRefreshDelay
	}
	return &Monitor{
		opts:        opts,
		httpService: httpService,
		nnccli:      nnccli,
		initialized: make(chan interface{}),
		nncSource:   make(chan v1alpha.NodeNetworkConfig),
	}
}

func (pm *Monitor) Start(ctx context.Context) error {
	logger.Printf("[ipam-pool-monitor] Starting CNS IPAM Pool Monitor")

	ticker := time.NewTicker(pm.opts.RefreshDelay)
	defer ticker.Stop()

	for {
		// block until something happens
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "pool monitor context closed")
		case <-ticker.C:
			// block on ticks until we have initialized
			<-pm.initialized
		case nnc := <-pm.nncSource:
			pm.nnc = &nnc
		}
		err := pm.reconcile(ctx)
		if err != nil {
			logger.Printf("[ipam-pool-monitor] Reconcile failed with err %v", err)
		}
	}
}

func (pm *Monitor) reconcile(ctx context.Context) error {
	cnsPodIPConfigCount := len(pm.httpService.GetPodIPConfigState())
	pendingProgramCount := len(pm.httpService.GetPendingProgramIPConfigs()) // TODO: add pending program count to real cns
	allocatedPodIPCount := len(pm.httpService.GetAllocatedIPConfigs())
	pendingReleaseIPCount := len(pm.httpService.GetPendingReleaseIPConfigs())
	availableIPConfigCount := len(pm.httpService.GetAvailableIPConfigs()) // TODO: add pending allocation count to real cns
	requestedIPConfigCount := pm.nnc.Spec.RequestedIPCount
	unallocatedIPConfigCount := cnsPodIPConfigCount - allocatedPodIPCount
	freeIPConfigCount := requestedIPConfigCount - int64(allocatedPodIPCount)
	batchSize := pm.nnc.Status.Scaler.BatchSize
	maxIPCount := pm.nnc.Status.Scaler.MaxIPCount

	msg := fmt.Sprintf("[ipam-pool-monitor] Pool Size: %v, Goal Size: %v, BatchSize: %v, MaxIPCount: %v, Allocated: %v, Available: %v, Pending Release: %v, Free: %v, Pending Program: %v",
		cnsPodIPConfigCount, pm.nnc.Spec.RequestedIPCount, batchSize, maxIPCount, allocatedPodIPCount, availableIPConfigCount, pendingReleaseIPCount, freeIPConfigCount, pendingProgramCount)

	ipamAllocatedIPCount.Set(float64(allocatedPodIPCount))
	ipamAvailableIPCount.Set(float64(availableIPConfigCount))
	ipamBatchSize.Set(float64(batchSize))
	ipamFreeIPCount.Set(float64(freeIPConfigCount))
	ipamIPPool.Set(float64(cnsPodIPConfigCount))
	ipamMaxIPCount.Set(float64(maxIPCount))
	ipamPendingProgramIPCount.Set(float64(pendingProgramCount))
	ipamPendingReleaseIPCount.Set(float64(pendingReleaseIPCount))
	ipamRequestedIPConfigCount.Set(float64(requestedIPConfigCount))
	ipamUnallocatedIPCount.Set(float64(unallocatedIPConfigCount))

	switch {
	// pod count is increasing
	case freeIPConfigCount < int64(CalculateMinFreeIPs(*pm.nnc)):
		if pm.nnc.Spec.RequestedIPCount == maxIPCount {
			// If we're already at the maxIPCount, don't try to increase
			return nil
		}

		logger.Printf("[ipam-pool-monitor] Increasing pool size...%s ", msg)
		return pm.increasePoolSize(ctx)

	// pod count is decreasing
	case freeIPConfigCount >= int64(CalculateMaxFreeIPs(*pm.nnc)):
		logger.Printf("[ipam-pool-monitor] Decreasing pool size...%s ", msg)
		return pm.decreasePoolSize(ctx, pendingReleaseIPCount)

	// CRD has reconciled CNS state, and target spec is now the same size as the state
	// free to remove the IP's from the CRD
	case len(pm.nnc.Spec.IPsNotInUse) != pendingReleaseIPCount:
		logger.Printf("[ipam-pool-monitor] Removing Pending Release IP's from CRD...%s ", msg)
		return pm.cleanPendingRelease(ctx)

	// no pods scheduled
	case allocatedPodIPCount == 0:
		logger.Printf("[ipam-pool-monitor] No pods scheduled, %s", msg)
		return nil
	}

	return nil
}

func (pm *Monitor) increasePoolSize(ctx context.Context) error {
	tempNNCSpec := pm.createNNCSpecForCRD()

	// Query the max IP count
	maxIPCount := pm.nnc.Status.Scaler.MaxIPCount
	previouslyRequestedIPCount := tempNNCSpec.RequestedIPCount
	batchSize := pm.nnc.Status.Scaler.BatchSize

	tempNNCSpec.RequestedIPCount += batchSize
	if tempNNCSpec.RequestedIPCount > maxIPCount {
		// We don't want to ask for more ips than the max
		logger.Printf("[ipam-pool-monitor] Requested IP count (%v) is over max limit (%v), requesting max limit instead.", tempNNCSpec.RequestedIPCount, maxIPCount)
		tempNNCSpec.RequestedIPCount = maxIPCount
	}

	// If the requested IP count is same as before, then don't do anything
	if tempNNCSpec.RequestedIPCount == previouslyRequestedIPCount {
		logger.Printf("[ipam-pool-monitor] Previously requested IP count %v is same as updated IP count %v, doing nothing", previouslyRequestedIPCount, tempNNCSpec.RequestedIPCount)
		return nil
	}

	logger.Printf("[ipam-pool-monitor] Increasing pool size, Current Pool Size: %v, Updated Requested IP Count: %v, Pods with IP's:%v, ToBeDeleted Count: %v", len(pm.httpService.GetPodIPConfigState()), tempNNCSpec.RequestedIPCount, len(pm.httpService.GetAllocatedIPConfigs()), len(tempNNCSpec.IPsNotInUse))

	if _, err := pm.nnccli.UpdateSpec(ctx, &tempNNCSpec); err != nil {
		// caller will retry to update the CRD again
		return err
	}

	logger.Printf("[ipam-pool-monitor] Increasing pool size: UpdateCRDSpec succeeded for spec %+v", tempNNCSpec)
	// start an alloc timer
	metric.StartPoolIncreaseTimer(int(batchSize))
	// save the updated state to cachedSpec
	pm.nnc.Spec = tempNNCSpec
	return nil
}

func (pm *Monitor) decreasePoolSize(ctx context.Context, existingPendingReleaseIPCount int) error {
	// mark n number of IP's as pending
	var newIpsMarkedAsPending bool
	var pendingIPAddresses map[string]cns.IPConfigurationStatus
	var updatedRequestedIPCount int64

	// Ensure the updated requested IP count is a multiple of the batch size
	previouslyRequestedIPCount := pm.nnc.Spec.RequestedIPCount
	batchSize := pm.nnc.Status.Scaler.BatchSize
	modResult := previouslyRequestedIPCount % batchSize

	logger.Printf("[ipam-pool-monitor] Previously RequestedIP Count %v", previouslyRequestedIPCount)
	logger.Printf("[ipam-pool-monitor] Batch size : %v", batchSize)
	logger.Printf("[ipam-pool-monitor] modResult of (previously requested IP count mod batch size) = %v", modResult)

	if modResult != 0 {
		// Example: previouscount = 25, batchsize = 10, 25 - 10 = 15, NOT a multiple of batchsize (10)
		// Don't want that, so make requestedIPCount 20 (25 - (25 % 10)) so that it is a multiple of the batchsize (10)
		updatedRequestedIPCount = previouslyRequestedIPCount - modResult
	} else {
		// Example: previouscount = 30, batchsize = 10, 30 - 10 = 20 which is multiple of batchsize (10) so all good
		updatedRequestedIPCount = previouslyRequestedIPCount - batchSize
	}

	decreaseIPCountBy := previouslyRequestedIPCount - updatedRequestedIPCount

	logger.Printf("[ipam-pool-monitor] updatedRequestedIPCount %v", updatedRequestedIPCount)

	if pm.updatingIpsNotInUseCount == 0 ||
		pm.updatingIpsNotInUseCount < existingPendingReleaseIPCount {
		logger.Printf("[ipam-pool-monitor] Marking IPs as PendingRelease, ipsToBeReleasedCount %d", int(decreaseIPCountBy))
		var err error
		pendingIPAddresses, err = pm.httpService.MarkIPAsPendingRelease(int(decreaseIPCountBy))
		if err != nil {
			return err
		}

		newIpsMarkedAsPending = true
	}

	tempNNCSpec := pm.createNNCSpecForCRD()

	if newIpsMarkedAsPending {
		// cache the updatingPendingRelease so that we dont re-set new IPs to PendingRelease in case UpdateCRD call fails
		pm.updatingIpsNotInUseCount = len(tempNNCSpec.IPsNotInUse)
	}

	logger.Printf("[ipam-pool-monitor] Releasing IPCount in this batch %d, updatingPendingIpsNotInUse count %d",
		len(pendingIPAddresses), pm.updatingIpsNotInUseCount)

	tempNNCSpec.RequestedIPCount -= int64(len(pendingIPAddresses))
	logger.Printf("[ipam-pool-monitor] Decreasing pool size, Current Pool Size: %v, Requested IP Count: %v, Pods with IP's: %v, ToBeDeleted Count: %v", len(pm.httpService.GetPodIPConfigState()), tempNNCSpec.RequestedIPCount, len(pm.httpService.GetAllocatedIPConfigs()), len(tempNNCSpec.IPsNotInUse))

	_, err := pm.nnccli.UpdateSpec(ctx, &tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	logger.Printf("[ipam-pool-monitor] Decreasing pool size: UpdateCRDSpec succeeded for spec %+v", tempNNCSpec)
	// start a dealloc timer
	metric.StartPoolDecreaseTimer(int(batchSize))

	// save the updated state to cachedSpec
	pm.nnc.Spec = tempNNCSpec

	// clear the updatingPendingIpsNotInUse, as we have Updated the CRD
	logger.Printf("[ipam-pool-monitor] cleaning the updatingPendingIpsNotInUse, existing length %d", pm.updatingIpsNotInUseCount)
	pm.updatingIpsNotInUseCount = 0

	return nil
}

// cleanPendingRelease removes IPs from the cache and CRD if the request controller has reconciled
// CNS state and the pending IP release map is empty.
func (pm *Monitor) cleanPendingRelease(ctx context.Context) error {
	tempNNCSpec := pm.createNNCSpecForCRD()

	_, err := pm.nnccli.UpdateSpec(ctx, &tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	logger.Printf("[ipam-pool-monitor] cleanPendingRelease: UpdateCRDSpec succeeded for spec %+v", tempNNCSpec)

	// save the updated state to cachedSpec
	pm.nnc.Spec = tempNNCSpec
	return nil
}

// createNNCSpecForCRD translates CNS's map of IPs to be released and requested IP count into an NNC Spec.
func (pm *Monitor) createNNCSpecForCRD() v1alpha.NodeNetworkConfigSpec {
	var spec v1alpha.NodeNetworkConfigSpec

	// Update the count from cached spec
	spec.RequestedIPCount = pm.nnc.Spec.RequestedIPCount

	// Get All Pending IPs from CNS and populate it again.
	pendingIPs := pm.httpService.GetPendingReleaseIPConfigs()
	for _, pendingIP := range pendingIPs {
		spec.IPsNotInUse = append(spec.IPsNotInUse, pendingIP.ID)
	}

	return spec
}

// GetStateSnapshot gets a snapshot of the IPAMPoolMonitor struct.
func (pm *Monitor) GetStateSnapshot() cns.IpamPoolMonitorStateSnapshot {
	nnc := *pm.nnc
	return cns.IpamPoolMonitorStateSnapshot{
		MinimumFreeIps:           CalculateMinFreeIPs(nnc),
		MaximumFreeIps:           CalculateMaxFreeIPs(nnc),
		UpdatingIpsNotInUseCount: pm.updatingIpsNotInUseCount,
		CachedNNC:                nnc,
	}
}

// Update ingests a NodeNetworkConfig, clamping some values to ensure they are legal and then
// pushing it to the PoolMonitor's source channel.
// As a side effect, marks the PoolMonitor as initialized, if it is not already.
func (pm *Monitor) Update(nnc *v1alpha.NodeNetworkConfig) {
	clampScaler(nnc)

	// if the nnc has conveged, observe the pool scaling latency (if any)
	allocatedIPs := len(pm.httpService.GetPodIPConfigState()) - len(pm.httpService.GetPendingReleaseIPConfigs())
	if int(nnc.Spec.RequestedIPCount) == allocatedIPs {
		// observe elapsed duration for IP pool scaling
		metric.ObserverPoolScaleLatency()
	}

	// defer closing the init channel to signify that we have received at least one NodeNetworkConfig.
	defer pm.once.Do(func() { close(pm.initialized) })
	pm.nncSource <- *nnc
}

// clampScaler makes sure that the values stored in the scaler are sane.
// we usually expect these to be correctly set for us, but we could crash
// without these checks. if they are incorrectly set, there will be some weird
// IP pool behavior for a while until the nnc reconciler corrects the state.
func clampScaler(nnc *v1alpha.NodeNetworkConfig) {
	if nnc.Status.Scaler.MaxIPCount < 1 {
		nnc.Status.Scaler.MaxIPCount = 1
	}
	if nnc.Status.Scaler.BatchSize < 1 {
		nnc.Status.Scaler.BatchSize = 1
	}
	if nnc.Status.Scaler.BatchSize > nnc.Status.Scaler.MaxIPCount {
		nnc.Status.Scaler.BatchSize = nnc.Status.Scaler.MaxIPCount
	}
	if nnc.Status.Scaler.RequestThresholdPercent < 1 {
		nnc.Status.Scaler.RequestThresholdPercent = 1
	}
	if nnc.Status.Scaler.RequestThresholdPercent > 100 { //nolint:gomnd // it's a percent
		nnc.Status.Scaler.RequestThresholdPercent = 100
	}
	if nnc.Status.Scaler.ReleaseThresholdPercent < nnc.Status.Scaler.RequestThresholdPercent+100 {
		nnc.Status.Scaler.ReleaseThresholdPercent = nnc.Status.Scaler.RequestThresholdPercent + 100 //nolint:gomnd // it's a percent
	}
}

// CalculateMinFreeIPs calculates the minimum free IP quantity based on the Scaler
// in the passed NodeNetworkConfig.
//nolint:gocritic // ignore hugeparam
func CalculateMinFreeIPs(nnc v1alpha.NodeNetworkConfig) int {
	batchSize := nnc.Status.Scaler.BatchSize
	requestThreshold := nnc.Status.Scaler.RequestThresholdPercent
	return int(float64(batchSize) * (float64(requestThreshold) / 100)) //nolint:gomnd // it's a percent
}

// CalculateMaxFreeIPs calculates the maximum free IP quantity based on the Scaler
// in the passed NodeNetworkConfig.
//nolint:gocritic // ignore hugeparam
func CalculateMaxFreeIPs(nnc v1alpha.NodeNetworkConfig) int {
	batchSize := nnc.Status.Scaler.BatchSize
	releaseThreshold := nnc.Status.Scaler.ReleaseThresholdPercent
	return int(float64(batchSize) * (float64(releaseThreshold) / 100)) //nolint:gomnd // it's a percent
}
