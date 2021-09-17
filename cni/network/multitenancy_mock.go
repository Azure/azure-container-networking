package network

import (
	"context"
	"errors"
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/containernetworking/cni/pkg/types/current"
)

type MockMultitenancy struct {
	fail bool
}

const (
	ipPrefixLen      = 24
	localIPPrefixLen = 17
)

var errMockMulAdd = errors.New("multitenancy fail")

func NewMockMultitenancy(fail bool) *MockMultitenancy {
	return &MockMultitenancy{
		fail: fail,
	}
}

func (m *MockMultitenancy) GetMultiTenancyCNIResult(
	ctx context.Context,
	enableInfraVnet bool,
	nwCfg *cni.NetworkConfig,
	plugin *netPlugin,
	k8sPodName string,
	k8sNamespace string,
	ifName string) (*current.Result, *cns.GetNetworkContainerResponse, net.IPNet, *current.Result, error) {

	if m.fail {
		return nil, nil, net.IPNet{}, nil, errMockMulAdd
	}

	cnsResponse := &cns.GetNetworkContainerResponse{
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "192.168.0.4",
				PrefixLength: ipPrefixLen,
			},
			GatewayIPAddress: "192.168.0.1",
		},
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "169.254.0.4",
				PrefixLength: localIPPrefixLen,
			},
			GatewayIPAddress: "169.254.0.1",
		},

		PrimaryInterfaceIdentifier: "10.240.0.4/24",
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        1,
		},
	}

	_, ipnet, _ := net.ParseCIDR(cnsResponse.PrimaryInterfaceIdentifier)
	result := convertToCniResult(cnsResponse, "eth1")

	return result, cnsResponse, *ipnet, nil, nil
}

func (m *MockMultitenancy) CleanupMultitenancyResources(
	enableInfraVnet bool,
	nwCfg *cni.NetworkConfig,
	azIpamResult *current.Result,
	plugin *netPlugin) {
}

func (m *MockMultitenancy) SetupRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	cnsNetworkConfig *cns.GetNetworkContainerResponse,
	azIpamResult *current.Result,
	epInfo *network.EndpointInfo,
	result *current.Result) {
}

func (m *MockMultitenancy) DetermineSnatFeatureOnHost(snatFile, nmAgentSupportedApisURL string) (bool, bool, error) {
	return true, true, nil
}
