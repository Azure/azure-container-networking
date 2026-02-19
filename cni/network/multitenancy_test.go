package network

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/google/go-cmp/cmp"
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

type requestIPsHandler struct {
	// arguments
	ipconfigArgument cns.IPConfigsRequest

	// results
	result *cns.IPConfigsResponse // this will return the IPConfigsResponse which contains a slice of IPs as opposed to one IP
	err    error
}

type releaseIPHandler struct {
	// arguments
	ipconfigArgument cns.IPConfigRequest

	// results
	err error
}

type releaseIPsHandler struct {
	// arguments
	ipconfigArgument cns.IPConfigsRequest // this will return the IPConfigsResponse which contains a slice of IPs as opposed to one IP

	// results
	err error
}

type getNetworkContainerConfigurationHandler struct {
	orchestratorContext []byte
	returnResponse      *cns.GetNetworkContainerResponse
	err                 error
}

// this is to get all the NCs for testing with given orchestratorContext
type getAllNetworkContainersConfigurationHandler struct {
	orchestratorContext []byte
	returnResponse      []cns.GetNetworkContainerResponse
	err                 error
}

type cnsAPIName string

const (
	GetAllNetworkContainers cnsAPIName = "GetAllNetworkContainers"
	RequestIPs              cnsAPIName = "RequestIPs"
	ReleaseIPs              cnsAPIName = "ReleaseIPs"
)

var (
	errUnsupportedAPI             = errors.New("Unsupported API")
	errNoRequestIPFound           = errors.New("No Request IP Found")
	errNoReleaseIPFound           = errors.New("No Release IP Found")
	errNoOrchestratorContextFound = errors.New("No CNI OrchestratorContext Found")
)

type MockCNSClient struct {
	unsupportedAPIs                      map[cnsAPIName]struct{}
	require                              *require.Assertions
	requestIP                            requestIPAddressHandler
	requestIPs                           requestIPsHandler
	releaseIP                            releaseIPHandler
	releaseIPs                           releaseIPsHandler
	getNetworkContainerConfiguration     getNetworkContainerConfigurationHandler
	getAllNetworkContainersConfiguration getAllNetworkContainersConfigurationHandler
}

func (c *MockCNSClient) RequestIPAddress(_ context.Context, ipconfig cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	if !cmp.Equal(c.requestIP.ipconfigArgument, ipconfig) {
		return nil, errNoRequestIPFound
	}
	return c.requestIP.result, c.requestIP.err
}

func (c *MockCNSClient) RequestIPs(_ context.Context, ipconfig cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
	if _, isUnsupported := c.unsupportedAPIs[RequestIPs]; isUnsupported {
		e := &client.CNSClientError{}
		e.Code = types.UnsupportedAPI
		e.Err = errUnsupportedAPI
		return nil, e
	}

	if !cmp.Equal(c.requestIPs.ipconfigArgument, ipconfig) {
		return nil, errNoRequestIPFound
	}
	return c.requestIPs.result, c.requestIPs.err
}

func (c *MockCNSClient) ReleaseIPAddress(_ context.Context, ipconfig cns.IPConfigRequest) error {
	if !cmp.Equal(c.releaseIP.ipconfigArgument, ipconfig) {
		return errNoReleaseIPFound
	}
	return c.releaseIP.err
}

func (c *MockCNSClient) ReleaseIPs(_ context.Context, ipconfig cns.IPConfigsRequest) error {
	if _, isUnsupported := c.unsupportedAPIs[ReleaseIPs]; isUnsupported {
		e := &client.CNSClientError{}
		e.Code = types.UnsupportedAPI
		e.Err = errUnsupportedAPI
		return e
	}

	if !cmp.Equal(c.releaseIPs.ipconfigArgument, ipconfig) {
		return errNoReleaseIPFound
	}
	return c.releaseIPs.err
}

func (c *MockCNSClient) GetNetworkContainer(ctx context.Context, orchestratorContext []byte) (*cns.GetNetworkContainerResponse, error) {
	if !cmp.Equal(c.getNetworkContainerConfiguration.orchestratorContext, orchestratorContext) {
		return nil, errNoOrchestratorContextFound
	}
	return c.getNetworkContainerConfiguration.returnResponse, c.getNetworkContainerConfiguration.err
}

