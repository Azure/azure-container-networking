package network

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/util"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/require"
)

var testPodInfo cns.KubernetesPodInfo

func getTestIPConfigRequest() cns.IPConfigRequest {
	return cns.IPConfigRequest{
		PodInterfaceID:      "testcont-testifname",
		InfraContainerID:    "testcontainerid",
		OrchestratorContext: marshallPodInfo(testPodInfo),
	}
}

func getTestIPConfigsRequest() cns.IPConfigsRequest {
	return cns.IPConfigsRequest{
		PodInterfaceID:      "testcont-testifname",
		InfraContainerID:    "testcontainerid",
		OrchestratorContext: marshallPodInfo(testPodInfo),
	}
}

func getTestOverlayGateway() net.IP {
	if runtime.GOOS == "windows" {
		return net.ParseIP("10.240.0.1")
	}

	return net.ParseIP("169.254.1.1")
}

func TestCNSIPAMInvoker_Add_Overlay(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t

	// set new CNS API is not supported
	unsupportedAPIs := make(map[cnsAPIName]struct{})
	unsupportedAPIs["RequestIPs"] = struct{}{}

	macAddress := "12:34:56:78:9a:bc"
	parsedMacAddress, _ := net.ParseMAC(macAddress)

	// parse default route (IP: 0.0.0.0 and mask /0)
	_, ipNet, _ := net.ParseCIDR("0.0.0.0/0")

	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
		ipamMode     util.IpamMode
	}
	type args struct {
		nwCfg            *cni.NetworkConfig
		args             *cniSkel.CmdArgs
		hostSubnetPrefix *net.IPNet
		options          map[string]interface{}
	}

	tests := []struct {
		name                        string
		fields                      fields
		args                        args
		wantDefaultResult           network.InterfaceInfo
		wantSecondaryInterfacesInfo network.InterfaceInfo
		wantErr                     bool
	}{
		{
			name: "Test happy CNI Overlay add in v4overlay ipamMode",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.Overlay,
				cnsClient: &MockCNSClient{
					unsupportedAPIs: unsupportedAPIs,
					require:         require,
					requestIP: requestIPAddressHandler{
						ipconfigArgument: cns.IPConfigRequest{
							PodInterfaceID:      "testcont-testifname3",
							InfraContainerID:    "testcontainerid3",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigResponse{
							PodIpInfo: cns.PodIpInfo{
								PodIPConfig: cns.IPSubnet{
									IPAddress:    "10.240.1.242",
									PrefixLength: 16,
								},
								NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
									IPSubnet: cns.IPSubnet{
										IPAddress:    "10.240.1.0",
										PrefixLength: 16,
									},
									DNSServers:       nil,
									GatewayIPAddress: "",
								},
								HostPrimaryIPInfo: cns.HostIPInfo{
									Gateway:   "10.224.0.1",
									PrimaryIP: "10.224.0.5",
									Subnet:    "10.224.0.0/16",
								},
								NICType: cns.InfraNIC,
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid3",
					Netns:       "testnetns3",
					IfName:      "testifname3",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.224.0.0/16"),
				options:          map[string]interface{}{},
			},
			wantDefaultResult: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.240.1.242/16"),
						Gateway: getTestOverlayGateway(),
					},
				},
				Routes: []network.RouteInfo{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						Gw:  getTestOverlayGateway(),
					},
				},
				NICType:          cns.InfraNIC,
				HostSubnetPrefix: *parseCIDR("10.224.0.0/16"),
			},
			wantErr: false,
		},
		{
			name: "Test happy CNI Overlay add in dualstack overlay ipamMode",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.Overlay,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType: cns.InfraNIC,
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "fd11:1234::1",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "fd11:1234::",
											PrefixLength: 112,
										},
										DNSServers:       nil,
										GatewayIPAddress: "fe80::1234:5678:9abc",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "fe80::1234:5678:9abc",
										PrimaryIP: "fe80::1234:5678:9abc",
										Subnet:    "fd11:1234::/112",
									},
									NICType: cns.InfraNIC,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantDefaultResult: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
					{
						Address: *getCIDRNotationForAddress("fd11:1234::1/112"),
						Gateway: net.ParseIP("fe80::1234:5678:9abc"),
					},
				},
				Routes: []network.RouteInfo{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						Gw:  net.ParseIP("10.0.0.1"),
					},
					{
						Dst: network.Ipv6DefaultRouteDstPrefix,
						Gw:  net.ParseIP("fe80::1234:5678:9abc"),
					},
				},
				NICType:          cns.InfraNIC,
				HostSubnetPrefix: *parseCIDR("fd11:1234::/112"),
			},
			wantErr: false,
		},
		{
			name: "Test happy CNI add with InfraNIC + DelegatedNIC interfaces",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.Overlay,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname3",
							InfraContainerID:    "testcontainerid3",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType:           cns.InfraNIC,
									SkipDefaultRoutes: true,
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "20.240.1.242",
										PrefixLength: 24,
									},
									NICType:           cns.NodeNetworkInterfaceFrontendNIC,
									MacAddress:        macAddress,
									SkipDefaultRoutes: false,
									Routes: []cns.Route{
										{
											IPAddress:        "0.0.0.0/0",
											GatewayIPAddress: "10.0.0.1",
										},
									},
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid3",
					Netns:       "testnetns3",
					IfName:      "testifname3",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantDefaultResult: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes:            []network.RouteInfo(nil),
				NICType:           cns.InfraNIC,
				SkipDefaultRoutes: true,
				HostSubnetPrefix:  *parseCIDR("10.0.0.0/24"),
			},
			wantSecondaryInterfacesInfo: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("20.240.1.242/24"),
					},
				},
				Routes: []network.RouteInfo{
					{
						Dst: *ipNet,
						Gw:  net.ParseIP("10.0.0.1"),
					},
				},
				NICType:    cns.NodeNetworkInterfaceFrontendNIC,
				MacAddress: parsedMacAddress,
				// secondaries don't have a host subnet prefix
			},
			wantErr: false,
		},
		{
			name: "Test fail CNI add with invalid mac in delegated VM nic response",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.Overlay,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname3",
							InfraContainerID:    "testcontainerid3",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType:           cns.InfraNIC,
									SkipDefaultRoutes: true,
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "20.240.1.242",
										PrefixLength: 24,
									},
									NICType:    cns.NodeNetworkInterfaceFrontendNIC,
									MacAddress: "bad mac",
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid3",
					Netns:       "testnetns3",
					IfName:      "testifname3",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "Test fail CNI add with invalid ip config in delegated VM nic response",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.Overlay,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname3",
							InfraContainerID:    "testcontainerid3",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType:           cns.InfraNIC,
									SkipDefaultRoutes: true,
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "bad ip",
										PrefixLength: 24,
									},
									NICType:    cns.NodeNetworkInterfaceFrontendNIC,
									MacAddress: macAddress,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid3",
					Netns:       "testnetns3",
					IfName:      "testifname3",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			if tt.fields.ipamMode != "" {
				invoker.ipamMode = tt.fields.ipamMode
			}
			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}

			for _, ifInfo := range ipamAddResult.interfaceInfo {
				if ifInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
					fmt.Printf("want:%+v\nrest:%+v\n", tt.wantSecondaryInterfacesInfo, ifInfo)
					if len(tt.wantSecondaryInterfacesInfo.IPConfigs) > 0 {
						require.EqualValues(tt.wantSecondaryInterfacesInfo, ifInfo, "incorrect response for delegatedNIC")
					}
				}
				if ifInfo.NICType == cns.InfraNIC {
					require.Equalf(tt.wantDefaultResult, ifInfo, "incorrect default response")
				}
			}
		})
	}
}

