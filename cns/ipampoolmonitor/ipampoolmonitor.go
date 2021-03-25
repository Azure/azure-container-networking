package ipampoolmonitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

type CNSIPAMPoolMonitor struct {
	pendingRelease bool

	cachedNNC   nnc.NodeNetworkConfig
	updatingIpsNotInUseCount int
	scalarUnits nnc.Scaler

	httpService    cns.HTTPService
	rc             requestcontroller.RequestController
	MinimumFreeIps int64
	MaximumFreeIps int64

	mu sync.RWMutex
}

func NewCNSIPAMPoolMonitor(httpService cns.HTTPService, rc requestcontroller.RequestController) *CNSIPAMPoolMonitor {
	logger.Printf("NewCNSIPAMPoolMonitor: Create IPAM Pool Monitor")
	return &CNSIPAMPoolMonitor{
		pendingRelease:				false,
		httpService:        		httpService,
		rc:             			rc,
	}
}

func stopReconcile(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
	}

	return false
}

func (pm *CNSIPAMPoolMonitor) Start(ctx context.Context, poolMonitorRefreshMilliseconds int) error {
	logger.Printf("[ipam-pool-monitor] Starting CNS IPAM Pool Monitor")

	ticker := time.NewTicker(time.Duration(poolMonitorRefreshMilliseconds) * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("[ipam-pool-monitor] CNS IPAM Pool Monitor received cancellation signal")
		case <-ticker.C:
			err := pm.Reconcile()
			if err != nil {
				logger.Printf("[ipam-pool-monitor] Reconcile failed with err %v", err)
			}
		}
	}
}

func (pm *CNSIPAMPoolMonitor) Reconcile() error {
	cnsPodIPConfigCount := len(pm.httpService.GetPodIPConfigState())
	pendingProgramCount := len(pm.httpService.GetPendingProgramIPConfigs()) // TODO: add pending program count to real cns
	allocatedPodIPCount := len(pm.httpService.GetAllocatedIPConfigs())
	pendingReleaseIPCount := len(pm.httpService.GetPendingReleaseIPConfigs())
	availableIPConfigCount := len(pm.httpService.GetAvailableIPConfigs()) // TODO: add pending allocation count to real cns
	freeIPConfigCount := pm.cachedNNC.Spec.RequestedIPCount - int64(allocatedPodIPCount)

	msg := fmt.Sprintf("[ipam-pool-monitor] Pool Size: %v, Goal Size: %v, BatchSize: %v, MinFree: %v, MaxFree:%v, Allocated: %v, Available: %v, Pending Release: %v, Free: %v, Pending Program: %v",
		cnsPodIPConfigCount, pm.cachedNNC.Spec.RequestedIPCount, pm.scalarUnits.BatchSize, pm.MinimumFreeIps, pm.MaximumFreeIps, allocatedPodIPCount, availableIPConfigCount, pendingReleaseIPCount, freeIPConfigCount, pendingProgramCount)

	switch {
	// pod count is increasing
	case freeIPConfigCount < pm.MinimumFreeIps:
		logger.Printf("[ipam-pool-monitor] Increasing pool size...%s ", msg)
		return pm.increasePoolSize()

	// pod count is decreasing
	case freeIPConfigCount > pm.MaximumFreeIps:
		logger.Printf("[ipam-pool-monitor] Decreasing pool size...%s ", msg)
		return pm.decreasePoolSize(pendingReleaseIPCount)

	// CRD has reconciled CNS state, and target spec is now the same size as the state
	// free to remove the IP's from the CRD
	case pm.pendingRelease && int(pm.cachedNNC.Spec.RequestedIPCount) == cnsPodIPConfigCount:
		logger.Printf("[ipam-pool-monitor] Removing Pending Release IP's from CRD...%s ", msg)
		return pm.cleanPendingRelease()

	// no pods scheduled
	case allocatedPodIPCount == 0:
		logger.Printf("[ipam-pool-monitor] No pods scheduled, %s", msg)
		return nil
	}

	return nil
}

