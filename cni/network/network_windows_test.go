//go:build windows
// +build windows

package network

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/hnswrapper"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/platform"
	hnsv2 "github.com/Microsoft/hcsshim/hcn"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	network.Hnsv2 = hnswrapper.NewHnsv2wrapperFake()
	network.Hnsv1 = hnswrapper.NewHnsv1wrapperFake()
}

// Test windows network policies is set
func TestAddWithRunTimeNetPolicies(t *testing.T) {
	_, ipnetv4, _ := net.ParseCIDR("10.240.0.0/12")
	_, ipnetv6, _ := net.ParseCIDR("fc00::/64")

	tests := []struct {
		name       string
		nwInfo     network.EndpointInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "add ipv6 endpoint policy",
			nwInfo: network.EndpointInfo{
				Subnets: []network.SubnetInfo{
					{
						Gateway: net.ParseIP("10.240.0.1"),
						Prefix:  *ipnetv4,
					},
					{
						Gateway: net.ParseIP("fc00::1"),
						Prefix:  *ipnetv6,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p, err := getIPV6EndpointPolicy(tt.nwInfo.Subnets)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Condition(t, assert.Comparison(func() bool { return p.Type == policy.EndpointPolicy }))
			}
		})
	}
}

func TestSetNetworkOptions(t *testing.T) {
	tests := []struct {
		name           string
		cnsNwConfig    cns.GetNetworkContainerResponse
		nwInfo         network.EndpointInfo
		expectedVlanID string
	}{
		{
			name: "set network options vlanid test",
			cnsNwConfig: cns.GetNetworkContainerResponse{
				MultiTenancyInfo: cns.MultiTenancyInfo{
					ID: 1,
				},
			},
			nwInfo: network.EndpointInfo{
				Options: make(map[string]interface{}),
			},
			expectedVlanID: "1",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			setNetworkOptions(&tt.cnsNwConfig, &tt.nwInfo)
			require.Condition(t, assert.Comparison(func() bool {
				vlanMap := tt.nwInfo.Options[dockerNetworkOption]
				value, ok := vlanMap.(map[string]interface{})[network.VlanIDKey]
				return ok && value == tt.expectedVlanID
			}))
		})
	}
}

func TestSetEndpointOptions(t *testing.T) {
	tests := []struct {
		name        string
		cnsNwConfig cns.GetNetworkContainerResponse
		epInfo      network.EndpointInfo
		vethName    string
	}{
		{
			name: "set network options vlanid test",
			cnsNwConfig: cns.GetNetworkContainerResponse{
				MultiTenancyInfo: cns.MultiTenancyInfo{
					ID: 1,
				},
				CnetAddressSpace: []cns.IPSubnet{
					{
						IPAddress:    "192.168.0.4",
						PrefixLength: 24,
					},
				},
				AllowHostToNCCommunication: true,
				AllowNCToHostCommunication: false,
				NetworkContainerID:         "abcd",
			},
			epInfo: network.EndpointInfo{
				Data: make(map[string]interface{}),
			},
			vethName: "azv1",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			setEndpointOptions(&tt.cnsNwConfig, &tt.epInfo, tt.vethName)
			require.Condition(t, assert.Comparison(func() bool {
				return tt.epInfo.AllowInboundFromHostToNC == true &&
					tt.epInfo.AllowInboundFromNCToHost == false &&
					tt.epInfo.NetworkContainerID == "abcd"
			}))
		})
	}
}