func TestCNSIPAMInvoker_Add(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t
	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
		ipamMode     util.IpamMode
	}
	type args struct {
		nwCfg            *cni.NetworkConfig
		args             *cniSkel.CmdArgs
		hostSubnetPrefix *net.IPNet
		options          map[string]interface{}
	}

	tests := []struct {
		name                  string
		fields                fields
		args                  args
		wantDefaultResult     network.InterfaceInfo
		wantMultitenantResult network.InterfaceInfo
		wantErr               bool
	}{
		{
			name: "Test happy CNI add",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType: cns.InfraNIC,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantDefaultResult: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes: []network.RouteInfo{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						Gw:  net.ParseIP("10.0.0.1"),
					},
				},
				NICType:          cns.InfraNIC,
				HostSubnetPrefix: *parseCIDR("10.0.0.0/24"),
			},
			wantErr: false,
		},
		{
			name: "Test CNI add with pod ip info empty nictype",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantDefaultResult: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes: []network.RouteInfo{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						Gw:  net.ParseIP("10.0.0.1"),
					},
				},
				NICType:          cns.InfraNIC,
				HostSubnetPrefix: *parseCIDR("10.0.0.0/24"),
			},
			wantErr: false,
		},
		{
			name: "Test happy CNI add for both ipv4 and ipv6",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType: cns.InfraNIC,
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "fd11:1234::1",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "fd11:1234::",
											PrefixLength: 112,
										},
										DNSServers:       nil,
										GatewayIPAddress: "fe80::1234:5678:9abc",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "fe80::1234:5678:9abc",
										PrimaryIP: "fe80::1234:5678:9abc",
										Subnet:    "fd11:1234::/112",
									},
									NICType: cns.InfraNIC,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantDefaultResult: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
					{
						Address: *getCIDRNotationForAddress("fd11:1234::1/112"),
						Gateway: net.ParseIP("fe80::1234:5678:9abc"),
					},
				},
				Routes: []network.RouteInfo{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						Gw:  net.ParseIP("10.0.0.1"),
					},
					{
						Dst: network.Ipv6DefaultRouteDstPrefix,
						Gw:  net.ParseIP("fe80::1234:5678:9abc"),
					},
				},
				NICType:          cns.InfraNIC,
				HostSubnetPrefix: *parseCIDR("fd11:1234::/112"),
			},
			wantErr: false,
		},
		{
			name: "fail to request IP addresses from cns",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
						result:           nil,
						err:              errors.New("failed error from CNS"), //nolint "error for ut"
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			if tt.fields.ipamMode != "" {
				invoker.ipamMode = tt.fields.ipamMode
			}
			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}

			for _, ifInfo := range ipamAddResult.interfaceInfo {
				require.NotEqual("", string(ifInfo.NICType), "nictype should be auto populated if empty")
				if ifInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
					fmt.Printf("want:%+v\nrest:%+v\n", tt.wantMultitenantResult, ifInfo)
					if len(tt.wantMultitenantResult.IPConfigs) > 0 {
						require.Equalf(tt.wantMultitenantResult, ifInfo, "incorrect multitenant response")
					}
				}
				if ifInfo.NICType == cns.InfraNIC {
					require.Equalf(tt.wantDefaultResult, ifInfo, "incorrect default response")
				}
			}
		})
	}
}

