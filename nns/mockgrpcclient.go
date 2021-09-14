package nns

import (
	"context"
	"errors"

	contracts "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
)

// Mock client to simulate Node network service APIs
type MockGrpcClient struct {
	Fail bool
}

var ErrMockNnsAdd = errors.New("mock nns add fail")

// Add container to the network. Container Id is appended to the podName
func (c *MockGrpcClient) AddContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (*contracts.ConfigureContainerNetworkingResponse, error) {
	if c.Fail {
		return nil, ErrMockNnsAdd
	}

	return &contracts.ConfigureContainerNetworkingResponse{}, nil
}

// Add container to the network. Container Id is appended to the podName
func (c *MockGrpcClient) DeleteContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (*contracts.ConfigureContainerNetworkingResponse, error) {

	return &contracts.ConfigureContainerNetworkingResponse{}, nil
}
