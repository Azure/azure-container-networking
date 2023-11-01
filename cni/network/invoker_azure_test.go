package network

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	"github.com/stretchr/testify/require"
)

const (
	ipv4cidr  = "10.0.0.1/24"
	v4NetCidr = "10.0.0.0/24"
	ipv4cidr2 = "10.0.0.4/24"
	ipv6cidr  = "2001:0db8:abcd:0015::0/64"
	v6NetCidr = "2001:db8:abcd:0012::0/64"
)

type mockDelegatePlugin struct {
	add
	del
}

type add struct {
	resultsIPv4      []*cniTypesCurr.Result
	resultsIPv6      []*cniTypesCurr.Result
	resultsIPv4Index int
	resultsIPv6Index int
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

func getSingleResult(ip string) []*cniTypesCurr.Result {
	return []*cniTypesCurr.Result{
		{
			IPs: []*cniTypesCurr.IPConfig{
				{
					Address: *getCIDRNotationForAddress(ip),
				},
			},
		},
	}
}

// getResult will return a slice of IPConfigs
func getResult(ips ...string) *cniTypesCurr.Result {
	res := &cniTypesCurr.Result{}
	for _, ip := range ips {
		res.IPs = append(res.IPs, &cniTypesCurr.IPConfig{Address: *getCIDRNotationForAddress(ip)})
	}
	return res
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
		name    string
		fields  fields
		args    args
		want    *cniTypesCurr.Result
		wantErr bool
	}{
		{
			name: "happy add ipv4",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getSingleResult(ipv4cidr),
					},
					del: del{},
				},
				nwInfo: getNwInfo(v4NetCidr, ""),
			},
			args: args{
				nwCfg:        &cni.NetworkConfig{},
				subnetPrefix: getCIDRNotationForAddress(v4NetCidr),
			},
			want:    getResult(ipv4cidr),
			wantErr: false,
		},
		{
			name: "happy add ipv4+ipv6",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getSingleResult(ipv4cidr),
						resultsIPv6: getSingleResult(ipv6cidr),
					},
				},
				nwInfo: getNwInfo(v4NetCidr, v6NetCidr),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{
					IPV6Mode: network.IPV6Nat,
				},
				subnetPrefix: getCIDRNotationForAddress(v4NetCidr),
			},
			want:    getResult(ipv4cidr, ipv6cidr),
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
				nwInfo: getNwInfo(v4NetCidr, ""),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error on ipv4+ipv6",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getSingleResult(ipv4cidr),
						errv6:       errors.New("test v6 error"), //nolint:goerr113
					},
				},
				nwInfo: getNwInfo(v4NetCidr, ""),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{
					IPV6Mode: network.IPV6Nat,
				},
				subnetPrefix: getCIDRNotationForAddress(v4NetCidr),
			},
			want:    getResult(ipv4cidr),
			wantErr: true,
		},
	}

	log.InitializeMock()

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

			fmt.Printf("want:%+v\nrest:%+v\n", tt.want, ipamAddResult.defaultInterfaceInfo.ipResult)
			require.Exactly(tt.want, ipamAddResult.defaultInterfaceInfo.ipResult)
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
				nwInfo: getNwInfo(v4NetCidr, ""),
			},
			args: args{
				address: getCIDRNotationForAddress(ipv4cidr2),
				nwCfg: &cni.NetworkConfig{
					IPAM: cni.IPAM{
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
				nwInfo: getNwInfo(v4NetCidr, v6NetCidr),
			},
			args: args{
				address: getCIDRNotationForAddress("2001:db8:abcd:0015::0/64"),
				nwCfg: &cni.NetworkConfig{
					IPAM: cni.IPAM{
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
				nwInfo: getNwInfo("", v6NetCidr),
			},
			args: args{
				address: nil,
				nwCfg: &cni.NetworkConfig{
					IPAM: cni.IPAM{
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
				nwInfo: getNwInfo(v4NetCidr, ""),
			},
			args: args{
				address: getCIDRNotationForAddress(ipv4cidr2),
				nwCfg: &cni.NetworkConfig{
					IPAM: cni.IPAM{
						Address: ipv4cidr2,
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
				nwInfo: getNwInfo(v4NetCidr, v6NetCidr),
			},
			args: args{
				address: getCIDRNotationForAddress("2001:db8:abcd:0015::0/64"),
				nwCfg: &cni.NetworkConfig{
					IPAM: cni.IPAM{
						Address: ipv4cidr2,
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
						resultsIPv4: getSingleResult(ipv4cidr),
						errv4:       ipam.ErrNoAvailableAddressPools,
					},
				},
				nwInfo: getNwInfo(v4NetCidr, ""),
			},
			args: args{
				nwCfg:        &cni.NetworkConfig{},
				subnetPrefix: getCIDRNotationForAddress(v4NetCidr),
			},
			want:       getResult(ipv4cidr),
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