func TestCNSIPAMInvoker_Add_UnsupportedAPI(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t

	// set new CNS API is not supported
	unsupportedAPIs := make(map[cnsAPIName]struct{})
	unsupportedAPIs["RequestIPs"] = struct{}{}

	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
		ipamMode     util.IpamMode
	}
	type args struct {
		nwCfg            *cni.NetworkConfig
		args             *cniSkel.CmdArgs
		hostSubnetPrefix *net.IPNet
		options          map[string]interface{}
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    network.InterfaceInfo
		wantErr bool
	}{
		{
			name: "Test happy CNI add for IPv4 without RequestIPs supported",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					unsupportedAPIs: unsupportedAPIs,
					require:         require,
					requestIP: requestIPAddressHandler{
						ipconfigArgument: getTestIPConfigRequest(),
						result: &cns.IPConfigResponse{
							PodIpInfo: cns.PodIpInfo{
								PodIPConfig: cns.IPSubnet{
									IPAddress:    "10.0.1.10",
									PrefixLength: 24,
								},
								NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
									IPSubnet: cns.IPSubnet{
										IPAddress:    "10.0.1.0",
										PrefixLength: 24,
									},
									DNSServers:       nil,
									GatewayIPAddress: "10.0.0.1",
								},
								HostPrimaryIPInfo: cns.HostIPInfo{
									Gateway:   "10.0.0.1",
									PrimaryIP: "10.0.0.1",
									Subnet:    "10.0.0.0/24",
								},
								NICType: cns.InfraNIC,
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			want: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes: []network.RouteInfo{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						Gw:  net.ParseIP("10.0.0.1"),
					},
				},
				NICType:          cns.InfraNIC,
				HostSubnetPrefix: *parseCIDR("10.0.0.0/24"),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			if tt.fields.ipamMode != "" {
				invoker.ipamMode = tt.fields.ipamMode
			}
			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if err != nil && tt.wantErr {
				t.Fatalf("expected an error %+v but none received", err)
			}
			require.NoError(err)

			for _, ifInfo := range ipamAddResult.interfaceInfo {
				if ifInfo.NICType == cns.InfraNIC {
					require.Equalf(tt.want, ifInfo, "incorrect ipv4 response")
				}
			}
		})
	}
}

