//go:build !ignore_uncovered
// +build !ignore_uncovered

package fakes

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/types"
)

type StringStack struct {
	sync.Mutex
	items []string
}

func NewStack() *StringStack {
	return &StringStack{items: make([]string, 0)}
}

func (stack *StringStack) Push(v string) {
	stack.Lock()
	defer stack.Unlock()

	stack.items = append(stack.items, v)
}

func (stack *StringStack) Pop() (string, error) {
	stack.Lock()
	defer stack.Unlock()

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
	AssignedIPConfigState       map[string]cns.IPConfigurationStatus
	PendingReleaseIPConfigState map[string]cns.IPConfigurationStatus
	AvailableIPIDStack          StringStack
	sync.RWMutex
}

func NewIPStateManager() IPStateManager {
	return IPStateManager{
		PendingProgramIPConfigState: make(map[string]cns.IPConfigurationStatus),
		AvailableIPConfigState:      make(map[string]cns.IPConfigurationStatus),
		AssignedIPConfigState:       make(map[string]cns.IPConfigurationStatus),
		PendingReleaseIPConfigState: make(map[string]cns.IPConfigurationStatus),
		AvailableIPIDStack:          StringStack{},
	}
}

func (ipm *IPStateManager) AddIPConfigs(ipconfigs []cns.IPConfigurationStatus) {
	ipm.Lock()
	defer ipm.Unlock()
	for _, ipconfig := range ipconfigs {
		switch ipconfig.GetState() {
		case types.PendingProgramming:
			ipm.PendingProgramIPConfigState[ipconfig.ID] = ipconfig
		case types.Available:
			ipm.AvailableIPConfigState[ipconfig.ID] = ipconfig
			ipm.AvailableIPIDStack.Push(ipconfig.ID)
		case types.Assigned:
			ipm.AssignedIPConfigState[ipconfig.ID] = ipconfig
		case types.PendingRelease:
			ipm.PendingReleaseIPConfigState[ipconfig.ID] = ipconfig
		}
	}
}

func (ipm *IPStateManager) RemovePendingReleaseIPConfigs(ipconfigNames []string) {
	ipm.Lock()
	defer ipm.Unlock()
	for _, name := range ipconfigNames {
		delete(ipm.PendingReleaseIPConfigState, name)
	}
}

func (ipm *IPStateManager) ReserveIPConfig() (cns.IPConfigurationStatus, error) {
	ipm.Lock()
	defer ipm.Unlock()
	id, err := ipm.AvailableIPIDStack.Pop()
	if err != nil {
		return cns.IPConfigurationStatus{}, err
	}
	ipc := ipm.AvailableIPConfigState[id]
	ipc.SetState(types.Assigned)
	ipm.AssignedIPConfigState[id] = ipc
	delete(ipm.AvailableIPConfigState, id)
	return ipm.AssignedIPConfigState[id], nil
}

func (ipm *IPStateManager) ReleaseIPConfig(ipconfigID string) (cns.IPConfigurationStatus, error) {
	ipm.Lock()
	defer ipm.Unlock()
	ipc := ipm.AssignedIPConfigState[ipconfigID]
	ipc.SetState(types.Available)
	ipm.AvailableIPConfigState[ipconfigID] = ipc
	ipm.AvailableIPIDStack.Push(ipconfigID)
	delete(ipm.AssignedIPConfigState, ipconfigID)
	return ipm.AvailableIPConfigState[ipconfigID], nil
}

func (ipm *IPStateManager) MarkIPAsPendingRelease(numberOfIPsToMark int) (map[string]cns.IPConfigurationStatus, error) {
	ipm.Lock()
	defer ipm.Unlock()

	var err error

	pendingReleaseIPs := make(map[string]cns.IPConfigurationStatus)

	defer func() {
		// if there was an error, and not all ip's have been freed, restore state
		if err != nil && len(pendingReleaseIPs) != numberOfIPsToMark {
			for uuid, ipState := range pendingReleaseIPs {
				ipState.SetState(types.Available)
				ipm.AvailableIPIDStack.Push(pendingReleaseIPs[uuid].ID)
				ipm.AvailableIPConfigState[pendingReleaseIPs[uuid].ID] = ipState
				delete(ipm.PendingReleaseIPConfigState, pendingReleaseIPs[uuid].ID)
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
		ipConfig.SetState(types.PendingRelease)
		pendingReleaseIPs[id] = ipConfig

		delete(ipm.AvailableIPConfigState, id)
	}

	// if no errors at this point, add the pending ips to the Pending state
	for _, pendingReleaseIP := range pendingReleaseIPs {
		ipm.PendingReleaseIPConfigState[pendingReleaseIP.ID] = pendingReleaseIP
	}

	return pendingReleaseIPs, nil
}

var _ cns.HTTPService = (*HTTPServiceFake)(nil)

type HTTPServiceFake struct {
	IPStateManager IPStateManager
	PoolMonitor    cns.IPAMPoolMonitor
}

func NewHTTPServiceFake() *HTTPServiceFake {
	return &HTTPServiceFake{
		IPStateManager: NewIPStateManager(),
	}
}

func (fake *HTTPServiceFake) SetNumberOfAssignedIPs(assign int) error {
	currentAssigned := len(fake.IPStateManager.AssignedIPConfigState)
	delta := (assign - currentAssigned)

	if delta > 0 {
		// assign IPs
		for i := 0; i < delta; i++ {
			if _, err := fake.IPStateManager.ReserveIPConfig(); err != nil {
				return err
			}
		}
		return nil
	}
	// unassign IPs
	delta *= -1
	i := 0
	for id := range fake.IPStateManager.AssignedIPConfigState {
		if i >= delta {
			break
		}
		if _, err := fake.IPStateManager.ReleaseIPConfig(id); err != nil {
			return err
		}
		i++
	}
	return nil
}

func (fake *HTTPServiceFake) SendNCSnapShotPeriodically(context.Context, int) {}

func (fake *HTTPServiceFake) SetNodeOrchestrator(*cns.SetOrchestratorTypeRequest) {}

func (fake *HTTPServiceFake) SyncNodeStatus(string, string, string, json.RawMessage) (types.ResponseCode, string) {
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

func (fake *HTTPServiceFake) GetAssignedIPConfigs() []cns.IPConfigurationStatus {
	ipconfigs := []cns.IPConfigurationStatus{}
	for _, ipconfig := range fake.IPStateManager.AssignedIPConfigState {
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
	for key, val := range fake.IPStateManager.AssignedIPConfigState {
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

func (fake *HTTPServiceFake) AttachSWIFTv2Middleware(cns.SWIFTv2Middleware) {}
