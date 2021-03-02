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
	scalarUnits nnc.Scaler

	cns            cns.HTTPService
	rc             requestcontroller.RequestController
	MinimumFreeIps int64
	MaximumFreeIps int64

	mu sync.RWMutex
}

func NewCNSIPAMPoolMonitor(cns cns.HTTPService, rc requestcontroller.RequestController) *CNSIPAMPoolMonitor {
	logger.Printf("NewCNSIPAMPoolMonitor: Create IPAM Pool Monitor")
	return &CNSIPAMPoolMonitor{
		pendingRelease: false,
		cns:            cns,
		rc:             rc,
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
	cnsPodIPConfigCount := len(pm.cns.GetPodIPConfigState())
	pendingProgramCount := len(pm.cns.GetPendingProgramIPConfigs()) // TODO: add pending program count to real cns
	allocatedPodIPCount := len(pm.cns.GetAllocatedIPConfigs())
	pendingReleaseIPCount := len(pm.cns.GetPendingReleaseIPConfigs())
	availableIPConfigCount := len(pm.cns.GetAvailableIPConfigs()) // TODO: add pending allocation count to real cns
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
		return pm.decreasePoolSize()

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
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var err error
	var tempNNCSpec nnc.NodeNetworkConfigSpec
	tempNNCSpec, err = pm.createNNCSpecForCRD(false)
	if err != nil {
		return err
	}

	tempNNCSpec.RequestedIPCount += pm.scalarUnits.BatchSize
	logger.Printf("[ipam-pool-monitor] Increasing pool size, Current Pool Size: %v, Updated Requested IP Count: %v, Pods with IP's:%v, ToBeDeleted Count: %v", len(pm.cns.GetPodIPConfigState()), tempNNCSpec.RequestedIPCount, len(pm.cns.GetAllocatedIPConfigs()), len(tempNNCSpec.IPsNotInUse))

	err = pm.rc.UpdateCRDSpec(context.Background(), tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	// save the updated state to cachedSpec
	pm.cachedNNC.Spec = tempNNCSpec
	return nil
}

func (pm *CNSIPAMPoolMonitor) decreasePoolSize() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// mark n number of IP's as pending
	pendingIpAddresses, err := pm.cns.MarkIPAsPendingRelease(int(pm.scalarUnits.BatchSize))
	if err != nil {
		return err
	}

	totalIpsSetForRelease := len(pendingIpAddresses)
	logger.Printf("[ipam-pool-monitor] Releasing IPCount in this batch %d", totalIpsSetForRelease)

	var tempNNCSpec nnc.NodeNetworkConfigSpec
	tempNNCSpec, err = pm.createNNCSpecForCRD(false)
	if err != nil {
		return err
	}

	tempNNCSpec.RequestedIPCount -= int64(totalIpsSetForRelease)
	logger.Printf("[ipam-pool-monitor] Decreasing pool size, Current Pool Size: %v, Requested IP Count: %v, Pods with IP's: %v, ToBeDeleted Count: %v", len(pm.cns.GetPodIPConfigState()), tempNNCSpec.RequestedIPCount, len(pm.cns.GetAllocatedIPConfigs()), len(tempNNCSpec.IPsNotInUse))

	err = pm.rc.UpdateCRDSpec(context.Background(), tempNNCSpec)
	if err != nil {
		// caller will retry to update the CRD again
		return err
	}

	// save the updated state to cachedSpec
	pm.cachedNNC.Spec = tempNNCSpec
	pm.pendingRelease = true
	return nil
}

// if cns pending ip release map is empty, request controller has already reconciled the CNS state,
// so we can remove it from our cache and remove the IP's from the CRD
func (pm *CNSIPAMPoolMonitor) cleanPendingRelease() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

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
		pendingIps := pm.cns.GetPendingReleaseIPConfigs()
		for _, pendingIp := range pendingIps {
			spec.IPsNotInUse = append(spec.IPsNotInUse, pendingIp.ID)
		}
	}

	return spec, nil
}

// UpdatePoolLimitsTransacted called by request controller on reconcile to set the batch size limits
func (pm *CNSIPAMPoolMonitor) Update(scalar nnc.Scaler, spec nnc.NodeNetworkConfigSpec) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.scalarUnits = scalar

	pm.MinimumFreeIps = int64(float64(pm.scalarUnits.BatchSize) * (float64(pm.scalarUnits.RequestThresholdPercent) / 100))
	pm.MaximumFreeIps = int64(float64(pm.scalarUnits.BatchSize) * (float64(pm.scalarUnits.ReleaseThresholdPercent) / 100))

	pm.cachedNNC.Spec = spec

	return nil
}