func (c *MockCNSClient) GetAllNetworkContainers(ctx context.Context, orchestratorContext []byte) ([]cns.GetNetworkContainerResponse, error) {
	if _, isUnsupported := c.unsupportedAPIs[GetAllNetworkContainers]; isUnsupported {
		e := &client.CNSClientError{}
		e.Code = types.UnsupportedAPI
		e.Err = errUnsupportedAPI
		return nil, e
	}

	if !cmp.Equal(c.getAllNetworkContainersConfiguration.orchestratorContext, orchestratorContext) {
		return nil, errNoOrchestratorContextFound
	}
	return c.getAllNetworkContainersConfiguration.returnResponse, c.getAllNetworkContainersConfiguration.err
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
		result           *network.InterfaceInfo
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
				result: &network.InterfaceInfo{},
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
				result: &network.InterfaceInfo{
					Routes: []network.RouteInfo{
						{
							Dst: net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: defaultIPNet().Mask},
							Gw:  net.ParseIP("10.0.0.1"),
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
					ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
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
					ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
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

func TestGetMultiTenancyCNIResult(t *testing.T) {
	require := require.New(t) //nolint:gocritic

	var ncResponses []cns.GetNetworkContainerResponse
	ncResponseOne := cns.GetNetworkContainerResponse{
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
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: "1", // vlanID 1
			ID:        1,
		},
	}

	ncResponseTwo := cns.GetNetworkContainerResponse{
		PrimaryInterfaceIdentifier: "20.0.0.0/16",
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "20.0.0.5",
				PrefixLength: 16,
			},
			GatewayIPAddress: "",
		},
		CnetAddressSpace: []cns.IPSubnet{
			{
				IPAddress:    "20.1.0.0",
				PrefixLength: 16,
			},
		},
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "20.1.0.5",
				PrefixLength: 16,
			},
			DNSServers:       nil,
			GatewayIPAddress: "20.1.0.1",
		},
		Routes: []cns.Route{
			{
				IPAddress:        "20.1.0.0/16",
				GatewayIPAddress: "20.1.0.1",
			},
		},
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: "2", // vlanID 2
			ID:        2,
		},
	}
	ncResponses = append(ncResponses, ncResponseOne, ncResponseTwo)

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
		want2   *cns.GetNetworkContainerResponse
		want3   net.IPNet
		want4   *cniTypesCurr.Result
		want5   []cns.GetNetworkContainerResponse
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
					IPAM:                       cni.IPAM{Type: "azure-vnet-ipam"},
				},
				plugin: &NetPlugin{
					ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
					multitenancyClient: &Multitenancy{
						netioshim: &mockNetIOShim{},
						cnsclient: &MockCNSClient{
							require: require,
							getAllNetworkContainersConfiguration: getAllNetworkContainersConfigurationHandler{
								orchestratorContext: marshallPodInfo(cns.KubernetesPodInfo{
									PodName:      "testpod",
									PodNamespace: "testnamespace",
								}),
								returnResponse: ncResponses,
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
				MultiTenancyInfo: cns.MultiTenancyInfo{
					EncapType: "1",
					ID:        1,
				},
			},
			want2: &cns.GetNetworkContainerResponse{
				PrimaryInterfaceIdentifier: "20.0.0.0/16",
				LocalIPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "20.0.0.5",
						PrefixLength: 16,
					},
					GatewayIPAddress: "",
				},
				CnetAddressSpace: []cns.IPSubnet{
					{
						IPAddress:    "20.1.0.0",
						PrefixLength: 16,
					},
				},
				IPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "20.1.0.5",
						PrefixLength: 16,
					},
					DNSServers:       nil,
					GatewayIPAddress: "20.1.0.1",
				},
				Routes: []cns.Route{
					{
						IPAddress:        "20.1.0.0/16",
						GatewayIPAddress: "20.1.0.1",
					},
				},
				MultiTenancyInfo: cns.MultiTenancyInfo{
					EncapType: "2",
					ID:        2,
				},
			},
			want3: *getCIDRNotationForAddress("10.0.0.0/16"),
			want4: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
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
			got, err := tt.args.plugin.multitenancyClient.GetAllNetworkContainers(
				tt.args.ctx,
				tt.args.nwCfg,
				tt.args.k8sPodName,
				tt.args.k8sNamespace,
				tt.args.ifName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAllNetworkContainers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				require.Error(err)
			}
			require.NoError(err)
			require.Exactly(tt.want1, got.interfaceInfo[string(cns.InfraNIC)+"0"].NCResponse)
			require.Exactly(tt.want2, got.interfaceInfo[string(cns.InfraNIC)+"1"].NCResponse)
			require.Exactly(tt.want3, got.interfaceInfo[string(cns.InfraNIC)+"0"].HostSubnetPrefix)

			// check multiple responses
			tt.want5 = append(tt.want5, *tt.want1, *tt.want2)
			require.Exactly(tt.want5, ncResponses)

			require.Equal(cns.InfraNIC, got.interfaceInfo[string(cns.InfraNIC)+"0"].NICType)
			require.Equal(cns.InfraNIC, got.interfaceInfo[string(cns.InfraNIC)+"1"].NICType)
		})
	}
}