func TestRequestIPAPIsFail(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t

	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
		ipamMode     util.IpamMode
	}
	type args struct {
		nwCfg            *cni.NetworkConfig
		args             *cniSkel.CmdArgs
		hostSubnetPrefix *net.IPNet
		options          map[string]interface{}
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "Test happy CNI add for dualstack mode with both requestIP and requestIPs get failed",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "fd11:1234::1",
										PrefixLength: 112,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "fd11:1234::",
											PrefixLength: 112,
										},
										DNSServers:       nil,
										GatewayIPAddress: "fe80::1234:5678:9abc",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "fe80::1234:5678:9abc",
										PrimaryIP: "fe80::1234:5678:9abc",
										Subnet:    "fd11:1234::/112",
									},
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns1",
					IfName:      "testifname1",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			if tt.fields.ipamMode != "" {
				invoker.ipamMode = tt.fields.ipamMode
			}
			_, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if err == nil && tt.wantErr {
				t.Fatalf("expected an error %+v but none received", err)
			}
			if !errors.Is(err, errNoRequestIPFound) {
				t.Fatalf("expected an error %s but %v received", errNoRequestIPFound, err)
			}
		})
	}
}

func TestCNSIPAMInvoker_Delete(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t
	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
	}
	type args struct {
		address *net.IPNet
		nwCfg   *cni.NetworkConfig
		args    *cniSkel.CmdArgs
		options map[string]interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test delete happy path",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					releaseIPs: releaseIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
					},
				},
			},
			args: args{
				nwCfg: nil,
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				options: map[string]interface{}{},
			},
		},
		{
			name: "test delete not happy path",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					releaseIPs: releaseIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
						err:              errors.New("handle CNS delete error"), //nolint ut error
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.args, tt.args.options)
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}
		})
	}
}

func TestCNSIPAMInvoker_Delete_Overlay(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t

	// set new CNS API is not supported
	unsupportedAPIs := make(map[cnsAPIName]struct{})
	unsupportedAPIs["ReleaseIPs"] = struct{}{}

	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
		ipamMode     util.IpamMode
	}
	type args struct {
		address *net.IPNet
		nwCfg   *cni.NetworkConfig
		args    *cniSkel.CmdArgs
		options map[string]interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test delete happy path in v4overlay ipamMode",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.V4Overlay,
				cnsClient: &MockCNSClient{
					unsupportedAPIs: unsupportedAPIs,
					require:         require,
					releaseIP: releaseIPHandler{
						ipconfigArgument: getTestIPConfigRequest(),
					},
				},
			},
			args: args{
				nwCfg: nil,
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				options: map[string]interface{}{},
			},
		},
		{
			name: "test delete happy path in dualStackOverlay ipamMode",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.DualStackOverlay,
				cnsClient: &MockCNSClient{
					require: require,
					releaseIPs: releaseIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
					},
				},
			},
			args: args{
				nwCfg: nil,
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				options: map[string]interface{}{},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.args, tt.args.options)
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}
		})
	}
}

func TestCNSIPAMInvoker_Delete_NotSupportedAPI(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t
	// set new CNS API is not supported
	unsupportedAPIs := make(map[cnsAPIName]struct{})
	unsupportedAPIs["ReleaseIPs"] = struct{}{}

	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
	}
	type args struct {
		address *net.IPNet
		nwCfg   *cni.NetworkConfig
		args    *cniSkel.CmdArgs
		options map[string]interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test delete happy path with unsupportedAPI",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					unsupportedAPIs: unsupportedAPIs,
					require:         require,
					releaseIP: releaseIPHandler{
						ipconfigArgument: getTestIPConfigRequest(),
					},
				},
			},
			args: args{
				nwCfg: nil,
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
				},
				options: map[string]interface{}{},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.args, tt.args.options)
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}
		})
	}
}

func TestReleaseIPAPIsFail(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t
	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
	}
	type args struct {
		address *net.IPNet
		nwCfg   *cni.NetworkConfig
		args    *cniSkel.CmdArgs
		options map[string]interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "test delete with both cns releaseIPs and releaseIP get failed",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					releaseIPs: releaseIPsHandler{
						ipconfigArgument: getTestIPConfigsRequest(),
					},
				},
			},
			args: args{
				nwCfg: nil,
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns1",
					IfName:      "testifname1",
				},
				options: map[string]interface{}{},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.args, tt.args.options)
			if !errors.Is(err, errNoReleaseIPFound) {
				t.Fatalf("expected an error %s but %v received", errNoReleaseIPFound, err)
			}
		})
	}
}

