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
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
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
		name           string
		fields         fields
		args           args
		wantIpv4Result *cniTypesCurr.Result
		wantIpv6Result *cniTypesCurr.Result
		wantErr        bool
	}{
		{
			name: "Test happy CNI Overlay add in v4overlay ipamMode",
			fields: fields{
				podName:      testPodInfo.PodName,
				podNamespace: testPodInfo.PodNamespace,
				ipamMode:     util.V4Overlay,
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
			wantIpv4Result: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.240.1.242/16"),
						Gateway: getTestOverlayGateway(),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						GW:  getTestOverlayGateway(),
					},
				},
			},
			wantIpv6Result: nil,
			wantErr:        false,
		},
		{
			name: "Test happy CNI Overlay add in dualstack overlay ipamMode",
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
			wantIpv4Result: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						GW:  net.ParseIP("10.0.0.1"),
					},
				},
			},
			wantIpv6Result: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: *getCIDRNotationForAddress("fd11:1234::1/112"),
						Gateway: net.ParseIP("fe80::1234:5678:9abc"),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv6DefaultRouteDstPrefix,
						GW:  net.ParseIP("fe80::1234:5678:9abc"),
					},
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
			if tt.fields.ipamMode != "" {
				invoker.ipamMode = tt.fields.ipamMode
			}
			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}

			fmt.Printf("want:%+v\nrest:%+v\n", tt.wantIpv4Result, ipamAddResult.ipv4Result)
			require.Equalf(tt.wantIpv4Result, ipamAddResult.ipv4Result, "incorrect ipv4 response")
			require.Equalf(tt.wantIpv6Result, ipamAddResult.ipv6Result, "incorrect ipv6 response")
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
		name           string
		fields         fields
		args           args
		wantIpv4Result *cniTypesCurr.Result
		wantIpv6Result *cniTypesCurr.Result
		wantErr        bool
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
			wantIpv4Result: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						GW:  net.ParseIP("10.0.0.1"),
					},
				},
			},
			wantIpv6Result: nil,
			wantErr:        false,
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
			wantIpv4Result: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						GW:  net.ParseIP("10.0.0.1"),
					},
				},
			},
			wantIpv6Result: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: *getCIDRNotationForAddress("fd11:1234::1/112"),
						Gateway: net.ParseIP("fe80::1234:5678:9abc"),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv6DefaultRouteDstPrefix,
						GW:  net.ParseIP("fe80::1234:5678:9abc"),
					},
				},
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

			fmt.Printf("want:%+v\nrest:%+v\n", tt.wantIpv4Result, ipamAddResult.ipv4Result)
			require.Equalf(tt.wantIpv4Result, ipamAddResult.ipv4Result, "incorrect ipv4 response")
			require.Equalf(tt.wantIpv6Result, ipamAddResult.ipv6Result, "incorrect ipv6 response")
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
		want    *cniTypesCurr.Result
		want1   *cniTypesCurr.Result
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
			want: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Address: *getCIDRNotationForAddress("10.0.1.10/24"),
						Gateway: net.ParseIP("10.0.0.1"),
					},
				},
				Routes: []*cniTypes.Route{
					{
						Dst: network.Ipv4DefaultRouteDstPrefix,
						GW:  net.ParseIP("10.0.0.1"),
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
			if err != nil && tt.wantErr {
				t.Fatalf("expected an error %+v but none received", err)
			}
			require.NoError(err)
			require.Equalf(tt.want, ipamAddResult.ipv4Result, "incorrect ipv4 response")
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
		want    *cniTypesCurr.Result
		want1   *cniTypesCurr.Result
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
