package network

import (
	"fmt"

	cnms "github.com/Azure/azure-container-networking/cnms/cnmspackage"
	"github.com/Azure/azure-container-networking/common"
)

//MockNetworkManager is a mock structure for Network Manager
type MockNetworkManager struct {
	TestNetworkInfoMap  map[string]*NetworkInfo
	TestEndpointInfoMap map[string]*EndpointInfo
}

//NewMockNetworkmanager returns a new mock
func NewMockNetworkmanager() *MockNetworkManager {
	return &MockNetworkManager{
		TestNetworkInfoMap:  make(map[string]*NetworkInfo),
		TestEndpointInfoMap: make(map[string]*EndpointInfo),
	}
}

//Initialize mock
func (nm *MockNetworkManager) Initialize(config *common.PluginConfig, isRehydrationRequired bool) error {
	return nil
}

//Uninitialize mock
func (nm *MockNetworkManager) Uninitialize() {}

//AddExternalInterface mock
func (nm *MockNetworkManager) AddExternalInterface(ifName string, subnet string) error {
	return nil
}

//CreateNetwork mock
func (nm *MockNetworkManager) CreateNetwork(nwInfo *NetworkInfo) error {
	nm.TestNetworkInfoMap[nwInfo.Id] = nwInfo
	return nil
}

//DeleteNetwork mock
func (nm *MockNetworkManager) DeleteNetwork(networkID string) error {
	return nil
}

//GetNetworkInfo mock
func (nm *MockNetworkManager) GetNetworkInfo(networkID string) (NetworkInfo, error) {
	if info, exists := nm.TestNetworkInfoMap[networkID]; exists {
		return *info, nil
	}
	return NetworkInfo{}, fmt.Errorf("Not found")
}

//CreateEndpoint mock
func (nm *MockNetworkManager) CreateEndpoint(networkID string, epInfo *EndpointInfo) error {
	nm.TestEndpointInfoMap[epInfo.Id] = epInfo
	return nil
}

//DeleteEndpoint mock
func (nm *MockNetworkManager) DeleteEndpoint(networkID string, endpointID string) error {
	return nil
}

func (nm *MockNetworkManager) GetAllEndpoints(networkID string) (map[string]*EndpointInfo, error) {
	return nm.TestEndpointInfoMap, nil
}

//GetEndpointInfo mock
func (nm *MockNetworkManager) GetEndpointInfo(networkID string, endpointID string) (*EndpointInfo, error) {
	return nm.TestEndpointInfoMap[networkID], nil
}

//GetEndpointInfoBasedOnPODDetails mock
func (nm *MockNetworkManager) GetEndpointInfoBasedOnPODDetails(networkID string, podName string, podNameSpace string, doExactMatchForPodName bool) (*EndpointInfo, error) {
	return &EndpointInfo{}, nil
}

//AttachEndpoint mock
func (nm *MockNetworkManager) AttachEndpoint(networkID string, endpointID string, sandboxKey string) (*endpoint, error) {
	return &endpoint{}, nil
}

//DetachEndpoint mock
func (nm *MockNetworkManager) DetachEndpoint(networkID string, endpointID string) error {
	return nil
}

//UpdateEndpoint mock
func (nm *MockNetworkManager) UpdateEndpoint(networkID string, existingEpInfo *EndpointInfo, targetEpInfo *EndpointInfo) error {
	return nil
}

//GetNumberOfEndpoints mock
func (nm *MockNetworkManager) GetNumberOfEndpoints(ifName string, networkID string) int {
	return 0
}

//SetupNetworkUsingState mock
func (nm *MockNetworkManager) SetupNetworkUsingState(networkMonitor *cnms.NetworkMonitor) error {
	return nil
}