func TestSetPoliciesFromNwCfg(t *testing.T) {
	tests := []struct {
		name          string
		nwCfg         cni.NetworkConfig
		isIPv6Enabled bool
		expected      []hnsv2.PortMappingPolicySetting
	}{
		{
			// ipv6 disabled, ipv4 host ip --> ipv4 host ip policy only
			name: "Runtime network polices",
			nwCfg: cni.NetworkConfig{
				RuntimeConfig: cni.RuntimeConfig{
					PortMappings: []cni.PortMapping{
						{
							Protocol:      "tcp",
							HostIp:        "192.168.0.4",
							HostPort:      8000,
							ContainerPort: 80,
						},
					},
				},
			},
			isIPv6Enabled: false,
			expected: []hnsv2.PortMappingPolicySetting{
				{
					ExternalPort: uint16(8000),
					InternalPort: uint16(80),
					VIP:          "192.168.0.4",
					Protocol:     policy.ProtocolTcp,
					Flags:        hnsv2.NatFlagsLocalRoutedVip,
				},
			},
		},
		{
			// ipv6 disabled, no host ip --> ipv4 policy only
			name: "Runtime hostPort mapping polices without hostIP",
			nwCfg: cni.NetworkConfig{
				RuntimeConfig: cni.RuntimeConfig{
					PortMappings: []cni.PortMapping{
						{
							Protocol:      "tcp",
							HostPort:      44000,
							ContainerPort: 80,
						},
					},
				},
			},
			isIPv6Enabled: false,
			expected: []hnsv2.PortMappingPolicySetting{
				{
					ExternalPort: uint16(44000),
					InternalPort: uint16(80),
					Protocol:     policy.ProtocolTcp,
					Flags:        hnsv2.NatFlagsLocalRoutedVip,
				},
			},
		},
		{
			// ipv6 enabled, ipv6 host ip --> ipv6 host ip policy only
			name: "Runtime hostPort mapping polices with ipv6 hostIP",
			nwCfg: cni.NetworkConfig{
				RuntimeConfig: cni.RuntimeConfig{
					PortMappings: []cni.PortMapping{
						{
							Protocol:      "tcp",
							HostPort:      44000,
							ContainerPort: 80,
							HostIp:        "2001:2002:2003::1",
						},
					},
				},
			},
			isIPv6Enabled: true,
			expected: []hnsv2.PortMappingPolicySetting{
				{
					ExternalPort: uint16(44000),
					InternalPort: uint16(80),
					VIP:          "2001:2002:2003::1",
					Protocol:     policy.ProtocolTcp,
					Flags:        hnsv2.NatFlagsIPv6,
				},
			},
		},
		{
			// ipv6 enabled, ipv4 host ip --> ipv4 host ip policy only
			name: "Runtime hostPort mapping polices with ipv4 hostIP on ipv6 enabled cluster",
			nwCfg: cni.NetworkConfig{
				RuntimeConfig: cni.RuntimeConfig{
					PortMappings: []cni.PortMapping{
						{
							Protocol:      "tcp",
							HostPort:      44000,
							ContainerPort: 80,
							HostIp:        "192.168.0.4",
						},
					},
				},
			},
			isIPv6Enabled: true,
			expected: []hnsv2.PortMappingPolicySetting{
				{
					ExternalPort: uint16(44000),
					InternalPort: uint16(80),
					VIP:          "192.168.0.4",
					Protocol:     policy.ProtocolTcp,
					Flags:        hnsv2.NatFlagsLocalRoutedVip,
				},
			},
		},
		{
			// ipv6 enabled, no host ip --> ipv4 and ipv6 policies
			name: "Runtime hostPort mapping polices with ipv6 without hostIP",
			nwCfg: cni.NetworkConfig{
				RuntimeConfig: cni.RuntimeConfig{
					PortMappings: []cni.PortMapping{
						{
							Protocol:      "tcp",
							HostPort:      44000,
							ContainerPort: 80,
						},
					},
				},
			},
			isIPv6Enabled: true,
			expected: []hnsv2.PortMappingPolicySetting{
				{
					ExternalPort: uint16(44000),
					InternalPort: uint16(80),
					VIP:          "",
					Protocol:     policy.ProtocolTcp,
					Flags:        hnsv2.NatFlagsLocalRoutedVip,
				},
				{
					ExternalPort: uint16(44000),
					InternalPort: uint16(80),
					VIP:          "",
					Protocol:     policy.ProtocolTcp,
					Flags:        hnsv2.NatFlagsIPv6,
				},
			},
		},
		{
			// ipv6 enabled, ipv6 localhost ip --> ipv6 host ip policy only
			name: "Runtime hostPort mapping polices with ipv6 localhost hostIP on ipv6 enabled cluster",
			nwCfg: cni.NetworkConfig{
				RuntimeConfig: cni.RuntimeConfig{
					PortMappings: []cni.PortMapping{
						{
							Protocol:      "tcp",
							HostPort:      44000,
							ContainerPort: 80,
							HostIp:        "::1",
						},
					},
				},
			},
			isIPv6Enabled: true,
			expected: []hnsv2.PortMappingPolicySetting{
				{
					ExternalPort: uint16(44000),
					InternalPort: uint16(80),
					VIP:          "::1",
					Protocol:     policy.ProtocolTcp,
					Flags:        hnsv2.NatFlagsIPv6,
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			policies, err := getPoliciesFromRuntimeCfg(&tt.nwCfg, tt.isIPv6Enabled)
			require.NoError(t, err)
			require.Condition(t, assert.Comparison(func() bool {
				return len(policies) > 0 && policies[0].Type == policy.EndpointPolicy
			}))
			require.Equal(t, len(tt.expected), len(policies), "expected number of policies not equal to actual")
			for index, policy := range policies {
				var hnsv2Policy hnsv2.EndpointPolicy
				err = json.Unmarshal(policy.Data, &hnsv2Policy)
				require.NoError(t, err, "failed to unmarshal hnsv2 policy")

				var rawPolicy hnsv2.PortMappingPolicySetting
				err = json.Unmarshal(hnsv2Policy.Settings, &rawPolicy)
				require.NoError(t, err, "failed to unmarshal hnsv2 port mapping policy")

				require.Equal(t, tt.expected[index], rawPolicy, "policies are not expected")
			}
		})
	}
}

func TestDSRPolciy(t *testing.T) {
	tests := []struct {
		name      string
		args      PolicyArgs
		wantCount int
	}{
		{
			name: "test enable dsr policy",
			args: PolicyArgs{
				nwCfg: &cni.NetworkConfig{
					WindowsSettings: cni.WindowsSettings{
						EnableLoopbackDSR: true,
					},
				},
				subnetInfos: []network.SubnetInfo{},
				ipconfigs: []*network.IPConfig{
					{
						Address: func() net.IPNet {
							_, ipnet, _ := net.ParseCIDR("10.0.0.5/24")
							return *ipnet
						}(),
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "test disable dsr policy",
			args: PolicyArgs{
				nwCfg:       &cni.NetworkConfig{},
				subnetInfos: []network.SubnetInfo{},
				ipconfigs: []*network.IPConfig{
					{
						Address: func() net.IPNet {
							_, ipnet, _ := net.ParseCIDR("10.0.0.5/24")
							return *ipnet
						}(),
					},
				},
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			policies, err := getEndpointPolicies(tt.args)
			require.NoError(t, err)
			require.Equal(t, tt.wantCount, len(policies))
		})
	}
}

func TestGetNetworkNameFromCNS(t *testing.T) {
	plugin, _ := cni.NewPlugin("name", "0.3.0")
	tests := []struct {
		name          string
		plugin        *NetPlugin
		netNs         string
		nwCfg         *cni.NetworkConfig
		interfaceInfo *network.InterfaceInfo
		want          string
		wantErr       bool
	}{
		{
			name: "Get Network Name from CNS with correct CIDR",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "net",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				Name:         "azure",
				MultiTenancy: true,
			},
			interfaceInfo: &network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: net.IPNet{
							IP:   net.ParseIP("10.240.0.5"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				NCResponse: &cns.GetNetworkContainerResponse{
					MultiTenancyInfo: cns.MultiTenancyInfo{
						ID: 1,
					},
				},
			},
			want:    "azure-vlan1-10-240-0-0_24",
			wantErr: false,
		},
		{
			name: "Get Network Name from CNS with malformed CIDR #1",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "net",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				Name:         "azure",
				MultiTenancy: true,
			},
			interfaceInfo: &network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: net.IPNet{
							IP:   net.ParseIP(""),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				NCResponse: &cns.GetNetworkContainerResponse{
					MultiTenancyInfo: cns.MultiTenancyInfo{
						ID: 1,
					},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Get Network Name from CNS with malformed CIDR #2",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "net",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				Name:         "azure",
				MultiTenancy: true,
			},
			interfaceInfo: &network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: net.IPNet{
							IP:   net.ParseIP("10.0.00.6"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				NCResponse: &cns.GetNetworkContainerResponse{
					MultiTenancyInfo: cns.MultiTenancyInfo{
						ID: 1,
					},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Get Network Name from CNS without NetNS",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				Name:         "azure",
				MultiTenancy: true,
			},
			interfaceInfo: &network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: net.IPNet{
							IP:   net.ParseIP("10.0.0.6"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				NCResponse: &cns.GetNetworkContainerResponse{
					MultiTenancyInfo: cns.MultiTenancyInfo{
						ID: 1,
					},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Get Network Name from CNS without multitenancy",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "azure",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				Name:         "azure",
				MultiTenancy: false,
			},
			interfaceInfo: &network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: net.IPNet{
							IP:   net.ParseIP("10.0.0.6"),
							Mask: net.CIDRMask(24, 32),
						},
					},
				},
				NCResponse: &cns.GetNetworkContainerResponse{},
			},
			want:    "azure",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			networkName, err := tt.plugin.getNetworkName(tt.netNs, tt.interfaceInfo, tt.nwCfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, networkName)
			}
		})
	}
}

func TestGetNetworkNameSwiftv2FromCNS(t *testing.T) {
	plugin, _ := cni.NewPlugin("name", "0.3.0")

	macAddress := "00:00:5e:00:53:01"
	swiftv2NetworkNamePrefix := "azure-"
	parsedMacAddress, _ := net.ParseMAC(macAddress)

	tests := []struct {
		name          string
		plugin        *NetPlugin
		netNs         string
		nwCfg         *cni.NetworkConfig
		interfaceInfo *network.InterfaceInfo
		want          string
		wantErr       bool
	}{
		{
			name: "Get Network Name from CNS for swiftv2 DelegatedNIC",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, true, false),
			},
			netNs: "azure",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				MultiTenancy: false,
			},
			interfaceInfo: &network.InterfaceInfo{
				Name:       "swiftv2L1VHDelegatedInterface",
				MacAddress: parsedMacAddress,
				NICType:    cns.NodeNetworkInterfaceFrontendNIC,
			},
			want:    swiftv2NetworkNamePrefix + parsedMacAddress.String(),
			wantErr: false,
		},
		{
			name: "Get Network Name from CNS for swiftv2 BackendNIC",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, true, false),
			},
			netNs: "azure",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				MultiTenancy: false,
			},
			interfaceInfo: &network.InterfaceInfo{
				Name:       "swiftv2L1VHIBinterface",
				MacAddress: parsedMacAddress,
				NICType:    cns.BackendNIC,
			},
			want:    swiftv2NetworkNamePrefix + parsedMacAddress.String(),
			wantErr: false,
		},
		{
			name: "Unhappy path: Get Network Name from CNS for swiftv2 AccelnetNIC with empty interfaceInfo",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "azure",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				MultiTenancy: false,
			},
			interfaceInfo: &network.InterfaceInfo{}, // return empty network name with empty interfaceInfo
			want:          "",
			wantErr:       false,
		},
		{
			name: "Unhappy path: Get Network Name from CNS for swiftv2 AccelnetNIC with invalid nicType",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "azure",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				MultiTenancy: false,
			},
			interfaceInfo: &network.InterfaceInfo{
				Name:       "swiftv2L1VHAccelnetInterface",
				MacAddress: parsedMacAddress,
				NICType:    "invalidNICType",
			}, // return empty network name with invalid nic type
			want:    "",
			wantErr: false,
		},
		{
			name: "Get Network Name from CNS for swiftv2 ApipaNIC",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "azure",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				MultiTenancy: false,
			},
			interfaceInfo: &network.InterfaceInfo{
				Name:       "apipa-interface",
				MacAddress: parsedMacAddress,
				NICType:    cns.ApipaNIC,
			},
			want:    swiftv2NetworkNamePrefix + "apipa", // "azure-apipa"
			wantErr: false,
		},
		{
			name: "Get Network Name from CNS for swiftv2 ApipaNIC with empty MacAddress",
			plugin: &NetPlugin{
				Plugin:      plugin,
				nm:          network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: NewMockIpamInvoker(false, false, false, false, false),
			},
			netNs: "azure",
			nwCfg: &cni.NetworkConfig{
				CNIVersion:   "0.3.0",
				MultiTenancy: false,
			},
			interfaceInfo: &network.InterfaceInfo{
				Name:    "apipa-test-interface",
				NICType: cns.ApipaNIC,
			},
			want:    swiftv2NetworkNamePrefix + "apipa", // "azure-apipa"
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.interfaceInfo)
			// compare networkNamess
			networkName, err := tt.plugin.getNetworkName(tt.netNs, tt.interfaceInfo, tt.nwCfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.Equal(t, tt.want, networkName)
			}

			// compare networkIDs
			networkID, err := tt.plugin.getNetworkID(tt.netNs, tt.interfaceInfo, tt.nwCfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.Equal(t, tt.want, networkID)
			}
		})
	}
}

