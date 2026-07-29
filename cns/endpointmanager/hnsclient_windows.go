package endpointmanager

import (
	"net"

	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/pkg/errors"
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
	endpointID, err := hnsclient.GetHNSEndpointbyIP(ipv4, ipv6)
	return endpointID, errors.Wrap(err, "get hns endpoint by IP")
}

func (defaultHNSClient) DeleteHNSEndpointbyID(hnsEndpointID string) error {
	return errors.Wrap(hnsclient.DeleteHNSEndpointbyID(hnsEndpointID), "delete hns endpoint by ID")
}

func (defaultHNSClient) DeleteNetworkByIDHnsV2(networkID string) error {
	return errors.Wrap(hnsclient.DeleteNetworkByIDHnsV2(networkID), "delete hns network by ID")
}

// hns is the HNS client used by the Windows endpoint manager. It is a package
// variable so tests can substitute a fake implementation.
var hns hnsEndpointClient = defaultHNSClient{}