func Test_setHostOptions(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t
	type args struct {
		hostSubnetPrefix *net.IPNet
		ncSubnetPrefix   *net.IPNet
		options          map[string]interface{}
		info             IPResultInfo
	}
	tests := []struct {
		name        string
		args        args
		wantOptions map[string]interface{}
		wantErr     bool
	}{
		{
			name: "test happy path",
			args: args{
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.1.0/24"),
				ncSubnetPrefix:   getCIDRNotationForAddress("10.0.1.0/24"),
				options:          map[string]interface{}{},
				info: IPResultInfo{
					podIPAddress:       "10.0.1.10",
					ncSubnetPrefix:     24,
					ncPrimaryIP:        "10.0.1.20",
					ncGatewayIPAddress: "10.0.1.1",
					hostSubnet:         "10.0.0.0/24",
					hostPrimaryIP:      "10.0.0.3",
					hostGateway:        "10.0.0.1",
				},
			},
			wantOptions: map[string]interface{}{
				network.IPTablesKey: []iptables.IPTableEntry{
					{
						Version: "4",
						Params:  "-t nat -N SWIFT",
					},
					{
						Version: "4",
						Params:  "-t nat -A POSTROUTING  -j SWIFT",
					},
					{
						Version: "4",
						Params:  "-t nat -I SWIFT 1  -m addrtype ! --dst-type local -s 10.0.1.0/24 -d 168.63.129.16 -p udp --dport 53 -j SNAT --to 10.0.1.20",
					},
					{
						Version: "4",
						Params:  "-t nat -I SWIFT 1  -m addrtype ! --dst-type local -s 10.0.1.0/24 -d 168.63.129.16 -p tcp --dport 53 -j SNAT --to 10.0.1.20",
					},
					{
						Version: "4",
						Params:  "-t nat -I SWIFT 1  -m addrtype ! --dst-type local -s 10.0.1.0/24 -d 169.254.169.254 -p tcp --dport 80 -j SNAT --to 10.0.0.3",
					},
				},
				network.RoutesKey: []network.RouteInfo{
					{
						Dst: *getCIDRNotationForAddress("10.0.1.0/24"),
						Gw:  net.ParseIP("10.0.0.1"),
					},
				},
			},

			wantErr: false,
		},
		{
			name: "test error on bad host subnet",
			args: args{
				info: IPResultInfo{
					hostSubnet: "",
				},
			},
			wantErr: true,
		},
		{
			name: "test error on nil hostsubnetprefix",
			args: args{
				info: IPResultInfo{
					hostSubnet: "10.0.0.0/24",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := setHostOptions(tt.args.ncSubnetPrefix, tt.args.options, &tt.args.info)
			if tt.wantErr {
				require.Error(err)
				return
			}
			require.NoError(err)

			require.Exactly(tt.wantOptions, tt.args.options)
		})
	}
}

func Test_getInterfaceInfoKey(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t
	inv := &CNSIPAMInvoker{}
	dummyMAC := "12:34:56:78:9a:bc"
	require.Equal(string(cns.InfraNIC), inv.getInterfaceInfoKey(cns.InfraNIC, dummyMAC))
	require.Equal(dummyMAC, inv.getInterfaceInfoKey(cns.NodeNetworkInterfaceFrontendNIC, dummyMAC))
	require.Equal("", inv.getInterfaceInfoKey(cns.NodeNetworkInterfaceFrontendNIC, ""))
	require.Equal(dummyMAC, inv.getInterfaceInfoKey(cns.BackendNIC, dummyMAC))
	require.Equal("", inv.getInterfaceInfoKey(cns.BackendNIC, ""))
}

func TestCNSIPAMInvoker_Add_SwiftV2(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t

	macAddress := "12:34:56:78:9a:bc"
	parsedMacAddress, _ := net.ParseMAC(macAddress)

	ibMacAddress := "bc:9a:78:56:34:12"
	ibParsedMacAddress, _ := net.ParseMAC(ibMacAddress)

	pnpID := "PCI\\VEN_15B3&DEV_101C&SUBSYS_000715B3&REV_00\\5&8c5acce&0&0"

	// parse default route (IP: 0.0.0.0 and mask /0)
	_, ipNet, _ := net.ParseCIDR("0.0.0.0/0")

	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
	}

	type args struct {
		nwCfg            *cni.NetworkConfig
		args             *cniSkel.CmdArgs
		hostSubnetPrefix *net.IPNet
		options          map[string]interface{}
	}

	tests := []struct {
		name                        string
		fields                      fields
		args                        args
		wantDefaultResult           network.InterfaceInfo
		wantSecondaryInterfacesInfo map[string]network.InterfaceInfo
		wantErr                     bool
	}{
		{
			name: "Test happy CNI add delegatedVMNIC type",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname1",
							InfraContainerID:    "testcontainerid1",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.1.1.10",
										PrefixLength: 24,
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.2",
										Subnet:    "10.0.0.1/24",
									},
									NICType:    cns.NodeNetworkInterfaceFrontendNIC,
									MacAddress: macAddress,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid1",
					Netns:       "testnetns1",
					IfName:      "testifname1",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantSecondaryInterfacesInfo: map[string]network.InterfaceInfo{
				macAddress: {
					IPConfigs: []*network.IPConfig{
						{
							Address: *getCIDRNotationForAddress("10.1.1.10/24"),
						},
					},
					Routes:     []network.RouteInfo{},
					NICType:    cns.NodeNetworkInterfaceFrontendNIC,
					MacAddress: parsedMacAddress,
				},
			},
			wantErr: false,
		},
		{
			name: "Test happy CNI add with DelegatedNIC + BackendNIC interfaces",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname1",
							InfraContainerID:    "testcontainerid1",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.1.1.10",
										PrefixLength: 24,
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.2",
										Subnet:    "10.0.0.1/24",
									},
									NICType:           cns.NodeNetworkInterfaceFrontendNIC,
									MacAddress:        macAddress,
									SkipDefaultRoutes: false,
								},
								{
									MacAddress: ibMacAddress,
									NICType:    cns.BackendNIC,
									PnPID:      pnpID,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid1",
					Netns:       "testnetns1",
					IfName:      "testifname1",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantSecondaryInterfacesInfo: map[string]network.InterfaceInfo{
				macAddress: {
					IPConfigs: []*network.IPConfig{
						{
							Address: *getCIDRNotationForAddress("10.1.1.10/24"),
						},
					},
					Routes:     []network.RouteInfo{},
					NICType:    cns.NodeNetworkInterfaceFrontendNIC,
					MacAddress: parsedMacAddress,
				},
				ibMacAddress: {
					NICType:    cns.BackendNIC,
					MacAddress: ibParsedMacAddress,
					PnPID:      pnpID,
				},
			},
			wantErr: false,
		},
		{
			name: "Test happy CNI add with InfraNIC + DelegatedNIC + BackendNIC interfaces",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname1",
							InfraContainerID:    "testcontainerid1",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType:           cns.InfraNIC,
									SkipDefaultRoutes: true,
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "20.1.1.10",
										PrefixLength: 24,
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "20.0.0.1",
										PrimaryIP: "20.0.0.2",
										Subnet:    "20.0.0.1/24",
									},
									NICType:           cns.NodeNetworkInterfaceFrontendNIC,
									MacAddress:        macAddress,
									SkipDefaultRoutes: false,
									Routes: []cns.Route{
										{
											IPAddress:        "0.0.0.0/0",
											GatewayIPAddress: "10.0.0.1",
										},
									},
								},
								{
									MacAddress: ibMacAddress,
									NICType:    cns.BackendNIC,
									PnPID:      pnpID,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid1",
					Netns:       "testnetns1",
					IfName:      "testifname1",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			wantDefaultResult: network.InterfaceInfo{
				IPConfigs: []*network.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes:            []network.RouteInfo(nil),
				NICType:           cns.InfraNIC,
				SkipDefaultRoutes: true,
				HostSubnetPrefix:  *parseCIDR("10.0.0.0/24"),
			},
			wantSecondaryInterfacesInfo: map[string]network.InterfaceInfo{
				macAddress: {
					IPConfigs: []*network.IPConfig{
						{
							Address: *getCIDRNotationForAddress("20.1.1.10/24"),
						},
					},
					Routes: []network.RouteInfo{{
						Dst: *ipNet,
						Gw:  net.ParseIP("10.0.0.1"),
					}},
					NICType:    cns.NodeNetworkInterfaceFrontendNIC,
					MacAddress: parsedMacAddress,
				},
				ibMacAddress: {
					NICType:    cns.BackendNIC,
					MacAddress: ibParsedMacAddress,
					PnPID:      pnpID,
					Routes:     []network.RouteInfo(nil),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}

			for _, ifInfo := range ipamAddResult.interfaceInfo {
				if ifInfo.NICType == cns.InfraNIC {
					require.Equalf(tt.wantDefaultResult, ifInfo, "incorrect ipv4 response")
				}

				if ifInfo.NICType == cns.BackendNIC {
					fmt.Printf("want:%+v\nrest:%+v\n", tt.wantSecondaryInterfacesInfo, ipamAddResult.interfaceInfo[ibMacAddress])
					require.EqualValues(tt.wantSecondaryInterfacesInfo[ibMacAddress], ipamAddResult.interfaceInfo[ibMacAddress], "incorrect response for IB")
				}

				if ifInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
					fmt.Printf("want:%+v\nrest:%+v\n", tt.wantSecondaryInterfacesInfo[macAddress], ipamAddResult.interfaceInfo[macAddress])
					require.EqualValues(tt.wantSecondaryInterfacesInfo[macAddress], ipamAddResult.interfaceInfo[macAddress], "incorrect response for Delegated")
				}
			}
		})
	}
}

