package network

import (
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	cniTypes "github.com/containernetworking/cni/pkg/types"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
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
	err              error
}

func (d *add) DelegateAdd(pluginName string, nwCfg *cni.NetworkConfig) (*cniTypesCurr.Result, error) {
	if d.err != nil {
		return nil, d.err
	}

	if pluginName == ipamV6 {
		if d.resultsIPv6 == nil || d.resultsIPv6Index-1 > len(d.resultsIPv6) {
			return nil, errors.New("no more ipv6 results in mock available")
		}
		res := d.resultsIPv6[d.resultsIPv6Index]
		d.resultsIPv6Index++
		return res, nil
	}
	if d.resultsIPv4 == nil || d.resultsIPv4Index-1 > len(d.resultsIPv4) {
		return nil, errors.New("no more ipv4 results in mock available")
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
	return nil
}

func getCIDRNotationForAddress(t *testing.T, ipaddresswithcidr string) *net.IPNet {
	ip, ipnet, err := net.ParseCIDR(ipaddresswithcidr)
	require.NoError(t, err)
	ipnet.IP = ip
	return ipnet
}

func getResult(t *testing.T, ip string) []*cniTypesCurr.Result {
	res := []*cniTypesCurr.Result{
		{
			IPs: []*cniTypesCurr.IPConfig{
				{
					Address: *getCIDRNotationForAddress(t, "10.0.0.5/24"),
				},
			},
		},
	}
	return res
}

type ipamStruct struct {
	Type          string `json:"type"`
	Environment   string `json:"environment,omitempty"`
	AddrSpace     string `json:"addressSpace,omitempty"`
	Subnet        string `json:"subnet,omitempty"`
	Address       string `json:"ipAddress,omitempty"`
	QueryInterval string `json:"queryInterval,omitempty"`
}

func getNwInfo(t *testing.T, subnetv4, subnetv6 string) *network.NetworkInfo {
	nwinfo := &network.NetworkInfo{
		Subnets: []network.SubnetInfo{
			{
				Prefix: *getCIDRNotationForAddress(t, subnetv4),
			},
			{},
		},
	}
	if subnetv6 != "" {
		nwinfo.Subnets = append(nwinfo.Subnets, network.SubnetInfo{
			Prefix: *getCIDRNotationForAddress(t, subnetv6),
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
		want1   *cniTypesCurr.Result
		wantErr bool
	}{
		{
			name: "test happy add ipv4",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getResult(t, "10.0.0.1/24"),
					},
					del: del{},
				},
				nwInfo: getNwInfo(t, "10.0.0.0/24", ""),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{
					Ipam: ipamStruct{},
				},
				subnetPrefix: getCIDRNotationForAddress(t, "10.0.0.0/24"),
			},
			want:    getResult(t, "10.0.0.1/24")[0],
			wantErr: false,
		},
		{
			name: "test happy add ipv4+ipv6",
			fields: fields{
				plugin: &mockDelegatePlugin{
					add: add{
						resultsIPv4: getResult(t, "10.0.0.1/24"),
						resultsIPv6: getResult(t, "2001:0DB8:ABCD:0015::::/64"),
					},
				},
				nwInfo: getNwInfo(t, "10.0.0.0/24", "2001:db8:abcd:0012::0/64"),
			},
			args: args{
				nwCfg: &cni.NetworkConfig{
					Ipam:     ipamStruct{},
					IPV6Mode: network.IPV6Nat,
				},
				subnetPrefix: getCIDRNotationForAddress(t, "2001:db8:abcd:0012::0/64"),
			},
			want:    getResult(t, "10.0.0.1/24")[0],
			want1:   getResult(t, "2001:0DB8:ABCD:0015::::/64")[0],
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			invoker := &AzureIPAMInvoker{
				plugin: tt.fields.plugin,
				nwInfo: tt.fields.nwInfo,
			}
			got, got1, err := invoker.Add(tt.args.nwCfg, tt.args.in1, tt.args.subnetPrefix, tt.args.options)
			if tt.wantErr {
				require.NotNil(err) // use NotNil since *cniTypes.Error is not of type Error
			} else {
				require.Nil(err)
			}

			require.Exactly(tt.want, got)
			require.Exactly(tt.want1, got1)
		})
	}
}

func TestAzureIPAMInvoker_Delete(t *testing.T) {
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
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &AzureIPAMInvoker{
				plugin: tt.fields.plugin,
				nwInfo: tt.fields.nwInfo,
			}
			if err := invoker.Delete(tt.args.address, tt.args.nwCfg, tt.args.in2, tt.args.options); (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAzureIpamInvoker(t *testing.T) {
	type args struct {
		plugin *netPlugin
		nwInfo *network.NetworkInfo
	}
	tests := []struct {
		name string
		args args
		want *AzureIPAMInvoker
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewAzureIpamInvoker(tt.args.plugin, tt.args.nwInfo); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewAzureIpamInvoker() = %v, want %v", got, tt.want)
			}
		})
	}
}
