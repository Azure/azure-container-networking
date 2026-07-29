package endpointmanager

import (
	"net"

	"github.com/Azure/azure-container-networking/cns/hnsclient"
)

// hnsEndpointClient abstracts the HNS package-level functions used by the
// Windows endpoint manager so they can be faked in unit tests.
type hnsEndpointClient interface {
	GetHNSEndpointbyIP(ipv4, ipv6 []net.IPNet) (string, error)
	DeleteHNSEndpointbyID(hnsEndpointID string) error
	DeleteNetworkByIDHnsV2(networkID string) error
}

type defaultHNSClient struct{}

func (defaultHNSClient) GetHNSEndpointbyIP(ipv4, ipv6 []net.IPNet) (string, error) {
	return hnsclient.GetHNSEndpointbyIP(ipv4, ipv6)
}

func (defaultHNSClient) DeleteHNSEndpointbyID(hnsEndpointID string) error {
	return hnsclient.DeleteHNSEndpointbyID(hnsEndpointID)
}

func (defaultHNSClient) DeleteNetworkByIDHnsV2(networkID string) error {
	return hnsclient.DeleteNetworkByIDHnsV2(networkID)
}

// hns is the HNS client used by the Windows endpoint manager. It is a package
// variable so tests can substitute a fake implementation.
var hns hnsEndpointClient = defaultHNSClient{}
