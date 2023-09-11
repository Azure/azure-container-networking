package restserver

import (
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
)

type ipState struct {
	// allocatedIPs are all the IPs given to CNS by DNC.
	allocatedIPs int64
	// assignedIPs are the IPs CNS gives to Pods.
	assignedIPs int64
	// availableIPs are the IPs in state "Available".
	availableIPs int64
	// programmingIPs are the IPs in state "PendingProgramming".
	programmingIPs int64
	// releasingIPs are the IPs in state "PendingReleasr".
	releasingIPs int64
}

func (service *HTTPRestService) buildIPState() *ipState {
	service.Lock()
	defer service.Unlock()

	state := ipState{
		allocatedIPs:   0,
		assignedIPs:    0,
		availableIPs:   0,
		programmingIPs: 0,
		releasingIPs:   0,
	}

	//nolint:gocritic // This has to iterate over the IP Config state to get the counts.
	for _, ipConfig := range service.PodIPConfigState {
		state.allocatedIPs++
		if ipConfig.GetState() == types.Assigned {
			state.assignedIPs++
		}
		if ipConfig.GetState() == types.Available {
			state.availableIPs++
		}
		if ipConfig.GetState() == types.PendingProgramming {
			state.programmingIPs++
		}
		if ipConfig.GetState() == types.PendingRelease {
			state.releasingIPs++
		}
	}

	logger.Printf("[IP Usage] Allocated IPs: %d, Assigned IPs: %d, Available IPs: %d, PendingProgramming IPs: %d, PendingRelease IPs: %d",
		state.allocatedIPs,
		state.assignedIPs,
		state.availableIPs,
		state.programmingIPs,
		state.releasingIPs,
	)
	return &state
}
