//go:build linux
// +build linux

package network

import (
	"fmt"
	"net"
	"regexp"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetNetworkOptions(t *testing.T) {
	tests := []struct {
		name             string
		cnsNwConfig      cns.GetNetworkContainerResponse
		nwInfo           network.EndpointInfo
		expectedVlanID   string
		expectedSnatBrIP string
	}{
		{
			name: "set network options multitenancy",
			cnsNwConfig: cns.GetNetworkContainerResponse{
				MultiTenancyInfo: cns.MultiTenancyInfo{
					ID: 1,
				},
				LocalIPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "169.254.0.4",
						PrefixLength: 17,
					},
					GatewayIPAddress: "169.254.0.1",
				},
			},
			nwInfo: network.EndpointInfo{
				Options: make(map[string]interface{}),
			},
			expectedVlanID:   "1",
			expectedSnatBrIP: "169.254.0.1/17",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			setNetworkOptions(&tt.cnsNwConfig, &tt.nwInfo)
			require.Condition(t, assert.Comparison(func() bool {
				optMap := tt.nwInfo.Options[dockerNetworkOption]
				vlanID, ok := optMap.(map[string]interface{})[network.VlanIDKey]
				if !ok {
					return false
				}
				snatBridgeIP, ok := optMap.(map[string]interface{})[network.SnatBridgeIPKey]
				return ok && vlanID == tt.expectedVlanID && snatBridgeIP == tt.expectedSnatBrIP
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
			name: "set endpoint options multitenancy",
			cnsNwConfig: cns.GetNetworkContainerResponse{
				MultiTenancyInfo: cns.MultiTenancyInfo{
					ID: 1,
				},
				LocalIPConfiguration: cns.IPConfiguration{
					IPSubnet: cns.IPSubnet{
						IPAddress:    "169.254.0.4",
						PrefixLength: 17,
					},
					GatewayIPAddress: "169.254.0.1",
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
				vlanID := tt.epInfo.Data[network.VlanIDKey]
				localIP := tt.epInfo.Data[network.LocalIPKey]
				snatBrIP := tt.epInfo.Data[network.SnatBridgeIPKey]

				return tt.epInfo.AllowInboundFromHostToNC == true &&
					tt.epInfo.AllowInboundFromNCToHost == false &&
					tt.epInfo.NetworkContainerID == "abcd" &&
					vlanID == 1 &&
					localIP == "169.254.0.4/17" &&
					snatBrIP == "169.254.0.1/17"
			}))
		})
	}
}

func TestAddDefaultRoute(t *testing.T) {
	tests := []struct {
		name   string
		gwIP   string
		epInfo network.EndpointInfo
		result network.InterfaceInfo
	}{
		{
			name: "add default route multitenancy",
			gwIP: "192.168.0.1",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			addDefaultRoute(tt.gwIP, &tt.epInfo, &tt.result)
			require.Condition(t, assert.Comparison(func() bool {
				return len(tt.epInfo.Routes) == 1 &&
					len(tt.result.Routes) == 1 &&
					tt.epInfo.Routes[0].DevName == snatInterface &&
					tt.epInfo.Routes[0].Gw.String() == "192.168.0.1"
			}))
		})
	}
}

func TestAddSnatForDns(t *testing.T) {
	tests := []struct {
		name   string
		gwIP   string
		epInfo network.EndpointInfo
		result network.InterfaceInfo
	}{
		{
			name: "add snat for dns",
			gwIP: "192.168.0.1",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			addSnatForDNS(tt.gwIP, &tt.epInfo, &tt.result)
			require.Condition(t, assert.Comparison(func() bool {
				return len(tt.epInfo.Routes) == 1 &&
					len(tt.result.Routes) == 1 &&
					tt.epInfo.Routes[0].DevName == snatInterface &&
					tt.epInfo.Routes[0].Gw.String() == "192.168.0.1" &&
					tt.epInfo.Routes[0].Dst.String() == "168.63.129.16/32"
			}))
		})
	}
}