func TestShallowCopyIpamAddConfigOptions(t *testing.T) {
	opts := IPAMAddConfig{
		// mock different types of map value
		options: map[string]interface{}{
			network.SNATIPKey:   "10",
			dockerNetworkOption: "20",
			"intType":           10,
			"floatType":         0.51,
			"byteType":          byte('A'),
		},
	}

	// shallow copy all ipamAddConfig options
	res := opts.shallowCopyIpamAddConfigOptions()
	require.Equal(t, opts.options, res)

	// modified copied res and make sure original opts is not changed
	newSNATIPKeyValue := "100"
	newDockerNetworkOptionValue := "200"

	res[network.SNATIPKey] = newSNATIPKeyValue
	res[dockerNetworkOption] = newDockerNetworkOptionValue

	expectedOpts := map[string]interface{}{
		network.SNATIPKey:   newSNATIPKeyValue,
		dockerNetworkOption: newDockerNetworkOptionValue,
		"intType":           10,
		"floatType":         0.51,
		"byteType":          byte('A'),
	}
	require.Equal(t, expectedOpts, res)

	// make sure original object is equal to expected opts after copied res is changed
	expectedOriginalOpts := map[string]interface{}{
		network.SNATIPKey:   "10",
		dockerNetworkOption: "20",
		"intType":           10,
		"floatType":         0.51,
		"byteType":          byte('A'),
	}
	require.Equal(t, expectedOriginalOpts, opts.options)

	// shallow copy empty opts and make sure it does not break anything
	emptyOpts := IPAMAddConfig{
		options: map[string]interface{}{},
	}
	emptyRes := emptyOpts.shallowCopyIpamAddConfigOptions()
	require.Equal(t, emptyOpts.options, emptyRes)

	// shallow copy null opts and make sure it does not break anything
	nullOpts := IPAMAddConfig{
		options: nil,
	}
	nullRes := nullOpts.shallowCopyIpamAddConfigOptions()
	require.Equal(t, map[string]interface{}{}, nullRes)
}

