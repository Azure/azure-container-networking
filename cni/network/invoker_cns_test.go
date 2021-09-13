package network

import (
	"net"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/network"
	cniTypes "github.com/containernetworking/cni/pkg/types"

	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-container-networking/cns"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

type MockCNSClient struct {
	requestResponse    *cns.IPConfigResponse
	requestResponseErr error
	releaseResponseErr error
}

func (c *MockCNSClient) RequestIPAddress(_ *cns.IPConfigRequest) (*cns.IPConfigResponse, error) {
	return c.requestResponse, c.requestResponseErr
}
func (c *MockCNSClient) ReleaseIPAddress(_ *cns.IPConfigRequest) error {
	return c.releaseResponseErr
}

func GetCIDRNotationForAddress(t *testing.T, ipaddresswithcidr string) *net.IPNet {
	ip, ipnet, err := net.ParseCIDR(ipaddresswithcidr)
	require.NoError(t, err)
	ipnet.IP = ip
	return ipnet
}

func TestCNSIPAMInvoker_Add(t *testing.T) {
	require := require.New(t)
	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient.CNSClientInterface
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
				podName:      "testpod",
				podNamespace: "testnamespace",
				cnsClient: &MockCNSClient{
					requestResponse: &cns.IPConfigResponse{
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
					requestResponseErr: nil,
					releaseResponseErr: nil,
				},
			},
			args: args{
				nwCfg: nil,
				args: &cniSkel.CmdArgs{
					ContainerID: "testcontainerid",
					Netns:       "testnetns",
					IfName:      "testifname",
					Args:        "",
					Path:        "",
					StdinData:   nil,
				},
				hostSubnetPrefix: GetCIDRNotationForAddress(t, "10.0.0.1/24"),
				options:          map[string]interface{}{},
			},
			want: &cniTypesCurr.Result{
				IPs: []*cniTypesCurr.IPConfig{
					{
						Version: "4",
						Address: *GetCIDRNotationForAddress(t, "10.0.1.10/24"),
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
			want1:   nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			got, got1, err := invoker.Add(tt.args.nwCfg, tt.args.args, tt.args.hostSubnetPrefix, tt.args.options)
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
			}

			require.Equalf(tt.want, got, "incorrect ipv4 response")
			require.Equalf(tt.want1, got1, "incorrect ipv6 response")
		})
	}
}

func TestCNSIPAMInvoker_Delete(t *testing.T) {
	type fields struct {
		podName      string
		podNamespace string
		cnsClient    cnsclient.CNSClientInterface
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
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &CNSIPAMInvoker{
				podName:      tt.fields.podName,
				podNamespace: tt.fields.podNamespace,
				cnsClient:    tt.fields.cnsClient,
			}
			if err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.args, tt.args.options); (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCNSInvoker(t *testing.T) {
	type args struct {
		podName   string
		namespace string
		cnsClient cnsclient.CNSClientInterface
	}
	tests := []struct {
		name    string
		args    args
		want    *CNSIPAMInvoker
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCNSInvoker(tt.args.podName, tt.args.namespace, tt.args.cnsClient)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewCNSInvoker() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_setHostOptions(t *testing.T) {
	type args struct {
		hostSubnetPrefix *net.IPNet
		ncSubnetPrefix   *net.IPNet
		options          map[string]interface{}
		info             IPv4ResultInfo
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := setHostOptions(tt.args.hostSubnetPrefix, tt.args.ncSubnetPrefix, tt.args.options, tt.args.info); (err != nil) != tt.wantErr {
				t.Errorf("setHostOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