func (pm *CNSIPAMPoolMonitor) increasePoolSize() error {
	defer pm.mu.Unlock()
	pm.mu.Lock()

	var err error
	var tempNNCSpec nnc.NodeNetworkConfigSpec
	tempNNCSpec, err = pm.createNNCSpecForCRD(false)
	if err != nil {
		return err
	}

	tempNNCSpec.RequestedIPCount += pm.scalarUnits.BatchSize
	logger.Printf("[ipam-pool-monitor] Increasing pool size, Current Pool Size: %v, Updated Requested IP Count: %v, Pods with IP's:%v, ToBeDeleted Count: %v", len(pm.httpService.GetPodIPConfigState()), tempNNCSpec.RequestedIPCount, len(pm.httpService.GetAllocatedIPConfigs()), len(tempNNCSpec.IPsNotInUse))

	err = pm.rc.UpdateCRDSpec(context.Background(), tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	logger.Printf("[ipam-pool-monitor] Increasing pool size: UpdateCRDSpec succeeded for spec %+v", tempNNCSpec)
	// save the updated state to cachedSpec
	pm.cachedNNC.Spec = tempNNCSpec
	return nil
}

func (pm *CNSIPAMPoolMonitor) decreasePoolSize(existingPendingReleaseIPCount int) error {
	defer pm.mu.Unlock()
	pm.mu.Lock()

	// mark n number of IP's as pending
	var err error
	var newIpsMarkedAsPending bool
	var pendingIpAddresses map[string]cns.IPConfigurationStatus
	if pm.updatingIpsNotInUseCount == 0 ||
		pm.updatingIpsNotInUseCount < existingPendingReleaseIPCount {
		logger.Printf("[ipam-pool-monitor] Marking IPs as PendingRelease, ipsToBeReleasedCount %d", int(pm.scalarUnits.BatchSize))
		pendingIpAddresses, err = pm.httpService.MarkIPAsPendingRelease(int(pm.scalarUnits.BatchSize))
		if err != nil {
			return err
		}

		newIpsMarkedAsPending = true
	}

	var tempNNCSpec nnc.NodeNetworkConfigSpec
	tempNNCSpec, err = pm.createNNCSpecForCRD(false)
	if err != nil {
		return err
	}

	if newIpsMarkedAsPending {
		// cache the updatingPendingRelease so that we dont re-set new IPs to PendingRelease in case UpdateCRD call fails
		pm.updatingIpsNotInUseCount = len(tempNNCSpec.IPsNotInUse)
	}

	logger.Printf("[ipam-pool-monitor] Releasing IPCount in this batch %d, updatingPendingIpsNotInUse count %d", len(pendingIpAddresses), pm.updatingIpsNotInUseCount)

	tempNNCSpec.RequestedIPCount -= int64(len(pendingIpAddresses))
	logger.Printf("[ipam-pool-monitor] Decreasing pool size, Current Pool Size: %v, Requested IP Count: %v, Pods with IP's: %v, ToBeDeleted Count: %v", len(pm.httpService.GetPodIPConfigState()), tempNNCSpec.RequestedIPCount, len(pm.httpService.GetAllocatedIPConfigs()), len(tempNNCSpec.IPsNotInUse))

	err = pm.rc.UpdateCRDSpec(context.Background(), tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	logger.Printf("[ipam-pool-monitor] Decreasing pool size: UpdateCRDSpec succeeded for spec %+v", tempNNCSpec)

	// save the updated state to cachedSpec
	pm.cachedNNC.Spec = tempNNCSpec
	pm.pendingRelease = true

	// clear the updatingPendingIpsNotInUse, as we have Updated the CRD
	logger.Printf("[ipam-pool-monitor] cleaning the updatingPendingIpsNotInUse, existing length %d", pm.updatingIpsNotInUseCount)
	pm.updatingIpsNotInUseCount = 0

	return nil
}

// if cns pending ip release map is empty, request controller has already reconciled the CNS state,
// so we can remove it from our cache and remove the IP's from the CRD
func (pm *CNSIPAMPoolMonitor) cleanPendingRelease() error {
	defer pm.mu.Unlock()
	pm.mu.Lock()

	var err error
	var tempNNCSpec nnc.NodeNetworkConfigSpec
	tempNNCSpec, err = pm.createNNCSpecForCRD(true)
	if err != nil {
		return err
	}

	err = pm.rc.UpdateCRDSpec(context.Background(), tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	logger.Printf("[ipam-pool-monitor] cleanPendingRelease: UpdateCRDSpec succeeded for spec %+v", tempNNCSpec)


	// save the updated state to cachedSpec
	pm.cachedNNC.Spec = tempNNCSpec
	pm.pendingRelease = false
	return nil
}

// CNSToCRDSpec translates CNS's map of Ips to be released and requested ip count into a CRD Spec
func (pm *CNSIPAMPoolMonitor) createNNCSpecForCRD(resetNotInUseList bool) (nnc.NodeNetworkConfigSpec, error) {
	var (
		spec nnc.NodeNetworkConfigSpec
	)

	// DUpdate the count from cached spec
	spec.RequestedIPCount = pm.cachedNNC.Spec.RequestedIPCount

	// Discard the ToBeDeleted list if requested. This happens if DNC has cleaned up the pending ips and CNS has also updated its state.
	if resetNotInUseList == true {
		spec.IPsNotInUse = make([]string, 0)
	} else {
		// Get All Pending IPs from CNS and populate it again.
		pendingIps := pm.httpService.GetPendingReleaseIPConfigs()
		for _, pendingIp := range pendingIps {
			spec.IPsNotInUse = append(spec.IPsNotInUse, pendingIp.ID)
		}
	}

	return spec, nil
}

// UpdatePoolLimitsTransacted called by request controller on reconcile to set the batch size limits
func (pm *CNSIPAMPoolMonitor) Update(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error {
	defer pm.mu.Unlock()
	pm.mu.Lock()

	pm.scalarUnits = scalar

	pm.MinimumFreeIps = int64(float64(pm.scalarUnits.BatchSize) * (float64(pm.scalarUnits.RequestThresholdPercent) / 100))
	pm.MaximumFreeIps = int64(float64(pm.scalarUnits.BatchSize) * (float64(pm.scalarUnits.ReleaseThresholdPercent) / 100))

	pm.cachedNNC.Spec = spec

	logger.Printf("[ipam-pool-monitor] Update spec %+v, pm.MinimumFreeIps %d, pm.MaximumFreeIps %d",
		pm.cachedNNC.Spec, pm.MinimumFreeIps, pm.MaximumFreeIps)

	return nil
}