// TestGetMultiTenancyCNIResultUnsupportedAPI tests if new CNS API is not supported and old CNS API can handle to get ncConfig with "Unsupported API" error
func TestGetMultiTenancyCNIResultUnsupportedAPI(t *testing.T) {
	require := require.New(t) //nolint:gocritic

	ncResponse := cns.GetNetworkContainerResponse{
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
	}

	// set new CNS API is not supported
	unsupportedAPIs := make(map[cnsAPIName]struct{})
	unsupportedAPIs["GetAllNetworkContainers"] = struct{}{}

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
		want    *cns.GetNetworkContainerResponse
		wantErr bool
	}{
		{
			name: "test happy path for Unsupported API with old CNS API",
			args: args{
				enableInfraVnet: true,
				nwCfg: &cni.NetworkConfig{
					MultiTenancy:               true,
					EnableSnatOnHost:           true,
					EnableExactMatchForPodName: true,
					InfraVnetAddressSpace:      "10.0.0.0/16",
					IPAM:                       cni.IPAM{Type: "azure-vnet-ipam"},
				},
				plugin: &NetPlugin{
					ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
					multitenancyClient: &Multitenancy{
						netioshim: &mockNetIOShim{},
						cnsclient: &MockCNSClient{
							unsupportedAPIs: unsupportedAPIs,
							require:         require,
							getNetworkContainerConfiguration: getNetworkContainerConfigurationHandler{
								orchestratorContext: marshallPodInfo(cns.KubernetesPodInfo{
									PodName:      "testpod",
									PodNamespace: "testnamespace",
								}),
								returnResponse: &ncResponse,
							},
						},
					},
				},
				k8sPodName:   "testpod",
				k8sNamespace: "testnamespace",
				ifName:       "eth0",
			},
			want: &cns.GetNetworkContainerResponse{
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
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.plugin.multitenancyClient.GetAllNetworkContainers(
				tt.args.ctx,
				tt.args.nwCfg,
				tt.args.k8sPodName,
				tt.args.k8sNamespace,
				tt.args.ifName)
			if err != nil && tt.wantErr {
				t.Fatalf("expected an error %+v but none received", err)
			}
			require.NoError(err)
			require.Exactly(tt.want, got.interfaceInfo[string(cns.InfraNIC)+"0"].NCResponse)
			require.Equal(cns.InfraNIC, got.interfaceInfo[string(cns.InfraNIC)+"0"].NICType)
		})
	}
}

