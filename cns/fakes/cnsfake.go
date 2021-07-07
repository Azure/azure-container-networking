package fakes

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	nnc "github.com/Azure/azure-container-networking/nodenetworkconfig/api/v1alpha"
)

// available IP's stack
// all IP's map

type StringStack struct {
	lock  sync.Mutex // you don't have to do this if you don't want thread safety
	items []string
}

func NewFakeScalar(releaseThreshold, requestThreshold, batchSize int) nnc.Scaler {
	return nnc.Scaler{
		BatchSize:               int64(batchSize),
		ReleaseThresholdPercent: int64(releaseThreshold),
		RequestThresholdPercent: int64(requestThreshold),
	}
}

func NewFakeNodeNetworkConfigSpec(requestedIPCount int) nnc.NodeNetworkConfigSpec {
	return nnc.NodeNetworkConfigSpec{
		RequestedIPCount: int64(requestedIPCount),
	}
}

func NewStack() *StringStack {
	return &StringStack{sync.Mutex{}, make([]string, 0)}
}

func (stack *StringStack) Push(v string) {
	stack.lock.Lock()
	defer stack.lock.Unlock()

	stack.items = append(stack.items, v)
}

func (stack *StringStack) Pop() (string, error) {
	stack.lock.Lock()
	defer stack.lock.Unlock()

	length := len(stack.items)
	if length == 0 {
		return "", errors.New("Empty Stack")
	}

	res := stack.items[length-1]
	stack.items = stack.items[:length-1]
	return res, nil
}

type IPStateManager struct {
	PendingProgramIPConfigState map[string]cns.IPConfigurationStatus
	AvailableIPConfigState      map[string]cns.IPConfigurationStatus
	AllocatedIPConfigState      map[string]cns.IPConfigurationStatus
	PendingReleaseIPConfigState map[string]cns.IPConfigurationStatus
	AvailableIPIDStack          StringStack
	sync.RWMutex
}

func NewIPStateManager() IPStateManager {
	return IPStateManager{
		PendingProgramIPConfigState: make(map[string]cns.IPConfigurationStatus),
		AvailableIPConfigState:      make(map[string]cns.IPConfigurationStatus),
		AllocatedIPConfigState:      make(map[string]cns.IPConfigurationStatus),
		PendingReleaseIPConfigState: make(map[string]cns.IPConfigurationStatus),
		AvailableIPIDStack:          StringStack{},
	}
}

func (ipm *IPStateManager) AddIPConfigs(ipconfigs []cns.IPConfigurationStatus) {
	ipm.Lock()
	defer ipm.Unlock()

	for i := 0; i < len(ipconfigs); i++ {

		switch {
		case ipconfigs[i].State == cns.PendingProgramming:
			ipm.PendingProgramIPConfigState[ipconfigs[i].ID] = ipconfigs[i]
		case ipconfigs[i].State == cns.Available:
			ipm.AvailableIPConfigState[ipconfigs[i].ID] = ipconfigs[i]
			ipm.AvailableIPIDStack.Push(ipconfigs[i].ID)
		case ipconfigs[i].State == cns.Allocated:
			ipm.AllocatedIPConfigState[ipconfigs[i].ID] = ipconfigs[i]
		case ipconfigs[i].State == cns.PendingRelease:
			ipm.PendingReleaseIPConfigState[ipconfigs[i].ID] = ipconfigs[i]
		}
	}
}

func (ipm *IPStateManager) RemovePendingReleaseIPConfigs(ipconfigNames []string) {
	ipm.Lock()
	defer ipm.Unlock()

	for i := 0; i < len(ipconfigNames); i++ {
		delete(ipm.PendingReleaseIPConfigState, ipconfigNames[i])
	}
}

func (ipm *IPStateManager) ReserveIPConfig() (cns.IPConfigurationStatus, error) {
	ipm.Lock()
	defer ipm.Unlock()
	id, err := ipm.AvailableIPIDStack.Pop()
	if err != nil {
		return cns.IPConfigurationStatus{}, err
	}
	ipm.AllocatedIPConfigState[id] = ipm.AvailableIPConfigState[id]
	delete(ipm.AvailableIPConfigState, id)
	return ipm.AllocatedIPConfigState[id], nil
}

func (ipm *IPStateManager) ReleaseIPConfig(ipconfigID string) (cns.IPConfigurationStatus, error) {
	ipm.Lock()
	defer ipm.Unlock()
	ipm.AvailableIPConfigState[ipconfigID] = ipm.AllocatedIPConfigState[ipconfigID]
	ipm.AvailableIPIDStack.Push(ipconfigID)
	delete(ipm.AllocatedIPConfigState, ipconfigID)
	return ipm.AvailableIPConfigState[ipconfigID], nil
}

