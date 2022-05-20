package network

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
	"github.com/stretchr/testify/require"
)

type mockDelegatePlugin struct {
	add
	del
}

type add struct {
	resultsIPv4Index int
	resultsIPv4      [](*cniTypesCurr.Result)
	resultsIPv6Index int
	resultsIPv6      [](*cniTypesCurr.Result)
	errv4            error
	errv6            error
}

func (d *add) DelegateAdd(pluginName string, nwCfg *cni.NetworkConfig) (*cniTypesCurr.Result, error) {
	if pluginName == ipamV6 {
		if d.errv6 != nil {
			return nil, d.errv6
		}
		if d.resultsIPv6 == nil || d.resultsIPv6Index-1 > len(d.resultsIPv6) {
			return nil, errors.New("no more ipv6 results in mock available") //nolint:goerr113
		}
		res := d.resultsIPv6[d.resultsIPv6Index]
		d.resultsIPv6Index++
		return res, nil
	}

	if d.errv4 != nil {
		return nil, d.errv4
	}
	if d.resultsIPv4 == nil || d.resultsIPv4Index-1 > len(d.resultsIPv4) {
		return nil, errors.New("no more ipv4 results in mock available") //nolint:goerr113
	}
	res := d.resultsIPv4[d.resultsIPv4Index]
	d.resultsIPv4Index++
	return res, nil
}

type del struct {
	err error
}

func (d *del) DelegateDel(pluginName string, nwCfg *cni.NetworkConfig) error {
	if d.err != nil {
		return d.err
	}
	return nil
}

func (m *mockDelegatePlugin) Errorf(format string, args ...interface{}) *cniTypes.Error {
	return &cniTypes.Error{
		Code:    1,
		Msg:     fmt.Sprintf(format, args...),
		Details: "",
	}
}

func getCIDRNotationForAddress(ipaddresswithcidr string) *net.IPNet {
	ip, ipnet, err := net.ParseCIDR(ipaddresswithcidr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse cidr with err: %v", err))
	}
	ipnet.IP = ip
	return ipnet
}

func getResult(ip string) []*cniTypesCurr.Result {
	res := []*cniTypesCurr.Result{
		{
			IPs: []*cniTypesCurr.IPConfig{
				{
					Address: *getCIDRNotationForAddress(ip),
				},
			},
		},
	}
	return res
}

// used in the tests below, unused ignores tags
type ipamStruct struct { //nolint:unused
	Mode          string `json:"mode,omitempty"`
	Type          string `json:"type"`
	Environment   string `json:"environment,omitempty"`
	AddrSpace     string `json:"addressSpace,omitempty"`
	Subnet        string `json:"subnet,omitempty"`
	Address       string `json:"ipAddress,omitempty"`
	QueryInterval string `json:"queryInterval,omitempty"`
}

func getNwInfo(subnetv4, subnetv6 string) *network.NetworkInfo {
	nwinfo := &network.NetworkInfo{}
	if subnetv4 != "" {
		nwinfo.Subnets = append(nwinfo.Subnets, network.SubnetInfo{
			Prefix: *getCIDRNotationForAddress(subnetv4),
		})
	}
	if subnetv6 != "" {
		nwinfo.Subnets = append(nwinfo.Subnets, network.SubnetInfo{
			Prefix: *getCIDRNotationForAddress(subnetv6),
		})
	}
	return nwinfo
}

func TestAzureIPAMInvoker_Add(t *testing.T) {
	require := require.New(t)
	type fields struct {
		plugin delegatePlugin
		nwInfo *network.NetworkInfo
	}
	type args struct {
		nwCfg        *cni.NetworkConfig
		in1          *cniSkel.CmdArgs
		subnetPrefix *net.IPNet
		options      map[string]interface{}
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		want        *cniTypesCurr.Result
		want1       *cniTypesCurr.Result
		wantErrType error
		wantErr     bool
	}{
		{
			name: "happy add ipv4",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getResult("10.0.0.1/24"),
					},
					del: del{},
				},
				nwInfo: getNwInfo("10.0.0.0/24", ""),
			},
			args: args{
				nwCfg:        &cni.NetworkConfig{},
				subnetPrefix: getCIDRNotationForAddress("10.0.0.0/24"),
			},
			want:    getResult("10.0.0.1/24")[0],
			wantErr: false,
		},
		{
			name: "happy add ipv4+ipv6",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getResult("10.0.0.1/24"),
						resultsIPv6: getResult("2001:0db8:abcd:0015::0/64"),
					},
				},
				nwInfo: getNwInfo("10.0.0.0/24", "2001:db8:abcd:0012::0/64"),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{
					IPV6Mode: network.IPV6Nat,
				},
				subnetPrefix: getCIDRNotationForAddress("10.0.0.0/24"),
			},
			want:    getResult("10.0.0.1/24")[0],
			want1:   getResult("2001:0db8:abcd:0015::0/64")[0],
			wantErr: false,
		},
		{
			name: "error on add ipv4",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						errv4: errors.New("test error"), //nolint:goerr113
					},
				},
				nwInfo: getNwInfo("10.0.0.0/24", ""),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
			},
			want:    nil,
			want1:   nil,
			wantErr: true,
		},
		{
			name: "error on ipv4+ipv6",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getResult("10.0.0.1/24"),
						errv6:       errors.New("test v6 error"), //nolint:goerr113
					},
				},
				nwInfo: getNwInfo("10.0.0.0/24", ""),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{
					IPV6Mode: network.IPV6Nat,
				},
				subnetPrefix: getCIDRNotationForAddress("10.0.0.0/24"),
			},
			want:    getResult("10.0.0.1/24")[0],
			want1:   nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &AzureIPAMInvoker{
				plugin: tt.fields.plugin,
				nwInfo: tt.fields.nwInfo,
			}

			ipamAddResult, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.in1, options: tt.args.options})
			if tt.wantErr {
				require.NotNil(err) // use NotNil since *cniTypes.Error is not of type Error
			} else {
				require.Nil(err)
			}

			require.Exactly(tt.want, ipamAddResult.ipv4Result)
			require.Exactly(tt.want1, ipamAddResult.ipv6Result)
		})
	}
}