// linux swiftv2 example
func GetTestCNSResponseSecondaryLinux(macAddress string) map[string]network.InterfaceInfo {
	parsedMAC, _ := net.ParseMAC(macAddress)
	return map[string]network.InterfaceInfo{
		string(cns.InfraNIC): {
			IPConfigs: []*network.IPConfig{
				{
					Address: *getCIDRNotationForAddress("20.241.0.35/16"),
					Gateway: net.ParseIP("20.241.0.35"), // actual scenario doesn't have a gateway
				},
			},
			Routes: []network.RouteInfo{
				{
					Dst: *getCIDRNotationForAddress("169.254.2.1/16"),
					Gw:  net.ParseIP("10.244.2.1"),
				},
				{
					Dst: *getCIDRNotationForAddress("0.0.0.0/32"),
					Gw:  net.ParseIP("169.254.2.1"),
				},
			},
			NICType:           cns.InfraNIC,
			SkipDefaultRoutes: true,
			HostSubnetPrefix:  *getCIDRNotationForAddress("10.224.0.0/16"),
		},
		macAddress: {
			MacAddress: parsedMAC,
			IPConfigs: []*network.IPConfig{
				{
					Address: *getCIDRNotationForAddress("10.241.0.35/32"),
					Gateway: net.ParseIP("10.241.0.35"), // actual scenario doesn't have a gateway
				},
			},
			Routes: []network.RouteInfo{
				{
					Dst: *getCIDRNotationForAddress("169.254.2.1/32"),
					Gw:  net.ParseIP("10.244.2.1"),
				},
				{
					Dst: *getCIDRNotationForAddress("0.0.0.0/0"),
					Gw:  net.ParseIP("169.254.2.1"),
				},
			},
			NICType:           cns.NodeNetworkInterfaceFrontendNIC,
			SkipDefaultRoutes: false,
		},
	}
}

