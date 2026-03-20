package hnsclient

// HNSClient abstracts HNS endpoint and network deletion for testability.
type HNSClient interface {
	DeleteEndpointByID(endpointID string) error
	DeleteNetworkByID(networkID string) error
}

type hnsClient struct{}

func NewHNSClient() HNSClient {
	return &hnsClient{}
}

func (*hnsClient) DeleteEndpointByID(endpointID string) error {
	return DeleteHNSEndpointbyID(endpointID)
}

func (*hnsClient) DeleteNetworkByID(networkID string) error {
	return DeleteNetworkByIDHnsV2(networkID)
}