// Test Multitenancy Windows Add (Dualnic)
func TestPluginMultitenancyWindowsAdd(t *testing.T) {
	plugin, _ := cni.NewPlugin("test", "0.3.0")

	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "mulnet",
		MultiTenancy:               true,
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	tests := []struct {
		name       string
		plugin     *NetPlugin
		args       *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Add Happy path",
			plugin: &NetPlugin{
				Plugin:             plugin,
				nm:                 network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				multitenancyClient: NewMockMultitenancy(false, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1(), GetTestCNSResponse2()}),
			},

			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr: false,
		},
		{
			name: "Add Fail",
			plugin: &NetPlugin{
				Plugin:             plugin,
				nm:                 network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				multitenancyClient: NewMockMultitenancy(true, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1(), GetTestCNSResponse2()}),
			},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "test-container",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			wantErr:    true,
			wantErrMsg: errMockMulAdd.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.Add(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg, "Expected %v but got %+v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
				endpoints, _ := tt.plugin.nm.GetAllEndpoints(localNwCfg.Name)
				// an extra cns response is added in windows multitenancy to test dualnic
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 2 }))
			}
		})
	}
}

func TestPluginMultitenancyWindowsDelete(t *testing.T) {
	plugin := GetTestResources()
	plugin.multitenancyClient = NewMockMultitenancy(false, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1(), GetTestCNSResponse2()})
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "mulnet",
		MultiTenancy:               true,
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}

	happyArgs := &cniSkel.CmdArgs{
		StdinData:   localNwCfg.Serialize(),
		ContainerID: "test-container",
		Netns:       "test-container",
		Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
		IfName:      eth0IfName,
	}

	tests := []struct {
		name       string
		methods    []string
		args       *cniSkel.CmdArgs
		delArgs    *cniSkel.CmdArgs
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "Multitenancy delete success",
			methods: []string{CNI_ADD, CNI_DEL},
			args:    happyArgs,
			delArgs: happyArgs,
			wantErr: false,
		},
		{
			name:    "Multitenancy delete net not found",
			methods: []string{CNI_ADD, CNI_DEL},
			args:    happyArgs,
			delArgs: &cniSkel.CmdArgs{
				StdinData: (&cni.NetworkConfig{
					CNIVersion:                 "0.3.0",
					Name:                       "othernet",
					MultiTenancy:               true,
					EnableExactMatchForPodName: true,
					Master:                     "eth0",
				}).Serialize(),
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
					err = plugin.Delete(tt.delArgs)
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

// windows swiftv2 example
func GetTestCNSResponseSecondaryWindows(macAddress string) map[string]network.InterfaceInfo {
	parsedMAC, _ := net.ParseMAC(macAddress)
	return map[string]network.InterfaceInfo{
		string(cns.InfraNIC): {
			IPConfigs: []*network.IPConfig{
				{
					Address: *getCIDRNotationForAddress("10.244.2.107/16"),
					Gateway: net.ParseIP("10.244.2.1"),
				},
			},
			Routes: []network.RouteInfo{
				{
					Dst: *getCIDRNotationForAddress("1.1.1.1/24"),
					Gw:  net.ParseIP("10.244.2.1"),
				},
			},
			SkipDefaultRoutes: true,
			NICType:           cns.InfraNIC,
			HostSubnetPrefix:  *getCIDRNotationForAddress("20.224.0.0/16"),
			EndpointPolicies: []policy.Policy{
				{
					Type: policy.EndpointPolicy,
					Data: GetRawACLPolicy(),
				},
			},
		},
		macAddress: {
			MacAddress: parsedMAC,
			IPConfigs: []*network.IPConfig{
				{
					Address: *getCIDRNotationForAddress("10.241.0.21/16"),
					Gateway: net.ParseIP("10.241.0.1"),
				},
			},
			Routes: []network.RouteInfo{
				{
					// just to ensure we don't overwrite if we had more routes
					Dst: *getCIDRNotationForAddress("2.2.2.2/24"),
					Gw:  net.ParseIP("99.244.2.1"),
				},
			},
			NICType: cns.NodeNetworkInterfaceFrontendNIC,
			EndpointPolicies: []policy.Policy{
				{
					Type: policy.EndpointPolicy,
					Data: GetRawOutBoundNATPolicy(),
				},
			},
		},
	}
}

func GetRawACLPolicy() (ret json.RawMessage) {
	var data map[string]interface{}
	formatted := []byte(`{
		"Type": "ACL",
		"Protocols": "6",
		"Action": "Block",
		"Direction": "Out",
		"RemoteAddresses": "168.63.129.16/32",
		"RemotePorts": "80",
		"Priority": 200,
		"RuleType": "Switch"
	  }`)
	json.Unmarshal(formatted, &data)  // nolint
	minified, _ := json.Marshal(data) // nolint
	ret = json.RawMessage(minified)
	return ret
}

func GetRawOutBoundNATPolicy() (ret json.RawMessage) {
	var data map[string]interface{}
	formatted := []byte(`{
		"Type": "OutBoundNAT",
		"ExceptionList": [
		  "10.224.0.0/16"
		]
	  }`)
	json.Unmarshal(formatted, &data)  // nolint
	minified, _ := json.Marshal(data) // nolint
	ret = json.RawMessage(minified)
	return ret
}

// Happy path scenario for add and delete
func TestPluginWindowsAdd(t *testing.T) {
	resources := GetTestResources()
	localNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "mulnet",
		MultiTenancy:               true,
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
		// these are added to test that policies propagate to endpoint info
		AdditionalArgs: []cni.KVPair{
			{
				Name:  "EndpointPolicy",
				Value: GetRawOutBoundNATPolicy(),
			},
			{
				Name:  "EndpointPolicy",
				Value: GetRawACLPolicy(),
			},
		},
		WindowsSettings: cni.WindowsSettings{ // included to test functionality
			EnableLoopbackDSR: true,
		},
	}
	nwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "net",
		MultiTenancy:               false,
		EnableExactMatchForPodName: true,
	}
	macAddress := "60:45:bd:76:f6:44"
	parsedMACAddress, _ := net.ParseMAC(macAddress)

	type endpointEntry struct {
		epInfo    *network.EndpointInfo
		epIDRegex string
	}

	tests := []struct {
		name   string
		plugin *NetPlugin
		args   *cniSkel.CmdArgs
		want   []endpointEntry
		match  func(*network.EndpointInfo, *network.EndpointInfo) bool
	}{
		{
			name: "Add Happy Path Dual NIC",
			plugin: &NetPlugin{
				Plugin:             resources.Plugin,
				nm:                 network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				multitenancyClient: NewMockMultitenancy(false, []*cns.GetNetworkContainerResponse{GetTestCNSResponse1(), GetTestCNSResponse2()}),
			},
			args: &cniSkel.CmdArgs{
				StdinData:   localNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			match: func(ei1, ei2 *network.EndpointInfo) bool {
				return ei1.NetworkID == ei2.NetworkID
			},
			want: []endpointEntry{
				// should match with GetTestCNSResponse1
				{
					epInfo: &network.EndpointInfo{
						ContainerID: "test-container",
						Data: map[string]interface{}{
							"cnetAddressSpace": []string(nil),
						},
						Routes:             []network.RouteInfo{},
						EnableSnatOnHost:   true,
						EnableMultiTenancy: true,
						EnableSnatForDns:   true,
						PODName:            "test-pod",
						PODNameSpace:       "test-pod-ns",
						NICType:            cns.InfraNIC,
						MasterIfName:       eth0IfName,
						NetworkID:          "mulnet-vlan1-20-0-0-0_24",
						NetNsPath:          "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						NetNs:              "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						HostSubnetPrefix:   "20.240.0.0/24",
						Options: map[string]interface{}{
							dockerNetworkOption: map[string]interface{}{
								"VlanID": "1",
							},
						},
						// matches with cns ip configuration
						IPAddresses: []net.IPNet{
							{
								IP:   net.ParseIP("20.0.0.10"),
								Mask: getIPNetWithString("20.0.0.10/24").Mask,
							},
						},
						// LocalIPConfiguration doesn't seem used in windows
						// Constant, in windows, NAT Info comes from
						// options > ipamAddConfig >
						// cns invoker may populate network.SNATIPKey with the default response received >
						// getNATInfo (with nwCfg) > adds nat info based on condition
						// typically adds azure dns (168.63.129.16)
						NATInfo: []policy.NATInfo{
							{
								Destinations: []string{"168.63.129.16"},
							},
						},
						// ip config pod ip + mask(s) from cns > interface info > subnet info
						Subnets: []network.SubnetInfo{
							{
								Family: platform.AfINET,
								// matches cns ip configuration (20.0.0.1/24 == 20.0.0.0/24)
								Prefix: *getIPNetWithString("20.0.0.0/24"),
								// matches cns ip configuration gateway ip address
								Gateway: net.ParseIP("20.0.0.1"),
							},
						},
						EndpointPolicies: []policy.Policy{
							{
								Type: policy.EndpointPolicy,
								Data: GetRawOutBoundNATPolicy(),
							},
							{
								Type: policy.EndpointPolicy,
								Data: GetRawACLPolicy(),
							},
							{
								Type: policy.EndpointPolicy,
								// if enabled we create a loopback dsr policy based on the cns ip config
								Data: json.RawMessage(`{"Type":"LoopbackDSR","IPAddress":"20.0.0.10"}`),
							},
						},
						NetworkPolicies: []policy.Policy{
							{
								Type: policy.EndpointPolicy,
								Data: GetRawOutBoundNATPolicy(),
							},
							{
								Type: policy.EndpointPolicy,
								Data: GetRawACLPolicy(),
							},
						},
					},
					epIDRegex: `.*`,
				},
				// should match with GetTestCNSResponse2
				{
					epInfo: &network.EndpointInfo{
						ContainerID: "test-container",
						Data: map[string]interface{}{
							"cnetAddressSpace": []string(nil),
						},
						Routes:             []network.RouteInfo{},
						EnableSnatOnHost:   true,
						EnableMultiTenancy: true,
						EnableSnatForDns:   true,
						PODName:            "test-pod",
						PODNameSpace:       "test-pod-ns",
						NICType:            cns.InfraNIC,
						MasterIfName:       eth0IfName,
						NetworkID:          "mulnet-vlan2-10-0-0-0_24",
						NetNsPath:          "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						NetNs:              "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						HostSubnetPrefix:   "10.240.0.0/24",
						Options: map[string]interface{}{
							dockerNetworkOption: map[string]interface{}{
								"VlanID": "2",
							},
						},
						IPAddresses: []net.IPNet{
							{
								IP:   net.ParseIP("10.0.0.10"),
								Mask: getIPNetWithString("10.0.0.10/24").Mask,
							},
						},
						NATInfo: []policy.NATInfo{
							{
								Destinations: []string{"168.63.129.16"},
							},
						},
						Subnets: []network.SubnetInfo{
							{
								Family:  platform.AfINET,
								Prefix:  *getIPNetWithString("10.0.0.0/24"),
								Gateway: net.ParseIP("10.0.0.1"),
							},
						},
						EndpointPolicies: []policy.Policy{
							{
								Type: policy.EndpointPolicy,
								Data: GetRawOutBoundNATPolicy(),
							},
							{
								Type: policy.EndpointPolicy,
								Data: GetRawACLPolicy(),
							},
							{
								Type: policy.EndpointPolicy,
								Data: json.RawMessage(`{"Type":"LoopbackDSR","IPAddress":"10.0.0.10"}`),
							},
						},
						NetworkPolicies: []policy.Policy{
							{
								Type: policy.EndpointPolicy,
								Data: GetRawOutBoundNATPolicy(),
							},
							{
								Type: policy.EndpointPolicy,
								Data: GetRawACLPolicy(),
							},
						},
					},
					epIDRegex: `.*`,
				},
			},
		},
		{
			// Based on a live swiftv2 windows cluster's (infra + delegated) cns invoker response
			name: "Add Happy Path Swiftv2",
			plugin: &NetPlugin{
				Plugin: resources.Plugin,
				nm:     network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),

				ipamInvoker: NewCustomMockIpamInvoker(GetTestCNSResponseSecondaryWindows(macAddress)),
				netClient: &InterfaceGetterMock{
					// used in secondary find master interface
					interfaces: []net.Interface{
						{
							Name:         "secondary",
							HardwareAddr: parsedMACAddress,
						},
						{
							Name:         "primary",
							HardwareAddr: net.HardwareAddr{},
						},
					},
					// used in primary find master interface
					interfaceAddrs: map[string][]net.Addr{
						"primary": {
							// match with the host subnet prefix to know that this ip belongs to the host
							getCIDRNotationForAddress("20.224.0.0/16"),
						},
					},
				},
			},
			args: &cniSkel.CmdArgs{
				StdinData:   nwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			match: func(ei1, ei2 *network.EndpointInfo) bool {
				return ei1.NICType == ei2.NICType
			},
			want: []endpointEntry{
				// should match infra
				{
					epInfo: &network.EndpointInfo{
						ContainerID: "test-container",
						Data:        map[string]interface{}{},
						Routes: []network.RouteInfo{
							{
								Dst: *getCIDRNotationForAddress("1.1.1.1/24"),
								Gw:  net.ParseIP("10.244.2.1"),
							},
						},
						PODName:           "test-pod",
						PODNameSpace:      "test-pod-ns",
						NICType:           cns.InfraNIC,
						SkipDefaultRoutes: true,
						MasterIfName:      "primary",
						NetworkID:         "net",
						NetNsPath:         "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						NetNs:             "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						HostSubnetPrefix:  "20.224.0.0/16",
						Options:           map[string]interface{}{},
						// matches with cns ip configuration
						IPAddresses: []net.IPNet{
							{
								IP:   net.ParseIP("10.244.2.107"),
								Mask: getIPNetWithString("10.244.2.107/16").Mask,
							},
						},
						NATInfo: nil,
						// ip config pod ip + mask(s) from cns > interface info > subnet info
						Subnets: []network.SubnetInfo{
							{
								Family: platform.AfINET,
								Prefix: *getIPNetWithString("10.244.0.0/16"),
								// matches cns ip configuration gateway ip address
								Gateway: net.ParseIP("10.244.2.1"),
							},
						},
						EndpointPolicies: []policy.Policy{
							{
								Type: policy.EndpointPolicy,
								Data: GetRawACLPolicy(),
							},
						},
					},
					epIDRegex: `.*`,
				},
				// should match secondary
				{
					epInfo: &network.EndpointInfo{
						MacAddress:  parsedMACAddress,
						ContainerID: "test-container",
						Data:        map[string]interface{}{},
						Routes: []network.RouteInfo{
							{
								// just to ensure we don't overwrite if we had more routes
								Dst: *getCIDRNotationForAddress("2.2.2.2/24"),
								Gw:  net.ParseIP("99.244.2.1"),
							},
						},
						PODName:           "test-pod",
						PODNameSpace:      "test-pod-ns",
						NICType:           cns.NodeNetworkInterfaceFrontendNIC,
						SkipDefaultRoutes: false,
						MasterIfName:      "secondary",
						NetworkID:         "azure-" + macAddress,
						NetNsPath:         "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						NetNs:             "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						HostSubnetPrefix:  "<nil>",
						Options:           map[string]interface{}{},
						// matches with cns ip configuration
						IPAddresses: []net.IPNet{
							{
								IP:   net.ParseIP("10.241.0.21"),
								Mask: getIPNetWithString("10.241.0.21/16").Mask,
							},
						},
						NATInfo: nil,
						// ip config pod ip + mask(s) from cns > interface info > subnet info
						Subnets: []network.SubnetInfo{
							{
								Family: platform.AfINET,
								Prefix: *getIPNetWithString("10.241.0.21/16"),
								// matches cns ip configuration gateway ip address
								Gateway: net.ParseIP("10.241.0.1"),
							},
						},
						EndpointPolicies: []policy.Policy{
							{
								Type: policy.EndpointPolicy,
								Data: GetRawOutBoundNATPolicy(),
							},
						},
					},
					epIDRegex: `.*`,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.plugin.Add(tt.args)
			require.NoError(t, err)
			allEndpoints, _ := tt.plugin.nm.GetAllEndpoints("")
			require.Len(t, allEndpoints, len(tt.want))
			for _, wantedEndpointEntry := range tt.want {
				epID := "none"
				for _, endpointInfo := range allEndpoints {
					if !tt.match(wantedEndpointEntry.epInfo, endpointInfo) {
						continue
					}
					// save the endpoint id before removing it
					epID = endpointInfo.EndpointID
					require.Regexp(t, regexp.MustCompile(wantedEndpointEntry.epIDRegex), epID)

					// omit endpoint id and ifname fields as they are nondeterministic
					endpointInfo.EndpointID = ""
					endpointInfo.IfName = ""

					require.Equal(t, wantedEndpointEntry.epInfo, endpointInfo)
				}
				if epID == "none" {
					t.Fail()
				}
				err = tt.plugin.nm.DeleteEndpoint("", epID, nil, "")
				require.NoError(t, err)
			}

			// confirm separate entities
			// that is, if one is modified, the other should not be modified
			epInfos := []*network.EndpointInfo{}
			for _, val := range allEndpoints {
				epInfos = append(epInfos, val)
			}
			if len(epInfos) > 1 {
				// ensure the endpoint data  and options are separate entities when in separate endpoint infos
				epInfo1 := epInfos[0]
				epInfo2 := epInfos[1]
				epInfo1.Data["dummy"] = "dummy value"
				epInfo1.Options["dummy"] = "another dummy value"
				require.NotEqual(t, epInfo1.Data, epInfo2.Data)
				require.NotEqual(t, epInfo1.Options, epInfo2.Options)

				// ensure the endpoint policy slices are separate entities when in separate endpoint infos
				if len(epInfo1.EndpointPolicies) > 0 {
					epInfo1.EndpointPolicies[0] = policy.Policy{
						Type: policy.ACLPolicy,
					}
					require.Len(t, epInfo1.EndpointPolicies, 1)
					require.Len(t, epInfo2.EndpointPolicies, 1)
					require.NotEqual(t, epInfo1.EndpointPolicies, epInfo2.EndpointPolicies)
				}
				// ensure the network policy slices are separate entities when in separate endpoint infos
				if len(epInfo1.NetworkPolicies) > 0 {
					epInfo1.NetworkPolicies[0] = policy.Policy{
						Type: policy.ACLPolicy,
					}
					require.NotEqual(t, epInfo1.NetworkPolicies, epInfo2.NetworkPolicies)
				}
			}

			// ensure deleted
			require.Empty(t, allEndpoints)
		})
	}
}
