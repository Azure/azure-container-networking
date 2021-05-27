package nns

import (
	"context"
	contracts "github.com/Azure/azure-container-networking/proto/nodenetworkservice/3.302.0.744"
)

// Mock client to simulate Node network service APIs
type MockGrpcClient struct {
}

// Add container to the network. Container Id is appended to the podName
func (c *MockGrpcClient) AddContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (error, *contracts.ConfigureContainerNetworkingResponse) {

	return nil, &contracts.ConfigureContainerNetworkingResponse{}
}

// Add container to the network. Container Id is appended to the podName
func (c *MockGrpcClient) DeleteContainerNetworking(
	ctx context.Context,
	podName, nwNamespace string) (error, *contracts.ConfigureContainerNetworkingResponse) {

	return nil, &contracts.ConfigureContainerNetworkingResponse{}
}
