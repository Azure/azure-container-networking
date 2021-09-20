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
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
)

type StringStack struct {
	lock  sync.Mutex // you don't have to do this if you don't want thread safety
	items []string
}

func NewFakeScalar(releaseThreshold, requestThreshold, batchSize int) v1alpha.Scaler {
	return v1alpha.Scaler{
		BatchSize:               int64(batchSize),
		ReleaseThresholdPercent: int64(releaseThreshold),
		RequestThresholdPercent: int64(requestThreshold),
	}
}

func NewFakeNodeNetworkConfigSpec(requestedIPCount int) v1alpha.NodeNetworkConfigSpec {
	return v1alpha.NodeNetworkConfigSpec{
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
	for _, ipconfig := range ipconfigs {
		switch ipconfig.State {
		case cns.PendingProgramming:
			ipm.PendingProgramIPConfigState[ipconfig.ID] = ipconfig
		case cns.Available:
			ipm.AvailableIPConfigState[ipconfig.ID] = ipconfig
			ipm.AvailableIPIDStack.Push(ipconfig.ID)
		case cns.Allocated:
			ipm.AllocatedIPConfigState[ipconfig.ID] = ipconfig
		case cns.PendingRelease:
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

	var err error

	pendingReleaseIPs := make(map[string]cns.IPConfigurationStatus)

	defer func() {
		// if there was an error, and not all ip's have been freed, restore state
		if err != nil && len(pendingReleaseIPs) != numberOfIPsToMark {
			for uuid, ipState := range pendingReleaseIPs {
				ipState.State = cns.Available
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
		ipConfig.State = cns.PendingRelease
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

func (fake *HTTPServiceFake) SetNumberOfAllocatedIPs(desiredAllocatedIPCount int) error {
	currentAllocatedIPCount := len(fake.IPStateManager.AllocatedIPConfigState)
	delta := (desiredAllocatedIPCount - currentAllocatedIPCount)

	if delta > 0 {
		// allocated IPs
		for i := 0; i < delta; i++ {
			if _, err := fake.IPStateManager.ReserveIPConfig(); err != nil {
				return err
			}
		}
		return nil
	}
	// deallocate IPs
	delta *= -1
	i := 0
	for id := range fake.IPStateManager.AllocatedIPConfigState {
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
