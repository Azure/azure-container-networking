package network

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/common"
	acnnetwork "github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/networkutils"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/nns"
	"github.com/Azure/azure-container-networking/telemetry"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	eth0IfName = "eth0"
)

var (
	args  *cniSkel.CmdArgs
	nwCfg cni.NetworkConfig
)

func TestMain(m *testing.M) {
	nwCfg = cni.NetworkConfig{
		Name:              "test-nwcfg",
		CNIVersion:        "0.3.0",
		Type:              "azure-vnet",
		Mode:              "bridge",
		Master:            eth0IfName,
		IPsToRouteViaHost: []string{"169.254.20.10"},
		IPAM: struct {
			Mode          string `json:"mode,omitempty"`
			Type          string `json:"type"`
			Environment   string `json:"environment,omitempty"`
			AddrSpace     string `json:"addressSpace,omitempty"`
			Subnet        string `json:"subnet,omitempty"`
			Address       string `json:"ipAddress,omitempty"`
			QueryInterval string `json:"queryInterval,omitempty"`
		}{
			Type: "azure-cns",
		},
	}

	args = &cniSkel.CmdArgs{
		ContainerID: "test-container",
		Netns:       "test-container",
	}
	args.StdinData = nwCfg.Serialize()
	podEnv := cni.K8SPodEnvArgs{
		K8S_POD_NAME:      "test-pod",
		K8S_POD_NAMESPACE: "test-pod-namespace",
	}
	args.Args = fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", podEnv.K8S_POD_NAME, podEnv.K8S_POD_NAMESPACE)
	args.IfName = eth0IfName

	// Run tests.
	exitCode := m.Run()
	os.Exit(exitCode)
}

func GetTestResources() *NetPlugin {
	pluginName := "testplugin"
	isIPv6 := false
	config := &common.PluginConfig{}
	grpcClient := &nns.MockGrpcClient{}
	plugin, _ := NewPlugin(pluginName, config, grpcClient, &Multitenancy{})
	plugin.report = &telemetry.CNIReport{}
	mockNetworkManager := acnnetwork.NewMockNetworkmanager(acnnetwork.NewMockEndpointClient(nil))
	plugin.nm = mockNetworkManager
	plugin.ipamInvoker = NewMockIpamInvoker(isIPv6, false, false, false, false)
	return plugin
}

/*
Multitenancy scenarios
*/
// For use with GetNetworkContainer
func GetTestCNSResponse0() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
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
}

// For use with GetAllNetworkContainers
func GetTestCNSResponse1() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
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
}

// For use with GetAllNetworkContainers in windows dualnic
func GetTestCNSResponse2() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
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
}

// For use with GetAllNetworkContainers in linux multitenancy
func GetTestCNSResponse3() *cns.GetNetworkContainerResponse {
	return &cns.GetNetworkContainerResponse{
		NetworkContainerID: "Swift_74b34111-6e92-49ee-a82a-8881c850ce0e",
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "20.0.0.10",
				PrefixLength: ipPrefixLen,
			},
			DNSServers: []string{
				"168.63.129.16",
			},
			GatewayIPAddress: "20.0.0.1",
		},
		Routes: []cns.Route{
			// dummy route
			{
				IPAddress:        "192.168.0.4/24",
				GatewayIPAddress: "192.168.0.1",
			},
		},
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        multiTenancyVlan1,
		},
		PrimaryInterfaceIdentifier: "20.240.0.4/24",
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "168.254.0.4",
				PrefixLength: localIPPrefixLen,
			},
			GatewayIPAddress: "168.254.0.1",
		},
		AllowHostToNCCommunication: true,
		AllowNCToHostCommunication: false,
	}
}

func TestPluginBaremetalDelete(t *testing.T) {
	plugin := GetTestResources()
	plugin.nnsClient = &nns.MockGrpcClient{}
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "baremetal-net",
		ExecutionMode:              string(util.Baremetal),
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	tests := []struct {
		name       string
		methods    []string
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "Baremetal delete success",
			methods: []string{CNI_ADD, CNI_DEL},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for _, method := range tt.methods {
				if method == CNI_ADD {
					err = plugin.Add(tt.args)
				} else if method == CNI_DEL {
					err = plugin.Delete(tt.args)
				}
			}

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := plugin.nm.GetAllEndpoints(localNwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 0 }))
			}
		})
	}
}

func TestGetOverlayNatInfo(t *testing.T) {
	nwCfg := &cni.NetworkConfig{ExecutionMode: string(util.V4Swift), IPAM: cni.IPAM{Mode: string(util.V4Overlay)}}
	natInfo := getNATInfo(nwCfg, nil, false)
	require.Empty(t, natInfo, "overlay natInfo should be empty")
}

func TestGetPodSubnetNatInfo(t *testing.T) {
	ncPrimaryIP := "10.241.0.4"
	nwCfg := &cni.NetworkConfig{ExecutionMode: string(util.V4Swift)}
	natInfo := getNATInfo(nwCfg, ncPrimaryIP, false)
	if runtime.GOOS == "windows" {
		require.Equalf(t, natInfo, []policy.NATInfo{
			{VirtualIP: ncPrimaryIP, Destinations: []string{networkutils.AzureDNS}},
			{Destinations: []string{networkutils.AzureIMDS}},
		}, "invalid windows podsubnet natInfo")
	} else {
		require.Empty(t, natInfo, "linux podsubnet natInfo should be empty")
	}
}

type InterfaceGetterMock struct {
	interfaces     []net.Interface
	interfaceAddrs map[string][]net.Addr // key is interfaceName, value is one interface's CIDRs(IPs+Masks)
	err            error
}

func (n *InterfaceGetterMock) GetNetworkInterfaces() ([]net.Interface, error) {
	if n.err != nil {
		return nil, n.err
	}
	return n.interfaces, nil
}

func (n *InterfaceGetterMock) GetNetworkInterfaceAddrs(iface *net.Interface) ([]net.Addr, error) {
	if n.err != nil {
		return nil, n.err
	}

	// actual net.Addr invokes syscall; here just create a mocked net.Addr{}
	netAddrs := []net.Addr{}
	for _, intf := range n.interfaces {
		if iface.Name == intf.Name {
			return n.interfaceAddrs[iface.Name], nil
		}
	}
	return netAddrs, nil
}
