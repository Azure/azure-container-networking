package ipampoolmonitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/metric"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

const defaultMaxIPCount = int64(250)

type nodeNetworkConfigSpecUpdater interface {
	UpdateSpec(context.Context, *v1alpha.NodeNetworkConfigSpec) (*v1alpha.NodeNetworkConfig, error)
}

type CNSIPAMPoolMonitor struct {
	MaximumFreeIps           int64
	MinimumFreeIps           int64
	cachedNNC                v1alpha.NodeNetworkConfig
	httpService              cns.HTTPService
	mu                       sync.RWMutex
	scalarUnits              v1alpha.Scaler
	updatingIpsNotInUseCount int
	nnccli                   nodeNetworkConfigSpecUpdater
}

func NewCNSIPAMPoolMonitor(httpService cns.HTTPService, nnccli nodeNetworkConfigSpecUpdater) *CNSIPAMPoolMonitor {
	logger.Printf("NewCNSIPAMPoolMonitor: Create IPAM Pool Monitor")
	return &CNSIPAMPoolMonitor{
		httpService: httpService,
		nnccli:      nnccli,
	}
}

func (pm *CNSIPAMPoolMonitor) Start(ctx context.Context, poolMonitorRefreshMilliseconds int) error {
	logger.Printf("[ipam-pool-monitor] Starting CNS IPAM Pool Monitor")

	ticker := time.NewTicker(time.Duration(poolMonitorRefreshMilliseconds) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("[ipam-pool-monitor] CNS IPAM Pool Monitor received cancellation signal")
		case <-ticker.C:
			err := pm.Reconcile(ctx)
			if err != nil {
				logger.Printf("[ipam-pool-monitor] Reconcile failed with err %v", err)
			}
		}
	}
}

func (pm *CNSIPAMPoolMonitor) Reconcile(ctx context.Context) error {
	cnsPodIPConfigCount := len(pm.httpService.GetPodIPConfigState())
	pendingProgramCount := len(pm.httpService.GetPendingProgramIPConfigs()) // TODO: add pending program count to real cns
	allocatedPodIPCount := len(pm.httpService.GetAllocatedIPConfigs())
	pendingReleaseIPCount := len(pm.httpService.GetPendingReleaseIPConfigs())
	availableIPConfigCount := len(pm.httpService.GetAvailableIPConfigs()) // TODO: add pending allocation count to real cns
	requestedIPConfigCount := pm.cachedNNC.Spec.RequestedIPCount
	unallocatedIPConfigCount := cnsPodIPConfigCount - allocatedPodIPCount
	freeIPConfigCount := requestedIPConfigCount - int64(allocatedPodIPCount)
	batchSize := pm.getBatchSize() // Use getters in case customer changes batchsize manually
	maxIPCount := pm.getMaxIPCount()

	msg := fmt.Sprintf("[ipam-pool-monitor] Pool Size: %v, Goal Size: %v, BatchSize: %v, MaxIPCount: %v, MinFree: %v, MaxFree:%v, Allocated: %v, Available: %v, Pending Release: %v, Free: %v, Pending Program: %v",
		cnsPodIPConfigCount, pm.cachedNNC.Spec.RequestedIPCount, batchSize, maxIPCount, pm.MinimumFreeIps, pm.MaximumFreeIps, allocatedPodIPCount, availableIPConfigCount, pendingReleaseIPCount, freeIPConfigCount, pendingProgramCount)

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
	case freeIPConfigCount < pm.MinimumFreeIps:
		if pm.cachedNNC.Spec.RequestedIPCount == maxIPCount {
			// If we're already at the maxIPCount, don't try to increase
			return nil
		}

		logger.Printf("[ipam-pool-monitor] Increasing pool size...%s ", msg)
		return pm.increasePoolSize(ctx)

	// pod count is decreasing
	case freeIPConfigCount >= pm.MaximumFreeIps:
		logger.Printf("[ipam-pool-monitor] Decreasing pool size...%s ", msg)
		return pm.decreasePoolSize(ctx, pendingReleaseIPCount)

	// CRD has reconciled CNS state, and target spec is now the same size as the state
	// free to remove the IP's from the CRD
	case len(pm.cachedNNC.Spec.IPsNotInUse) != pendingReleaseIPCount:
		logger.Printf("[ipam-pool-monitor] Removing Pending Release IP's from CRD...%s ", msg)
		return pm.cleanPendingRelease(ctx)

	// no pods scheduled
	case allocatedPodIPCount == 0:
		logger.Printf("[ipam-pool-monitor] No pods scheduled, %s", msg)
		return nil
	}

	return nil
}

func (pm *CNSIPAMPoolMonitor) increasePoolSize(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	tempNNCSpec := pm.createNNCSpecForCRD()

	// Query the max IP count
	maxIPCount := pm.getMaxIPCount()
	previouslyRequestedIPCount := tempNNCSpec.RequestedIPCount
	batchSize := pm.getBatchSize()

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
	pm.cachedNNC.Spec = tempNNCSpec
	return nil
}

