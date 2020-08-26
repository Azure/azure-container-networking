package ipampoolmonitor

import (
	"context"
	"sync"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/requestcontroller"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

var (
	increasePoolSize = 1
	decreasePoolSize = -1
	doNothing        = 0
)

type CNSIPAMPoolMonitor struct {
	initialized bool

	cns            cns.HTTPService
	rc             requestcontroller.RequestController
	scalarUnits    cns.ScalarUnits
	MinimumFreeIps int
	MaximumFreeIps int

	sync.RWMutex
}

func NewCNSIPAMPoolMonitor(cnsService cns.HTTPService, requestController requestcontroller.RequestController) *CNSIPAMPoolMonitor {
	return &CNSIPAMPoolMonitor{
		initialized: false,
		cns:         cnsService,
		rc:          requestController,
	}
}

// TODO: add looping and cancellation to this, and add to CNS MAIN
func (pm *CNSIPAMPoolMonitor) Start() error {

	if pm.initialized {
		availableIPConfigs := pm.cns.GetAvailableIPConfigs()
		rebatchAction := pm.checkForResize(len(availableIPConfigs))
		switch rebatchAction {
		case increasePoolSize:
			return pm.increasePoolSize()
		case decreasePoolSize:
			return pm.decreasePoolSize()
		}
	}

	return nil
}

// UpdatePoolLimitsTransacted called by request controller on reconcile to set the batch size limits
func (pm *CNSIPAMPoolMonitor) UpdatePoolLimitsTransacted(scalarUnits cns.ScalarUnits) {
	pm.Lock()
	defer pm.Unlock()
	pm.scalarUnits = scalarUnits

	// TODO rounding?
	pm.MinimumFreeIps = int(pm.scalarUnits.BatchSize * (pm.scalarUnits.RequestThresholdPercent / 100))
	pm.MaximumFreeIps = int(pm.scalarUnits.BatchSize * (pm.scalarUnits.ReleaseThresholdPercent / 100))

	pm.initialized = true
}

func (pm *CNSIPAMPoolMonitor) checkForResize(freeIPConfigCount int) int {
	switch {
	// pod count is increasing
	case freeIPConfigCount < pm.MinimumFreeIps:
		logger.Printf("Number of free IP's (%d) < minimum free IPs (%d), request batch increase\n", freeIPConfigCount, pm.MinimumFreeIps)
		return increasePoolSize

	// pod count is decreasing
	case freeIPConfigCount > pm.MaximumFreeIps:
		logger.Printf("Number of free IP's (%d) > maximum free IPs (%d), request batch decrease\n", freeIPConfigCount, pm.MaximumFreeIps)
		return decreasePoolSize
	}
	return doNothing
}

func (pm *CNSIPAMPoolMonitor) increasePoolSize() error {
	increaseIPCount := len(pm.cns.GetPodIPConfigState()) + int(pm.scalarUnits.BatchSize)

	// pass nil map to CNStoCRDSpec because we don't want to modify the to be deleted ipconfigs
	spec, err := CNSToCRDSpec(nil, increaseIPCount)
	if err != nil {
		return err
	}

	return pm.rc.UpdateCRDSpec(context.Background(), spec)
}

func (pm *CNSIPAMPoolMonitor) decreasePoolSize() error {

	// TODO: Better handling here, negatives
	// TODO: Maintain desired state to check against if pool size adjustment is already happening
	decreaseIPCount := len(pm.cns.GetPodIPConfigState()) - int(pm.scalarUnits.BatchSize)

	// mark n number of IP's as pending
	pendingIPAddresses, err := pm.cns.MarkIPsAsPending(decreaseIPCount)
	if err != nil {
		return err
	}

	// convert the pending IP addresses to a spec
	spec, err := CNSToCRDSpec(pendingIPAddresses, decreaseIPCount)
	if err != nil {
		return err
	}

	return pm.rc.UpdateCRDSpec(context.Background(), spec)
}

// CNSToCRDSpec translates CNS's map of Ips to be released and requested ip count into a CRD Spec
func CNSToCRDSpec(toBeDeletedSecondaryIPConfigs map[string]cns.SecondaryIPConfig, ipCount int) (nnc.NodeNetworkConfigSpec, error) {
	var (
		spec nnc.NodeNetworkConfigSpec
		uuid string
	)

	spec.RequestedIPCount = int64(ipCount)

	for uuid = range toBeDeletedSecondaryIPConfigs {
		spec.IPsNotInUse = append(spec.IPsNotInUse, uuid)
	}

	return spec, nil
}