func TestAzureIPAMInvoker_Delete(t *testing.T) {
	require := require.New(t)
	type fields struct {
		plugin delegatePlugin
		nwInfo *network.NetworkInfo
	}
	type args struct {
		address *net.IPNet
		nwCfg   *cni.NetworkConfig
		in2     *cniSkel.CmdArgs
		options map[string]interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "delete happy path ipv4",
			fields: fields{
				plugin: &mockDelegatePlugin{
					del: del{},
				},
				nwInfo: getNwInfo("10.0.0.0/24", ""),
			},
			args: args{
				address: getCIDRNotationForAddress("10.0.0.4/24"),
				nwCfg: &cni.NetworkConfig{
					Ipam: ipamStruct{
						Address: "10.0.0.4",
					},
				},
			},
		},
		{
			name: "delete happy path ipv6",
			fields: fields{
				plugin: &mockDelegatePlugin{
					del: del{},
				},
				nwInfo: getNwInfo("10.0.0.0/24", "2001:db8:abcd:0012::0/64"),
			},
			args: args{
				address: getCIDRNotationForAddress("2001:db8:abcd:0015::0/64"),
				nwCfg: &cni.NetworkConfig{
					Ipam: ipamStruct{
						Address: "2001:db8:abcd:0015::0/64",
					},
				},
			},
		},
		{
			name: "error address is nil",
			fields: fields{
				plugin: &mockDelegatePlugin{
					del: del{
						err: errors.New("error when address is nil"), //nolint:goerr113
					},
				},
				nwInfo: getNwInfo("", "2001:db8:abcd:0012::0/64"),
			},
			args: args{
				address: nil,
				nwCfg: &cni.NetworkConfig{
					Ipam: ipamStruct{
						Address: "2001:db8:abcd:0015::0/64",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error on v4 delete",
			fields: fields{
				plugin: &mockDelegatePlugin{
					del: del{
						err: errors.New("error on v4 delete"), //nolint:goerr113
					},
				},
				nwInfo: getNwInfo("10.0.0.0/24", ""),
			},
			args: args{
				address: getCIDRNotationForAddress("10.0.0.4/24"),
				nwCfg: &cni.NetworkConfig{
					Ipam: ipamStruct{
						Address: "10.0.0.4/24",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error on v6 delete",
			fields: fields{
				plugin: &mockDelegatePlugin{
					del: del{
						err: errors.New("error on v6 delete"), //nolint:goerr113
					},
				},
				nwInfo: getNwInfo("10.0.0.0/24", "2001:db8:abcd:0012::0/64"),
			},
			args: args{
				address: getCIDRNotationForAddress("2001:db8:abcd:0015::0/64"),
				nwCfg: &cni.NetworkConfig{
					Ipam: ipamStruct{
						Address: "10.0.0.4/24",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &AzureIPAMInvoker{
				plugin: tt.fields.plugin,
				nwInfo: tt.fields.nwInfo,
			}
			err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.in2, tt.args.options)
			if tt.wantErr {
				require.NotNil(err)
				return
			}
			require.Nil(err)
		})
	}
}

func TestNewAzureIpamInvoker(t *testing.T) {
	NewAzureIpamInvoker(nil, nil)
}

func TestRemoveIpamState_Add(t *testing.T) {
	requires := require.New(t)
	type fields struct {
		plugin delegatePlugin
		nwInfo *network.NetworkInfo
	}
	type args struct {
		nwCfg        *cni.NetworkConfig
		in1          *cniSkel.CmdArgs
		subnetPrefix *net.IPNet
		options      map[string]interface{}
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		want       *cniTypesCurr.Result
		want1      *cniTypesCurr.Result
		wantErrMsg string
		wantErr    bool
	}{
		{
			name: "add ipv4 and delete IPAM state on ErrNoAvailableAddressPools",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getResult("10.0.0.1/24"),
						errv4:       ipam.ErrNoAvailableAddressPools,
					},
				},
				nwInfo: getNwInfo("10.0.0.0/24", ""),
			},
			args: args{
				nwCfg:        &cni.NetworkConfig{},
				subnetPrefix: getCIDRNotationForAddress("10.0.0.0/24"),
			},
			want:       getResult("10.0.0.1/24")[0],
			wantErrMsg: ipam.ErrNoAvailableAddressPools.Error(),
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &AzureIPAMInvoker{
				plugin: tt.fields.plugin,
				nwInfo: tt.fields.nwInfo,
			}

			_, err := invoker.Add(IPAMAddConfig{nwCfg: tt.args.nwCfg, args: tt.args.in1, options: tt.args.options})
			if tt.wantErr {
				requires.NotNil(err) // use NotNil since *cniTypes.Error is not of type Error
				requires.ErrorContains(err, tt.wantErrMsg)
			} else {
				requires.Nil(err)
			}
		})
	}
}