func (ipm *IPStateManager) MarkIPAsPendingRelease(numberOfIPsToMark int) (map[string]cns.IPConfigurationStatus, error) {
	ipm.Lock()
	defer ipm.Unlock()

	var (
		err error
	)

	pendingRelease := make(map[string]cns.IPConfigurationStatus)

	defer func() {
		// if there was an error, and not all ip's have been freed, restore state
		if err != nil && len(pendingRelease) != numberOfIPsToMark {
			for uuid, ipState := range pendingRelease {
				ipState.State = cns.Available
				ipm.AvailableIPIDStack.Push(pendingRelease[uuid].ID)
				ipm.AvailableIPConfigState[pendingRelease[uuid].ID] = ipState
				delete(ipm.PendingReleaseIPConfigState, pendingRelease[uuid].ID)
			}
		}
	}()

	for i := 0; i < numberOfIPsToMark; i++ {
		id, err := ipm.AvailableIPIDStack.Pop()
		if err != nil {
			return ipm.PendingReleaseIPConfigState, err
		}

		// add all pending release to a slice
		ipConfig := ipm.AvailableIPConfigState[id]
		ipConfig.State = cns.PendingRelease
		pendingRelease[id] = ipConfig

		delete(ipm.AvailableIPConfigState, id)
	}

	// if no errors at this point, add the pending ips to the Pending state
	for i := range pendingRelease {
		ipm.PendingReleaseIPConfigState[pendingRelease[i].ID] = pendingRelease[i]
	}

	return pendingRelease, nil
}

var _ cns.HTTPService = (*HTTPServiceFake)(nil)

type HTTPServiceFake struct {
	IPStateManager IPStateManager
	PoolMonitor    cns.IPAMPoolMonitor
}

func NewHTTPServiceFake() *HTTPServiceFake {
	svc := &HTTPServiceFake{
		IPStateManager: NewIPStateManager(),
	}

	return svc
}

func (fake *HTTPServiceFake) SetNumberOfAllocatedIPs(desiredAllocatedIPCount int) error {
	currentAllocatedIPCount := len(fake.IPStateManager.AllocatedIPConfigState)
	delta := (desiredAllocatedIPCount - currentAllocatedIPCount)

	if delta > 0 {
		for i := 0; i < delta; i++ {
			if _, err := fake.IPStateManager.ReserveIPConfig(); err != nil {
				return err
			}
		}
	} else if delta < 0 {

		// deallocate IP's
		delta *= -1
		i := 0
		for id := range fake.IPStateManager.AllocatedIPConfigState {
			if i < delta {
				if _, err := fake.IPStateManager.ReleaseIPConfig(id); err != nil {
					return err
				}

			} else {
				break
			}
			i++
		}
	}

	return nil
}

func (fake *HTTPServiceFake) SendNCSnapShotPeriodically(context.Context, int) {}

func (fake *HTTPServiceFake) SetNodeOrchestrator(*cns.SetOrchestratorTypeRequest) {}

func (fake *HTTPServiceFake) SyncNodeStatus(string, string, string, json.RawMessage) (int, string) {
	return 0, ""
}

// SyncHostNCVersion will update HostVersion in containerstatus.
func (fake *HTTPServiceFake) SyncHostNCVersion(context.Context, string, time.Duration) {}

func (fake *HTTPServiceFake) GetPendingProgramIPConfigs() []cns.IPConfigurationStatus {
	ipconfigs := []cns.IPConfigurationStatus{}
	for _, ipconfig := range fake.IPStateManager.PendingProgramIPConfigState {
		ipconfigs = append(ipconfigs, ipconfig)
	}

	return ipconfigs
}

func (fake *HTTPServiceFake) GetAvailableIPConfigs() []cns.IPConfigurationStatus {
	ipconfigs := []cns.IPConfigurationStatus{}
	for _, ipconfig := range fake.IPStateManager.AvailableIPConfigState {
		ipconfigs = append(ipconfigs, ipconfig)
	}

	return ipconfigs
}

func (fake *HTTPServiceFake) GetAllocatedIPConfigs() []cns.IPConfigurationStatus {
	ipconfigs := []cns.IPConfigurationStatus{}
	for _, ipconfig := range fake.IPStateManager.AllocatedIPConfigState {
		ipconfigs = append(ipconfigs, ipconfig)
	}

	return ipconfigs
}

func (fake *HTTPServiceFake) GetPendingReleaseIPConfigs() []cns.IPConfigurationStatus {
	ipconfigs := []cns.IPConfigurationStatus{}
	for _, ipconfig := range fake.IPStateManager.PendingReleaseIPConfigState {
		ipconfigs = append(ipconfigs, ipconfig)
	}

	return ipconfigs
}

// Return union of all state maps
func (fake *HTTPServiceFake) GetPodIPConfigState() map[string]cns.IPConfigurationStatus {
	ipconfigs := make(map[string]cns.IPConfigurationStatus)
	for key, val := range fake.IPStateManager.AllocatedIPConfigState {
		ipconfigs[key] = val
	}

	for key, val := range fake.IPStateManager.AvailableIPConfigState {
		ipconfigs[key] = val
	}

	for key, val := range fake.IPStateManager.PendingReleaseIPConfigState {
		ipconfigs[key] = val
	}

	return ipconfigs
}

// TODO: Populate on scale down
func (fake *HTTPServiceFake) MarkIPAsPendingRelease(numberToMark int) (map[string]cns.IPConfigurationStatus, error) {
	return fake.IPStateManager.MarkIPAsPendingRelease(numberToMark)
}

func (fake *HTTPServiceFake) GetOption(string) interface{} {
	return nil
}

func (fake *HTTPServiceFake) SetOption(string, interface{}) {}

func (fake *HTTPServiceFake) Start(*common.ServiceConfig) error {
	return nil
}

func (fake *HTTPServiceFake) Init(*common.ServiceConfig) error {
	return nil
}

func (fake *HTTPServiceFake) Stop() {}