// Test addBackendNICToResult() and configureSecondaryAddResult() to update secondary interfaces to cni Result
func TestAddNICsToCNIResult(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t

	macAddress := "bc:9a:78:56:34:12"
	newMacAddress := "bc:9a:78:56:34:45"
	newParsedMacAddress, _ := net.ParseMAC(newMacAddress)
	newNCGatewayIPAddress := "40.0.0.1"

	pnpID := "PCI\\VEN_15B3&DEV_101C&SUBSYS_000715B3&REV_00\\5&8c5acce&0&0"
	newPnpID := "PCI\\VEN_15B3&DEV_101C&SUBSYS_000715B3&REV_00\\5&8c5acce&0&1"

	newPodIPConfig := &cns.IPSubnet{
		IPAddress:    "30.1.1.10",
		PrefixLength: 24,
	}

	newIP, newIPNet, _ := newPodIPConfig.GetIPNet()

	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient
	}

	type args struct {
		nwCfg            *cni.NetworkConfig
		args             *cniSkel.CmdArgs
		hostSubnetPrefix *net.IPNet
		options          map[string]interface{}
		info             IPResultInfo
		podIPConfig      *cns.IPSubnet
	}

	tests := []struct {
		name                        string
		fields                      fields
		args                        args
		wantSecondaryInterfacesInfo map[string]network.InterfaceInfo
	}{
		{
			name: "add new backendNIC to cni Result",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname1",
							InfraContainerID:    "testcontainerid1",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType: cns.InfraNIC,
								},
								{
									MacAddress: macAddress,
									NICType:    cns.BackendNIC,
									PnPID:      pnpID,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid1",
					Netns:       "testnetns1",
					IfName:      "testifname1",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
				// update new pnpID, macAddress
				info: IPResultInfo{
					pnpID:      newPnpID,
					macAddress: newMacAddress,
					nicType:    cns.BackendNIC,
				},
			},
			wantSecondaryInterfacesInfo: map[string]network.InterfaceInfo{
				macAddress: {
					MacAddress: newParsedMacAddress,
					PnPID:      newPnpID,
					NICType:    cns.BackendNIC,
				},
			},
		},
		{
			name: "add new delegatedVMNIC to cni Result",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				cnsClient: &MockCNSClient{
					require: require,
					requestIPs: requestIPsHandler{
						ipconfigArgument: cns.IPConfigsRequest{
							PodInterfaceID:      "testcont-testifname1",
							InfraContainerID:    "testcontainerid1",
							OrchestratorContext: marshallPodInfo(testPodInfo),
						},
						result: &cns.IPConfigsResponse{
							PodIPInfo: []cns.PodIpInfo{
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "10.0.1.10",
										PrefixLength: 24,
									},
									NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
										IPSubnet: cns.IPSubnet{
											IPAddress:    "10.0.1.0",
											PrefixLength: 24,
										},
										DNSServers:       nil,
										GatewayIPAddress: "10.0.0.1",
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "10.0.0.1",
										PrimaryIP: "10.0.0.1",
										Subnet:    "10.0.0.0/24",
									},
									NICType:           cns.InfraNIC,
									SkipDefaultRoutes: true,
								},
								{
									PodIPConfig: cns.IPSubnet{
										IPAddress:    "20.1.1.10",
										PrefixLength: 24,
									},
									HostPrimaryIPInfo: cns.HostIPInfo{
										Gateway:   "20.0.0.1",
										PrimaryIP: "20.0.0.2",
										Subnet:    "20.0.0.1/24",
									},
									NICType:    cns.NodeNetworkInterfaceFrontendNIC,
									MacAddress: macAddress,
								},
							},
							Response: cns.Response{
								ReturnCode: 0,
								Message:    "",
							},
						},
						err: nil,
					},
				},
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid1",
					Netns:       "testnetns1",
					IfName:      "testifname1",
				},
				hostSubnetPrefix: getCIDRNotationForAddress("10.0.0.1/24"),
				options:          map[string]interface{}{},
				// update podIPConfig
				podIPConfig: newPodIPConfig,
				// update new mac address and ncGatewayIPAddress
				info: IPResultInfo{
					macAddress:         newMacAddress,
					nicType:            cns.NodeNetworkInterfaceFrontendNIC,
					ncGatewayIPAddress: newNCGatewayIPAddress,
				},
			},
			wantSecondaryInterfacesInfo: map[string]network.InterfaceInfo{
				macAddress: {
					MacAddress: newParsedMacAddress,
					NICType:    cns.NodeNetworkInterfaceFrontendNIC,
					IPConfigs: []*network.IPConfig{
						{
							Address: net.IPNet{
								IP:   newIP,
								Mask: newIPNet.Mask,
							},
							Gateway: net.ParseIP(newNCGatewayIPAddress),
						},
					},
					Routes: []network.RouteInfo{},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if err != nil {
				t.Fatalf("Failed to create ipamAddResult due to error: %v", err)
			}

			for _, ifInfo := range ipamAddResult.interfaceInfo {
				if ifInfo.NICType == cns.BackendNIC {
					// add new backendNIC info to cni Result
					err := addBackendNICToResult(&tt.args.info, &ipamAddResult, macAddress)
					if err != nil {
						t.Fatalf("Failed to add backend NIC to cni Result due to error %v", err)
					}
					fmt.Printf("want:%+v\nrest:%+v\n", tt.wantSecondaryInterfacesInfo, ipamAddResult.interfaceInfo[macAddress])
					require.EqualValues(tt.wantSecondaryInterfacesInfo[macAddress], ipamAddResult.interfaceInfo[macAddress], "incorrect response for IB")
				}
				if ifInfo.NICType == cns.NodeNetworkInterfaceFrontendNIC {
					// add new secondaryInterfaceNIC to cni Result
					err := configureSecondaryAddResult(&tt.args.info, &ipamAddResult, tt.args.podIPConfig, macAddress)
					if err != nil {
						t.Fatalf("Failed to add secondary interface NIC %s to cni Result due to error %v", ifInfo.NICType, err)
					}
					fmt.Printf("want:%+v\nrest:%+v\n", tt.wantSecondaryInterfacesInfo, ipamAddResult.interfaceInfo[macAddress])
					require.EqualValues(tt.wantSecondaryInterfacesInfo[macAddress], ipamAddResult.interfaceInfo[macAddress], "incorrect response for delegatedVMNIC")
				}
			}
		})
	}
}
