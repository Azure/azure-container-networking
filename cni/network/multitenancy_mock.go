package network

import (
	"context"
	"errors"
	"net"
	"runtime"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	current "github.com/containernetworking/cni/pkg/types/100"
)

type MockMultitenancy struct {
	fail bool
}

const (
	ipPrefixLen       = 24
	localIPPrefixLen  = 17
	multiTenancyVlan1 = 1
	multiTenancyVlan2 = 2
)

var errMockMulAdd = errors.New("multitenancy fail")

func NewMockMultitenancy(fail bool) *MockMultitenancy {
	return &MockMultitenancy{
		fail: fail,
	}
}

func (m *MockMultitenancy) Init(cnsclient cnsclient, netnetioshim netioshim) {}

func (m *MockMultitenancy) SetupRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	cnsNetworkConfig *cns.GetNetworkContainerResponse,
	azIpamResult *current.Result,
	epInfo *network.EndpointInfo,
	result *current.Result) {
}

func (m *MockMultitenancy) DetermineSnatFeatureOnHost(snatFile, nmAgentSupportedApisURL string) (snatDNS, snatHost bool, err error) {
	return true, true, nil
}

func (m *MockMultitenancy) GetNetworkContainer(
	ctx context.Context,
	nwCfg *cni.NetworkConfig,
	podName string,
	podNamespace string,
) (*cns.GetNetworkContainerResponse, net.IPNet, error) {
	if m.fail {
		return nil, net.IPNet{}, errMockMulAdd
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

	return cnsResponse, *ipnet, nil
}

func (m *MockMultitenancy) GetAllNetworkContainers(
	ctx context.Context,
	nwCfg *cni.NetworkConfig,
	podName string,
	podNamespace string,
	ifName string,
) ([]IPAMAddResult, error) {
	if m.fail {
		return nil, errMockMulAdd
	}

	var cnsResponses []cns.GetNetworkContainerResponse
	var ipNets []net.IPNet

	cnsResponseOne := &cns.GetNetworkContainerResponse{
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "20.0.0.10",
				PrefixLength: ipPrefixLen,
			},
			GatewayIPAddress: "20.0.0.1",
		},
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "168.254.0.4",
				PrefixLength: localIPPrefixLen,
			},
			GatewayIPAddress: "168.254.0.1",
		},

		PrimaryInterfaceIdentifier: "20.240.0.4/24",
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        multiTenancyVlan1,
		},
	}

	// TODO: add dual nic test cases for windows
	if runtime.GOOS == "windows" {
		cnsResponseTwo := &cns.GetNetworkContainerResponse{
			IPConfiguration: cns.IPConfiguration{
				IPSubnet: cns.IPSubnet{
					IPAddress:    "10.0.0.10",
					PrefixLength: ipPrefixLen,
				},
				GatewayIPAddress: "10.0.0.1",
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
				ID:        multiTenancyVlan2,
			},
		}

		_, secondIPnet, _ := net.ParseCIDR(cnsResponseTwo.PrimaryInterfaceIdentifier)
		ipNets = append(ipNets, *secondIPnet)
		cnsResponses = append(cnsResponses, *cnsResponseTwo)
	}

	_, firstIPnet, _ := net.ParseCIDR(cnsResponseOne.PrimaryInterfaceIdentifier)

	ipNets = append(ipNets, *firstIPnet)
	cnsResponses = append(cnsResponses, *cnsResponseOne)

	ipamResults := make([]IPAMAddResult, len(cnsResponses))
	for i := 0; i < len(cnsResponses); i++ {
		ipamResults[i].ncResponse = &cnsResponses[i]
		ipamResults[i].hostSubnetPrefix = ipNets[i]
		ipamResults[i].ipv4Result = convertToCniResult(ipamResults[i].ncResponse, ifName)
	}

	return ipamResults, nil
}
