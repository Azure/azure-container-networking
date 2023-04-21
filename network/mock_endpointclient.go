package network

import (
	"errors"
	"fmt"
)

var errMockEpClient = errors.New("MockEndpointClient Error")

func newErrorMockEndpointClient(errStr string) error {
	return fmt.Errorf("%w : %s", errMockEpClient, errStr)
}

type MockEndpointClient struct {
	endpoints   map[string]bool
	returnError bool
}

func NewMockEndpointClient(returnError bool) *MockEndpointClient {
	client := &MockEndpointClient{
		endpoints:   make(map[string]bool),
		returnError: returnError,
	}

	return client
}

func (client *MockEndpointClient) AddEndpoints(epInfo *EndpointInfo) error {
	if ok := client.endpoints[epInfo.Id]; ok {
		return newErrorMockEndpointClient("Endpoint already exists")
	}

	client.endpoints[epInfo.Id] = true

	if client.returnError {
		return newErrorMockEndpointClient("AddEndpoints failed")
	}

	return nil
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
