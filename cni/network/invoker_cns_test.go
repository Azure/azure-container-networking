package network

import (
	"errors"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/stretchr/testify/require"
)

var testPodInfo cns.KubernetesPodInfo

func getTestIPConfigsRequest() cns.IPConfigsRequest {
	return cns.IPConfigsRequest{
		PodInterfaceID:      "testcont-testifname",
		InfraContainerID:    "testcontainerid",
		OrchestratorContext: marshallPodInfo(testPodInfo),
	}
}

func TestCNSIPAMInvoker_Add(t *testing.T) {
	require := require.New(t) //nolint further usage of require without passing t
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
		name    string
		fields  fields
		args    args
		want    *cniTypesCurr.Result
		want1   *cniTypesCurr.Result
		wantErr bool
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
			want1: &cniTypesCurr.Result{
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
			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.args, options: tt.args.options})
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}

			require.Equalf(tt.want, ipamAddResult.ipv4Result, "incorrect ipv4 response")
			require.Equalf(tt.want1, ipamAddResult.ipv6Result, "incorrect ipv6 response")
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
		addresses []*net.IPNet
		nwCfg     *cni.NetworkConfig
		args      *cniSkel.CmdArgs
		options   map[string]interface{}
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
					release: releaseIPsHandler{
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
					release: releaseIPsHandler{
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
			err := invoker.Delete(tt.args.addresses, tt.args.nwCfg, tt.args.args, tt.args.options)
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
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
