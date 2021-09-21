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

// ErrMockNnsAdd - mock add failure
var ErrMockNnsAdd = errors.New("mock nns add fail")

// AddContainerNetworking - Mock nns add
func (c *MockGrpcClient) AddContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (*contracts.ConfigureContainerNetworkingResponse, error) {
	if c.Fail {
		return nil, ErrMockNnsAdd
	}

	return &contracts.ConfigureContainerNetworkingResponse{}, nil
}

// DeleteContainerNetworking - Mock nns delete
func (c *MockGrpcClient) DeleteContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (*contracts.ConfigureContainerNetworkingResponse, error) {

	return &contracts.ConfigureContainerNetworkingResponse{}, nil
}
