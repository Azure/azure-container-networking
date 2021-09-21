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
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
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

type getNetworkConfigurationHandler struct {
	orchestratorContext []byte
	returnResponse      *cns.GetNetworkContainerResponse
	err                 error
}

type MockCNSClient struct {
	require                 *require.Assertions
	request                 requestIPAddressHandler
	release                 releaseIPAddressHandler
	getNetworkConfiguration getNetworkConfigurationHandler
}

func (c *MockCNSClient) RequestIPAddress(_ context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	c.require.Exactly(c.request.ipconfigArgument, ipconfig)
	return c.request.result, c.request.err
}

func (c *MockCNSClient) ReleaseIPAddress(_ context.Context, ipconfig cns.IPConfigRequest) error {
	c.require.Exactly(c.release.ipconfigArgument, ipconfig)
	return c.release.err
}

func (c *MockCNSClient) GetNetworkConfiguration(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error) {
	c.require.Exactly(c.getNetworkConfiguration.orchestratorContext, orchestratorContext)
	return c.getNetworkConfiguration.returnResponse, c.getNetworkConfiguration.err
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
					Ipam:             ipamStruct{},
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
			CleanupMultitenancyResources(tt.args.enableInfraVnet, tt.args.nwCfg, tt.args.infraIPNet, tt.args.plugin)
			require.Exactly(tt.expected.nwCfg, tt.args.nwCfg)
			require.Exactly(tt.expected.infraIPNet, tt.args.infraIPNet)
			require.Exactly(tt.expected.plugin, tt.args.plugin)
		})
	}
}

func TestGetMultiTenancyCNIResult(t *testing.T) {
	require := require.New(t) //nolint:gocritic
	type args struct {
		ctx             context.Context
		enableInfraVnet bool
		nwCfg           *cni.NetworkConfig
		plugin          *NetPlugin
		k8sPodName      string
		k8sNamespace    string
		ifName          string
	}
	tests := []struct {
		name    string
		args    args
		want    *cniTypesCurr.Result
		want1   *cns.GetNetworkContainerResponse
		want2   net.IPNet
		want3   *cniTypesCurr.Result
		wantErr bool
	}{
		{
			name: "test happy path",
			args: args{
				enableInfraVnet: true,
				nwCfg: &cni.NetworkConfig{
					MultiTenancy:               true,
					EnableSnatOnHost:           true,
					EnableExactMatchForPodName: true,
					InfraVnetAddressSpace:      "10.0.0.0/16",
					Ipam:                       ipamStruct{Type: "azure-vnet-ipam"},
				},
				plugin: &NetPlugin{
					ipamInvoker: NewMockIpamInvoker(false, false, false),
					multitenancyClient: &Multitenancy{
						netioshim: &mockNetIOShim{},
						cnsclient: &MockCNSClient{
							require: require,
							getNetworkConfiguration: getNetworkConfigurationHandler{
								orchestratorContext: marshallPodInfo(cns.KubernetesPodInfo{
									PodName:      "testpod",
									PodNamespace: "testnamespace",
								}),
								returnResponse: &cns.GetNetworkContainerResponse{
									PrimaryInterfaceIdentifier: "10.0.0.0/16",
									LocalIPConfiguration: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.0.5",
											PrefixLength: 16,
										},
										GatewayIPAddress: "",
									},
									CnetAddressSpace: []cns.IPSubnet{
										{
											IPAddress:    "10.1.0.0",
											PrefixLength: 16,
										},
									},
									IPConfiguration: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.1.0.5",
											PrefixLength: 16,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.1.0.1",
									},
									Routes: []cns.Route{
										{
											IPAddress:        "10.1.0.0/16",
											GatewayIPAddress: "10.1.0.1",
										},
									},
								},
							},
						},
					},
				},
				k8sPodName:   "testpod",
				k8sNamespace: "testnamespace",
				ifName:       "eth0",
			},

			want: &cniTypesCurr.Result{
				Interfaces: []*cniTypesCurr.Interface{
					{
						Name: "eth0",
					},
				},
				IPs: []*cniTypesCurr.IPConfig{
					{
						Version: "4",
						Address: getIPNet(net.IPv4(10, 1, 0, 5), net.CIDRMask(16, 32)),
						Gateway: net.ParseIP("10.1.0.1"),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: *getIPNetWithString("10.1.0.0/16"),
						GW:  net.ParseIP("10.1.0.1"),
					},
					{
						Dst: net.IPNet{IP: net.ParseIP("10.1.0.0"), Mask: net.CIDRMask(16, 32)},
						GW:  net.ParseIP("10.1.0.1"),
					},
				},
			},
			want1: &cns.GetNetworkContainerResponse{
				PrimaryInterfaceIdentifier: "10.0.0.0/16",
				LocalIPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.0.0.5",
						PrefixLength: 16,
					},
					GatewayIPAddress: "",
				},
				CnetAddressSpace: []cns.IPSubnet{
					{
						IPAddress:    "10.1.0.0",
						PrefixLength: 16,
					},
				},
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "10.1.0.5",
						PrefixLength: 16,
					},
					DNSServers:       nil,
					GatewayIPAddress: "10.1.0.1",
				},
				Routes: []cns.Route{
					{
						IPAddress:        "10.1.0.0/16",
						GatewayIPAddress: "10.1.0.1",
					},
				},
			},
			want2: *getCIDRNotationForAddress("10.0.0.0/16"),
			want3: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Version: "4",
						Address: net.IPNet{
							IP:   net.ParseIP("10.240.0.5"),
							Mask: net.CIDRMask(24, 32),
						},
						Gateway: net.ParseIP("10.240.0.1"),
					},
				},
				Routes: nil,
				DNS:    cniTypes.DNS{},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, got1, got2, got3, err := tt.args.plugin.GetMultiTenancyCNIResult(
				tt.args.ctx,
				tt.args.enableInfraVnet,
				tt.args.nwCfg,
				tt.args.k8sPodName,
				tt.args.k8sNamespace,
				tt.args.ifName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMultiTenancyCNIResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				require.Error(err)
			}
			require.NoError(err)
			require.Exactly(tt.want, got)
			require.Exactly(tt.want1, got1)
			require.Exactly(tt.want2, got2)
			require.Exactly(tt.want3, got3)
		})
	}
}