// TestGetMultiTenancyCNIResultNotFound test includes two sub test cases:
// 1. CNS supports new API and it does not have orchestratorContext info
// 2. CNS does not support new API and it does not have orchestratorContext info
func TestGetMultiTenancyCNIResultNotFound(t *testing.T) {
	require := require.New(t) //nolint:gocritic

	ncResponse := cns.GetNetworkContainerResponse{
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
	}

	// set new CNS API is not supported
	unsupportedAPIs := make(map[cnsAPIName]struct{})
	unsupportedAPIs["GetAllNetworkContainers"] = struct{}{}

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
		want    *cns.GetNetworkContainerResponse
		wantErr bool
	}{
		{
			name: "test happy path, CNS does not support new API without orchestratorContext found",
			args: args{
				enableInfraVnet: true,
				nwCfg: &cni.NetworkConfig{
					MultiTenancy:               true,
					EnableSnatOnHost:           true,
					EnableExactMatchForPodName: true,
					InfraVnetAddressSpace:      "10.0.0.0/16",
					IPAM:                       cni.IPAM{Type: "azure-vnet-ipam"},
				},
				plugin: &NetPlugin{
					ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
					multitenancyClient: &Multitenancy{
						netioshim: &mockNetIOShim{},
						cnsclient: &MockCNSClient{
							unsupportedAPIs: unsupportedAPIs,
							require:         require,
							getNetworkContainerConfiguration: getNetworkContainerConfigurationHandler{
								orchestratorContext: marshallPodInfo(cns.KubernetesPodInfo{
									PodName:      "testpod",
									PodNamespace: "testnamespace",
								}),
								returnResponse: &ncResponse,
							},
						},
					},
				},
				// use mismatched k8sPodName and k8sNamespace
				k8sPodName:   "testpod1",
				k8sNamespace: "testnamespace1",
				ifName:       "eth0",
			},
			wantErr: true,
		},
		{
			name: "test happy path, CNS does support new API without orchestratorContext found",
			args: args{
				enableInfraVnet: true,
				nwCfg: &cni.NetworkConfig{
					MultiTenancy:               true,
					EnableSnatOnHost:           true,
					EnableExactMatchForPodName: true,
					InfraVnetAddressSpace:      "10.0.0.0/16",
					IPAM:                       cni.IPAM{Type: "azure-vnet-ipam"},
				},
				plugin: &NetPlugin{
					ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
					multitenancyClient: &Multitenancy{
						netioshim: &mockNetIOShim{},
						cnsclient: &MockCNSClient{
							require: require,
							getNetworkContainerConfiguration: getNetworkContainerConfigurationHandler{
								orchestratorContext: marshallPodInfo(cns.KubernetesPodInfo{
									PodName:      "testpod",
									PodNamespace: "testnamespace",
								}),
								returnResponse: &ncResponse,
							},
						},
					},
				},
				// use mismatched k8sPodName and k8sNamespace
				k8sPodName:   "testpod1",
				k8sNamespace: "testnamespace1",
				ifName:       "eth0",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.args.plugin.multitenancyClient.GetAllNetworkContainers(
				tt.args.ctx,
				tt.args.nwCfg,
				tt.args.k8sPodName,
				tt.args.k8sNamespace,
				tt.args.ifName)
			if err == nil && tt.wantErr {
				t.Fatalf("expected an error %+v but none received", err)
			}

			if !errors.Is(err, errNoOrchestratorContextFound) {
				t.Fatalf("expected an error %s but %v received", errNoOrchestratorContextFound, err)
			}
		})
	}
}

// TestGetAllNetworkContainersWithIPv6Multitenancy verifies CNI multitenancy
// correctly processes IPv6Configuration
func TestGetAllNetworkContainersWithIPv6Multitenancy(t *testing.T) {
	require := require.New(t)

	// Create NC responses with IPv6Configuration
	ncResponseWithIPv6 := cns.GetNetworkContainerResponse{
		PrimaryInterfaceIdentifier: "10.0.0.0/16",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "10.1.0.5",
				PrefixLength: 16,
			},
			DNSServers:       []string{"8.8.8.8"},
			GatewayIPAddress: "10.1.0.1",
		},
		IPv6Configuration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "2001:db8::5",
				PrefixLength: 64,
			},
			DNSServers:       []string{"2001:4860:4860::8888"},
			GatewayIPAddress: "2001:db8::1",
		},
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: "1",
			ID:        1,
		},
	}

	ncResponses := []cns.GetNetworkContainerResponse{ncResponseWithIPv6}

	podInfo := cns.KubernetesPodInfo{
		PodName:      "test-pod",
		PodNamespace: "test-namespace",
	}
	orchestratorContext, err := json.Marshal(podInfo)
	require.NoError(err)

	// Mock CNS client
	cnsclient := &MockCNSClient{
		require: require,
		getAllNetworkContainersConfiguration: getAllNetworkContainersConfigurationHandler{
			orchestratorContext: orchestratorContext,
			returnResponse:      ncResponses,
			err:                 nil,
		},
	}

	// Mock network IO
	mockNetIO := &MockNetIOShim{
		interfaceSubnet: net.IPNet{
			IP:   net.IPv4(10, 0, 0, 0),
			Mask: net.CIDRMask(16, 32),
		},
	}

	multitenancy := &Multitenancy{}
	multitenancy.Init(cnsclient, mockNetIO)

	nwCfg := &cni.NetworkConfig{
		EnableExactMatchForPodName: true,
	}

	// Get network containers (this will process IPv6Configuration)
	ipamResult, err := multitenancy.GetAllNetworkContainers(
		context.TODO(),
		nwCfg,
		"test-pod",
		"test-namespace",
		"eth0",
	)

	require.NoError(err)
	require.NotNil(ipamResult)
	require.Len(ipamResult.interfaceInfo, 1)

	// Get the interface info
	var ifInfo network.InterfaceInfo
	for _, info := range ipamResult.interfaceInfo {
		ifInfo = info
		break
	}

	// Verify both IPv4 and IPv6 configurations are present
	require.Len(ifInfo.IPConfigs, 2, "Expected 2 IP configs (IPv4 and IPv6)")

	// Verify IPv6 configuration
	ipv6Config := ifInfo.IPConfigs[1]
	require.Equal("2001:db8::5", ipv6Config.Address.IP.String())
	require.Equal(64, getPrefixLength(ipv6Config.Address.Mask))
	require.Equal("2001:db8::1", ipv6Config.Gateway.String())
}

