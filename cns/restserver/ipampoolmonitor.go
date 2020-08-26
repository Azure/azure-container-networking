package restserver

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

type IPAMPoolMonitor interface {
	Start()
	UpdatePoolLimitsTransacted(batchSize int64, requestThreshold float64, releaseThreshold float64)
}

type CNSIPAMPoolMonitor struct {
	initialized bool

	cns *HTTPRestService
	rc  requestcontroller.RequestController

	batchSize        int
	requestThreshold float64
	releaseThreshold float64
	minimumFreeIps   int
	maximumFreeIps   int
	sync.RWMutex
}

func NewCNSIPAMPoolMonitor(cnsService *HTTPRestService, requestController requestcontroller.RequestController) CNSIPAMPoolMonitor {
	return CNSIPAMPoolMonitor{
		initialized: false,
		cns:         cnsService,
		rc:          requestController,
	}
}

// UpdatePoolLimitsTransacted called by request controller on reconcile to set the batch size limits
func (pm *CNSIPAMPoolMonitor) UpdatePoolLimitsTransacted(batchSize int, requestThreshold float64, releaseThreshold float64) {

	pm.Lock()
	defer pm.Unlock()
	pm.batchSize = batchSize
	pm.requestThreshold = requestThreshold
	pm.releaseThreshold = releaseThreshold

	//TODO: rounding
	pm.minimumFreeIps = int(float64(pm.batchSize) * requestThreshold)
	pm.maximumFreeIps = int(float64(pm.batchSize) * releaseThreshold)
	pm.initialized = true
}

func (pm *CNSIPAMPoolMonitor) checkForResize(freeIPConfigCount int) int {
	switch {
	// pod count is increasing
	case freeIPConfigCount < pm.minimumFreeIps:
		logger.Printf("Number of free IP's (%.1f) < minimum free IPs (%.1f), request batch increase\n", freeIPConfigCount, pm.minimumFreeIps)
		return increasePoolSize

	// pod count is decreasing
	case freeIPConfigCount > pm.maximumFreeIps:
		logger.Printf("Number of free IP's (%.1f) > maximum free IPs (%.1f), request batch decrease\n", freeIPConfigCount, pm.maximumFreeIps)
		return decreasePoolSize
	}
	return doNothing
}

func (pm *CNSIPAMPoolMonitor) increasePoolSize() error {
	increaseIPCount := len(pm.cns.PodIPConfigState) + pm.batchSize

	// pass nil map to CNStoCRDSpec because we don't want to modify the to be deleted ipconfigs
	spec, err := CNSToCRDSpec(nil, increaseIPCount)
	if err != nil {
		return err
	}

	return pm.rc.UpdateCRDSpec(context.Background(), spec)
}

func (pm *CNSIPAMPoolMonitor) decreasePoolSize() error {

	// TODO: Better handling here, negatives?
	decreaseIPCount := len(pm.cns.PodIPConfigState) - pm.batchSize

	// mark n number of IP's as pending
	pendingIPAddresses, err := pm.cns.MarkIPsAsPendingTransacted(decreaseIPCount)
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

// TODO: add looping and cancellation to this, and add to CNS MAIN
func (pm *CNSIPAMPoolMonitor) Start() (err error) {

	if pm.initialized {
		availableIPConfigs := pm.cns.GetAvailableIPConfigs()
		rebatchAction := pm.checkForResize(len(availableIPConfigs))
		switch rebatchAction {
		case increasePoolSize:
			pm.increasePoolSize()
		case decreasePoolSize:
			pm.decreasePoolSize()
		}
	}
	return err
}