// Happy path scenario for add and delete
func TestPluginLinuxAdd(t *testing.T) {
	resources := GetTestResources()
	mulNwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "mulnet",
		MultiTenancy:               true,
		EnableExactMatchForPodName: true,
		Master:                     "eth0",
	}
	nwCfg := cni.NetworkConfig{
		CNIVersion:                 "0.3.0",
		Name:                       "net",
		MultiTenancy:               false,
		EnableExactMatchForPodName: true,
		// test auto finding master interface
		DNS: types.DNS{
			Nameservers: []string{
				"ns1", "ns2",
			},
			Domain: "myDomain",
		},
	}
	macAddress := "60:45:bd76:f6:44"
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
			// in swiftv1 linux multitenancy, we only get 1 response from cns at a time
			name: "Add Happy Path Swiftv1 Multitenancy",
			plugin: &NetPlugin{
				Plugin:             resources.Plugin,
				nm:                 network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				multitenancyClient: NewMockMultitenancy(false, []*cns.GetNetworkContainerResponse{GetTestCNSResponse3()}),
			},
			args: &cniSkel.CmdArgs{
				StdinData:   mulNwCfg.Serialize(),
				ContainerID: "test-container",
				Netns:       "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "test-pod", "test-pod-ns"),
				IfName:      eth0IfName,
			},
			match: func(ei1, ei2 *network.EndpointInfo) bool {
				return ei1.NetworkContainerID == ei2.NetworkContainerID
			},
			want: []endpointEntry{
				// should match with GetTestCNSResponse3
				{
					epInfo: &network.EndpointInfo{
						ContainerID: "test-container",
						Data: map[string]interface{}{
							"VlanID":       1, // Vlan ID used here
							"localIP":      "168.254.0.4/17",
							"snatBridgeIP": "168.254.0.1/17",
							"vethname":     "mulnettest-containereth0",
						},
						Routes: []network.RouteInfo{
							{
								Dst: *parseCIDR("192.168.0.4/24"),
								Gw:  net.ParseIP("192.168.0.1"),
								// interface to use is NOT propagated to ep info
							},
						},
						AllowInboundFromHostToNC: true,
						EnableSnatOnHost:         true,
						EnableMultiTenancy:       true,
						EnableSnatForDns:         true,
						PODName:                  "test-pod",
						PODNameSpace:             "test-pod-ns",
						NICType:                  cns.InfraNIC,
						MasterIfName:             eth0IfName,
						NetworkContainerID:       "Swift_74b34111-6e92-49ee-a82a-8881c850ce0e",
						NetworkID:                "mulnet",
						NetNsPath:                "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						NetNs:                    "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						HostSubnetPrefix:         "20.240.0.0/24",
						Options: map[string]interface{}{
							dockerNetworkOption: map[string]interface{}{
								"VlanID":       "1", // doesn't seem to be used in linux
								"snatBridgeIP": "168.254.0.1/17",
							},
						},
						// matches with cns ip configuration
						IPAddresses: []net.IPNet{
							{
								IP:   net.ParseIP("20.0.0.10"),
								Mask: getIPNetWithString("20.0.0.10/24").Mask,
							},
						},
						NATInfo: nil,
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
					},
					epIDRegex: `test-con-eth0`,
				},
			},
		},
		{
			// Based on a live swiftv2 linux cluster's cns invoker response
			name: "Add Happy Path Swiftv2",
			plugin: &NetPlugin{
				Plugin: resources.Plugin,
				nm:     network.NewMockNetworkmanager(network.NewMockEndpointClient(nil)),
				ipamInvoker: &MockIpamInvoker{
					add: func(opt IPAMAddConfig) (ipamAddResult IPAMAddResult, err error) {
						ipamAddResult = IPAMAddResult{interfaceInfo: make(map[string]network.InterfaceInfo)}
						ipamAddResult.interfaceInfo = GetTestCNSResponseSecondaryLinux(macAddress)
						opt.options["testflag"] = "copy"
						return ipamAddResult, nil
					},
					ipMap: make(map[string]bool),
				},
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
							getCIDRNotationForAddress("10.224.0.0/16"),
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
						Data: map[string]interface{}{
							"vethname": "nettest-containereth0",
						},
						Routes: []network.RouteInfo{
							{
								Dst: *getCIDRNotationForAddress("169.254.2.1/16"),
								Gw:  net.ParseIP("10.244.2.1"),
							},
							{
								Dst: *getCIDRNotationForAddress("0.0.0.0/32"),
								Gw:  net.ParseIP("169.254.2.1"),
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
						HostSubnetPrefix:  "10.224.0.0/16",
						EndpointDNS: network.DNSInfo{
							Servers: []string{
								"ns1", "ns2",
							},
							Suffix: "myDomain",
						},
						Options: map[string]interface{}{
							"testflag": "copy",
						},
						// matches with cns ip configuration
						IPAddresses: []net.IPNet{
							{
								IP:   net.ParseIP("20.241.0.35"),
								Mask: getIPNetWithString("20.241.0.35/16").Mask,
							},
						},
						NATInfo: nil,
						// ip config pod ip + mask(s) from cns > interface info > subnet info
						Subnets: []network.SubnetInfo{
							{
								Family: platform.AfINET,
								// matches cns ip configuration (20.241.0.0/16 == 20.241.0.35/16)
								Prefix: *getIPNetWithString("20.241.0.0/16"),
								// matches cns ip configuration gateway ip address
								Gateway: net.ParseIP("20.241.0.35"),
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
						Data: map[string]interface{}{
							"vethname": "nettest-containereth0",
						},
						Routes: []network.RouteInfo{
							{
								Dst: *getCIDRNotationForAddress("169.254.2.1/32"),
								Gw:  net.ParseIP("10.244.2.1"),
							},
							{
								Dst: *getCIDRNotationForAddress("0.0.0.0/0"),
								Gw:  net.ParseIP("169.254.2.1"),
							},
						},
						PODName:           "test-pod",
						PODNameSpace:      "test-pod-ns",
						NICType:           cns.NodeNetworkInterfaceFrontendNIC,
						SkipDefaultRoutes: false,
						MasterIfName:      "secondary",
						NetworkID:         "net",
						NetNsPath:         "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						NetNs:             "bc526fae-4ba0-4e80-bc90-ad721e5850bf",
						HostSubnetPrefix:  "<nil>",
						EndpointDNS: network.DNSInfo{
							Servers: []string{
								"ns1", "ns2",
							},
							Suffix: "myDomain",
						},
						Options: map[string]interface{}{
							"testflag": "copy",
						},
						// matches with cns ip configuration
						IPAddresses: []net.IPNet{
							{
								IP:   net.ParseIP("10.241.0.35"),
								Mask: getIPNetWithString("10.241.0.35/32").Mask,
							},
						},
						NATInfo: nil,
						// ip config pod ip + mask(s) from cns > interface info > subnet info
						Subnets: []network.SubnetInfo{
							{
								Family: platform.AfINET,
								Prefix: *getIPNetWithString("10.241.0.35/32"),
								// matches cns ip configuration gateway ip address
								Gateway: net.ParseIP("10.241.0.35"),
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

			// compare contents
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
				epInfo1 := epInfos[0]
				epInfo2 := epInfos[1]
				epInfo1.Data["dummy"] = "dummy value"
				epInfo1.Options["dummy"] = "another dummy value"
				require.NotEqual(t, epInfo1.Data, epInfo2.Data)
				require.NotEqual(t, epInfo1.Options, epInfo2.Options)
			}

			// ensure deleted
			require.Empty(t, allEndpoints)
		})
	}
}
