package network

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/stretchr/testify/require"
)

// Handler structs
type requestIPAddressHandler struct {
	// arguments
	ipconfigArgument cns.IPConfigRequest

	// results
	result *cns.IPConfigResponse
	err    error
}

type releaseIPAddressHandler struct {
	ipconfigArgument cns.IPConfigRequest
	err              error
}

type getNetworkContainerConfigurationHandler struct {
	orchestratorContext []byte
	returnResponse      *cns.GetNetworkContainerResponse
	err                 error
}

type getNetworkContainersConfigurationHandler struct {
	orchestratorContext []byte
	returnResponse      []cns.GetNetworkContainerResponse
	err                 error
}

type MockCNSClient struct {
	require                           *require.Assertions
	request                           requestIPAddressHandler
	release                           releaseIPAddressHandler
	getNetworkContainerConfiguration  getNetworkContainerConfigurationHandler
	getNetworkContainersConfiguration getNetworkContainersConfigurationHandler
}

func (c *MockCNSClient) RequestIPAddress(_ context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	c.require.Exactly(c.request.ipconfigArgument, ipconfig)
	return c.request.result, c.request.err
}

func (c *MockCNSClient) ReleaseIPAddress(_ context.Context, ipconfig cns.IPConfigRequest) error {
	c.require.Exactly(c.release.ipconfigArgument, ipconfig)
	return c.release.err
}

func (c *MockCNSClient) GetNetworkContainer(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error) {
	c.require.Exactly(c.getNetworkContainerConfiguration.orchestratorContext, orchestratorContext)
	return c.getNetworkContainerConfiguration.returnResponse, c.getNetworkContainerConfiguration.err
}

func (c *MockCNSClient) GetNetworkContainers(ctx context.Context, orchestratorContext []byte) ([]cns.GetNetworkContainerResponse, error) {
	c.require.Exactly(c.getNetworkContainersConfiguration.orchestratorContext, orchestratorContext)
	return c.getNetworkContainersConfiguration.returnResponse, c.getNetworkContainersConfiguration.err
}

func defaultIPNet() *net.IPNet {
	_, defaultIPNet, _ := net.ParseCIDR("0.0.0.0/0")
	return defaultIPNet
}

func marshallPodInfo(podInfo cns.KubernetesPodInfo) []byte {
	orchestratorContext, _ := json.Marshal(podInfo)
	return orchestratorContext
}

type mockNetIOShim struct{}

func (a *mockNetIOShim) GetInterfaceSubnetWithSpecificIP(ipAddr string) *net.IPNet {
	return getCIDRNotationForAddress(ipAddr)
}

func getIPNet(ipaddr net.IP, mask net.IPMask) net.IPNet {
	return net.IPNet{
		IP:   ipaddr,
		Mask: mask,
	}
}

func getIPNetWithString(ipaddrwithcidr string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(ipaddrwithcidr)
	if err != nil {
		panic(err)
	}

	return ipnet
}

func TestSetupRoutingForMultitenancy(t *testing.T) {
	require := require.New(t) //nolint:gocritic
	type args struct {
		nwCfg            *cni.NetworkConfig
		cnsNetworkConfig *cns.GetNetworkContainerResponse
		azIpamResult     *cniTypesCurr.Result
		epInfo           *network.EndpointInfo
		result           *cniTypesCurr.Result
	}

	tests := []struct {
		name               string
		args               args
		multitenancyClient *Multitenancy
		expected           args
	}{
		{
			name: "test happy path",
			args: args{
				nwCfg: &cni.NetworkConfig{
					MultiTenancy:     true,
					EnableSnatOnHost: false,
				},
				cnsNetworkConfig: &cns.GetNetworkContainerResponse{
					IPConfiguration: cns.IPConfiguration{
						IPSubnet:         cns.IPSubnet{},
						DNSServers:       nil,
						GatewayIPAddress: "10.0.0.1",
					},
				},
				epInfo: &network.EndpointInfo{},
				result: &cniTypesCurr.Result{},
			},
			expected: args{
				nwCfg: &cni.NetworkConfig{
					MultiTenancy:     true,
					EnableSnatOnHost: false,
				},
				cnsNetworkConfig: &cns.GetNetworkContainerResponse{
					IPConfiguration: cns.IPConfiguration{
						IPSubnet:         cns.IPSubnet{},
						DNSServers:       nil,
						GatewayIPAddress: "10.0.0.1",
					},
				},
				epInfo: &network.EndpointInfo{
					Routes: []network.RouteInfo{
						{
							Dst: net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: defaultIPNet().Mask},
							Gw:  net.ParseIP("10.0.0.1"),
						},
					},
				},
				result: &cniTypesCurr.Result{
					Routes: []*cniTypes.Route{
						{
							Dst: net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: defaultIPNet().Mask},
							GW:  net.ParseIP("10.0.0.1"),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.multitenancyClient.SetupRoutingForMultitenancy(tt.args.nwCfg, tt.args.cnsNetworkConfig, tt.args.azIpamResult, tt.args.epInfo, tt.args.result)
			require.Exactly(tt.expected.nwCfg, tt.args.nwCfg)
			require.Exactly(tt.expected.cnsNetworkConfig, tt.args.cnsNetworkConfig)
			require.Exactly(tt.expected.azIpamResult, tt.args.azIpamResult)
			require.Exactly(tt.expected.epInfo, tt.args.epInfo)
			require.Exactly(tt.expected.result, tt.args.result)
		})
	}
}

func TestCleanupMultitenancyResources(t *testing.T) {
	require := require.New(t) //nolint:gocritic
	type args struct {
		enableInfraVnet bool
		nwCfg           *cni.NetworkConfig
		infraIPNet      *cniTypesCurr.Result
		plugin          *NetPlugin
	}
	tests := []struct {
		name               string
		args               args
		multitenancyClient *Multitenancy
		expected           args
	}{
		{
			name: "test happy path",
			args: args{
				enableInfraVnet: true,
				nwCfg: &cni.NetworkConfig{
					MultiTenancy: true,
				},
				infraIPNet: &cniTypesCurr.Result{},
				plugin: &NetPlugin{
					ipamInvoker: NewMockIpamInvoker(false, false, false),
				},
			},
			expected: args{
				nwCfg: &cni.NetworkConfig{
					MultiTenancy:     true,
					EnableSnatOnHost: false,
					IPAM:             cni.IPAM{},
				},
				infraIPNet: &cniTypesCurr.Result{},
				plugin: &NetPlugin{
					ipamInvoker: NewMockIpamInvoker(false, false, false),
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			require.Exactly(tt.expected.nwCfg, tt.args.nwCfg)
			require.Exactly(tt.expected.infraIPNet, tt.args.infraIPNet)
			require.Exactly(tt.expected.plugin, tt.args.plugin)
		})
	}
}