func (pm *CNSIPAMPoolMonitor) decreasePoolSize(ctx context.Context, existingPendingReleaseIPCount int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// mark n number of IP's as pending
	var newIpsMarkedAsPending bool
	var pendingIPAddresses map[string]cns.IPConfigurationStatus
	var updatedRequestedIPCount int64

	// Ensure the updated requested IP count is a multiple of the batch size
	previouslyRequestedIPCount := pm.cachedNNC.Spec.RequestedIPCount
	batchSize := pm.getBatchSize()
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
	pm.cachedNNC.Spec = tempNNCSpec

	// clear the updatingPendingIpsNotInUse, as we have Updated the CRD
	logger.Printf("[ipam-pool-monitor] cleaning the updatingPendingIpsNotInUse, existing length %d", pm.updatingIpsNotInUseCount)
	pm.updatingIpsNotInUseCount = 0

	return nil
}

// cleanPendingRelease removes IPs from the cache and CRD if the request controller has reconciled
// CNS state and the pending IP release map is empty.
func (pm *CNSIPAMPoolMonitor) cleanPendingRelease(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	tempNNCSpec := pm.createNNCSpecForCRD()

	_, err := pm.nnccli.UpdateSpec(ctx, &tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	logger.Printf("[ipam-pool-monitor] cleanPendingRelease: UpdateCRDSpec succeeded for spec %+v", tempNNCSpec)

	// save the updated state to cachedSpec
	pm.cachedNNC.Spec = tempNNCSpec
	return nil
}

// createNNCSpecForCRD translates CNS's map of IPs to be released and requested IP count into an NNC Spec.
func (pm *CNSIPAMPoolMonitor) createNNCSpecForCRD() v1alpha.NodeNetworkConfigSpec {
	var spec v1alpha.NodeNetworkConfigSpec

	// Update the count from cached spec
	spec.RequestedIPCount = pm.cachedNNC.Spec.RequestedIPCount

	// Get All Pending IPs from CNS and populate it again.
	pendingIPs := pm.httpService.GetPendingReleaseIPConfigs()
	for _, pendingIP := range pendingIPs {
		spec.IPsNotInUse = append(spec.IPsNotInUse, pendingIP.ID)
	}

	return spec
}

// UpdatePoolLimitsTransacted called by request controller on reconcile to set the batch size limits
func (pm *CNSIPAMPoolMonitor) Update(scalar v1alpha.Scaler, spec v1alpha.NodeNetworkConfigSpec) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.scalarUnits = scalar

	pm.MinimumFreeIps = int64(float64(pm.getBatchSize()) * (float64(pm.scalarUnits.RequestThresholdPercent) / 100))
	pm.MaximumFreeIps = int64(float64(pm.getBatchSize()) * (float64(pm.scalarUnits.ReleaseThresholdPercent) / 100))

	pm.cachedNNC.Spec = spec

	// if the nnc has conveged, observe the pool scaling latency (if any)
	allocatedIPs := len(pm.httpService.GetPodIPConfigState()) - len(pm.httpService.GetPendingReleaseIPConfigs())
	if int(pm.cachedNNC.Spec.RequestedIPCount) == allocatedIPs {
		// observe elapsed duration for IP pool scaling
		metric.ObserverPoolScaleLatency()
	}

	logger.Printf("[ipam-pool-monitor] Update spec %+v, pm.MinimumFreeIps %d, pm.MaximumFreeIps %d",
		pm.cachedNNC.Spec, pm.MinimumFreeIps, pm.MaximumFreeIps)
}

func (pm *CNSIPAMPoolMonitor) getMaxIPCount() int64 {
	if pm.scalarUnits.MaxIPCount == 0 {
		pm.scalarUnits.MaxIPCount = defaultMaxIPCount
	}
	return pm.scalarUnits.MaxIPCount
}

func (pm *CNSIPAMPoolMonitor) getBatchSize() int64 {
	maxIPCount := pm.getMaxIPCount()
	if pm.scalarUnits.BatchSize > maxIPCount {
		pm.scalarUnits.BatchSize = maxIPCount
	}
	return pm.scalarUnits.BatchSize
}

// GetStateSnapshot gets a snapshot of the IPAMPoolMonitor struct.
func (pm *CNSIPAMPoolMonitor) GetStateSnapshot() cns.IpamPoolMonitorStateSnapshot {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return cns.IpamPoolMonitorStateSnapshot{
		MinimumFreeIps:           pm.MinimumFreeIps,
		MaximumFreeIps:           pm.MaximumFreeIps,
		UpdatingIpsNotInUseCount: pm.updatingIpsNotInUseCount,
		CachedNNC:                pm.cachedNNC,
	}
}
