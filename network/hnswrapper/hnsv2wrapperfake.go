package hnswrapper

import (
	"github.com/Microsoft/hcsshim/hcn"
	uuid "github.com/satori/go.uuid"
)

type Hnsv2wrapperFake struct {
}

func (f Hnsv2wrapperFake) CreateNetwork(network *hcn.HostComputeNetwork) (*hcn.HostComputeNetwork, error) {
	return network,nil
}

func (f Hnsv2wrapperFake) DeleteNetwork(network *hcn.HostComputeNetwork) error {
	return nil
}

func (f Hnsv2wrapperFake) GetNetworkByID(networkId string) (*hcn.HostComputeNetwork, error) {
	network := &hcn.HostComputeNetwork{Id: uuid.NewV4().String()}
	return network,nil
}

func (f Hnsv2wrapperFake) GetEndpointByID(endpointId string) (*hcn.HostComputeEndpoint, error) {
	endpoint := &hcn.HostComputeEndpoint{Id: uuid.NewV4().String()}
	return endpoint,nil
}

func (Hnsv2wrapperFake) CreateEndpoint(endpoint *hcn.HostComputeEndpoint)  (*hcn.HostComputeEndpoint, error)  {
	return endpoint, nil
}

func (Hnsv2wrapperFake) DeleteEndpoint(endpoint *hcn.HostComputeEndpoint) error {
	return nil
}

func (Hnsv2wrapperFake) GetNamespaceByID(netNamespacePath string) (*hcn.HostComputeNamespace, error) {
	nameSpace := &hcn.HostComputeNamespace{Id: uuid.NewV4().String(), NamespaceId: 1000}
	return nameSpace, nil
}

func (Hnsv2wrapperFake) AddNamespaceEndpoint(namespaceId string, hnsResponseId string) error {
	return nil
}

func (Hnsv2wrapperFake) RemoveNamespaceEndpoint(namespaceId string, hnsResponseId string) error {
	return nil
}