// TestGetAllNetworkContainersEmptyIPv6Multitenancy verifies CNI handles
// empty IPv6Configuration gracefully
func TestGetAllNetworkContainersEmptyIPv6Multitenancy(t *testing.T) {
	require := require.New(t)

	// Create NC response without IPv6Configuration (empty/zero value)
	ncResponseWithoutIPv6 := cns.GetNetworkContainerResponse{
		PrimaryInterfaceIdentifier: "10.0.0.0/16",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "10.1.0.5",
				PrefixLength: 16,
			},
			DNSServers:       []string{"8.8.8.8"},
			GatewayIPAddress: "10.1.0.1",
		},
		// IPv6Configuration omitted (zero value)
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: "1",
			ID:        1,
		},
	}

	ncResponses := []cns.GetNetworkContainerResponse{ncResponseWithoutIPv6}

	podInfo := cns.KubernetesPodInfo{
		PodName:      "test-pod",
		PodNamespace: "test-namespace",
	}
	orchestratorContext, err := json.Marshal(podInfo)
	require.NoError(err)

	cnsclient := &MockCNSClient{
		require: require,
		getAllNetworkContainersConfiguration: getAllNetworkContainersConfigurationHandler{
			orchestratorContext: orchestratorContext,
			returnResponse:      ncResponses,
			err:                 nil,
		},
	}

	mockNetIO := &MockNetIOShim{
		interfaceSubnet: net.IPNet{
			IP:   net.IPv4(10, 0, 0, 0),
			Mask: net.CIDRMask(16, 32),
		},
	}

	multitenancy := &Multitenancy{}
	multitenancy.Init(cnsclient, mockNetIO)

	nwCfg := &cni.NetworkConfig{
		EnableExactMatchForPodName: true,
	}

	ipamResult, err := multitenancy.GetAllNetworkContainers(
		context.TODO(),
		nwCfg,
		"test-pod",
		"test-namespace",
		"eth0",
	)

	require.NoError(err)
	require.NotNil(ipamResult)
	require.Len(ipamResult.interfaceInfo, 1)

	var ifInfo network.InterfaceInfo
	for _, info := range ipamResult.interfaceInfo {
		ifInfo = info
		break
	}

	// Verify only IPv4 configuration is present (no IPv6)
	require.Len(ifInfo.IPConfigs, 1, "Expected only 1 IP config (IPv4 only)")

	// Verify IPv4 configuration works correctly
	ipv4Config := ifInfo.IPConfigs[0]
	require.Equal("10.1.0.5", ipv4Config.Address.IP.String())
	require.Equal("10.1.0.1", ipv4Config.Gateway.String())
}

// Helper function to get prefix length from mask
func getPrefixLength(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
}

// MockNetIOShim for testing
type MockNetIOShim struct {
	interfaceSubnet net.IPNet
	callCount       int
}

func (m *MockNetIOShim) GetInterfaceSubnetWithSpecificIP(ipAddr string) *net.IPNet {
	m.callCount++
	return &m.interfaceSubnet
}
