package network

import (
	"errors"
	"fmt"
)

var errMockEpClient = errors.New("MockEndpointClient Error")

const (
	eth0IfName = "eth0"
)

func NewErrorMockEndpointClient(errStr string) error {
	return fmt.Errorf("%w : %s", errMockEpClient, errStr)
}

type MockEndpointClient struct {
	endpoints         map[string]bool
	testAddEndpointFn func(*EndpointInfo) error
}

func NewMockEndpointClient(fn func(*EndpointInfo) error) *MockEndpointClient {
	if fn == nil {
		fn = func(_ *EndpointInfo) error {
			return nil
		}
	}

	client := &MockEndpointClient{
		endpoints:         make(map[string]bool),
		testAddEndpointFn: fn,
	}

	return client
}

func (client *MockEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	if ok := client.endpoints[epInfo.Id]; ok && epInfo.IfName == eth0IfName {
		return NewErrorMockEndpointClient("Endpoint already exists")
	}

	client.endpoints[epInfo.Id] = true

	return client.testAddEndpointFn(epInfo)
}

func (client *MockEndpointClient) AddEndpointRules(_ *EndpointInfo) error {
	return nil
}

func (client *MockEndpointClient) DeleteEndpointRules(_ *endpoint) {
}

func (client *MockEndpointClient) MoveEndpointsToContainerNS(_ *EndpointInfo, _ uintptr) error {
	return nil
}

func (client *MockEndpointClient) SetupContainerInterfaces(_ *EndpointInfo) error {
	return nil
}

func (client *MockEndpointClient) ConfigureContainerInterfacesAndRoutes(_ *EndpointInfo) error {
	return nil
}

func (client *MockEndpointClient) DeleteEndpoints(ep *endpoint) error {
	delete(client.endpoints, ep.Id)
	return nil
}
