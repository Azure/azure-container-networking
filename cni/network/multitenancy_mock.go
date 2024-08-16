package network

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	current "github.com/containernetworking/cni/pkg/types/100"
)

type MockMultitenancy struct {
	fail         bool
	cnsResponses []*cns.GetNetworkContainerResponse
}

const (
	ipPrefixLen       = 24
	localIPPrefixLen  = 17
	multiTenancyVlan1 = 1
	multiTenancyVlan2 = 2
)

var errMockMulAdd = errors.New("multitenancy fail")

func NewMockMultitenancy(fail bool, cnsResponses []*cns.GetNetworkContainerResponse) *MockMultitenancy {
	return &MockMultitenancy{
		fail:         fail,
		cnsResponses: cnsResponses,
	}
}

func (m *MockMultitenancy) Init(cnsclient cnsclient, netnetioshim netioshim) {}

func (m *MockMultitenancy) SetupRoutingForMultitenancy(
	nwCfg *cni.NetworkConfig,
	cnsNetworkConfig *cns.GetNetworkContainerResponse,
	azIpamResult *current.Result,
	epInfo *network.EndpointInfo,
	_ *network.InterfaceInfo) {
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

	_, ipnet, _ := net.ParseCIDR(m.cnsResponses[0].PrimaryInterfaceIdentifier)

	return m.cnsResponses[0], *ipnet, nil
}

func (m *MockMultitenancy) GetAllNetworkContainers(
	ctx context.Context,
	nwCfg *cni.NetworkConfig,
	podName string,
	podNamespace string,
	ifName string,
) (IPAMAddResult, error) {
	if m.fail {
		return IPAMAddResult{}, errMockMulAdd
	}

	var cnsResponses []cns.GetNetworkContainerResponse
	var ipNets []net.IPNet

	for _, cnsResp := range m.cnsResponses {
		_, ipNet, _ := net.ParseCIDR(cnsResp.PrimaryInterfaceIdentifier)

		ipNets = append(ipNets, *ipNet)
		cnsResponses = append(cnsResponses, *cnsResp)
	}

	ipamResult := IPAMAddResult{}
	ipamResult.interfaceInfo = make(map[string]network.InterfaceInfo)

	for i := 0; i < len(cnsResponses); i++ {
		// one ncResponse gets you one interface info in the returned IPAMAddResult
		ifInfo := network.InterfaceInfo{
			NCResponse:       &cnsResponses[i],
			HostSubnetPrefix: ipNets[i],
		}

		ipconfig, routes := convertToIPConfigAndRouteInfo(ifInfo.NCResponse)
		ifInfo.IPConfigs = append(ifInfo.IPConfigs, ipconfig)
		ifInfo.Routes = routes
		ifInfo.NICType = cns.InfraNIC

		// assuming we only assign infra nics in this function
		ipamResult.interfaceInfo[string(ifInfo.NICType)+strconv.Itoa(i)] = ifInfo
	}

	return ipamResult, nil
}
