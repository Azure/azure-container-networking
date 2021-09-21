//+build windows

package network

import (
	"fmt"
	"github.com/Azure/azure-container-networking/cns"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test windows network policies is set
func TestAddWithRunTimeNetPolicies(t *testing.T) {
	_, ipnetv4, _ := net.ParseCIDR("10.240.0.0/12")
	_, ipnetv6, _ := net.ParseCIDR("fc00::/64")

	tests := []struct {
		name       string
		nwInfo     network.NetworkInfo
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "add ipv6 endpoint policy",
			nwInfo: network.NetworkInfo{
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
			p, err := addIPV6EndpointPolicy(tt.nwInfo)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Condition(t, assert.Comparison(func() bool { return p.Type == policy.EndpointPolicy }))
			}
		})
	}
}

func TestPluginSecondAddSamePodWindows(t *testing.T) {
	plugin, _ := cni.NewPlugin("name", "0.3.0")

	tests := []struct {
		name       string
		methods    []string
		cniArgs    skel.CmdArgs
		plugin     *NetPlugin
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "CNI consecutive add already hot attached",
			methods: []string{"ADD", "ADD"},
			cniArgs: skel.CmdArgs{
				ContainerID: "test1-container",
				Netns:       "test1-container",
				StdinData:   nwCfg.Serialize(),
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container1", "container1-ns"),
				IfName:      eth0IfName,
			},
			plugin: &NetPlugin{
				Plugin:            plugin,
				nm:                network.NewMockNetworkmanager(),
				ipamInvoker:       NewMockIpamInvoker(false, false, false),
				hnsEndpointClient: network.NewMockHNSEndpoint(true, false),
				report:            &telemetry.CNIReport{},
				tb:                &telemetry.TelemetryBuffer{},
			},
			wantErr: false,
		},
		{
			name:    "CNI consecutive add not hot attached",
			methods: []string{"ADD", "ADD"},
			cniArgs: skel.CmdArgs{
				ContainerID: "test1-container",
				Netns:       "test1-container",
				StdinData:   nwCfg.Serialize(),
				Args:        fmt.Sprintf("K8S_POD_NAME=%v;K8S_POD_NAMESPACE=%v", "container1", "container1-ns"),
				IfName:      eth0IfName,
			},
			plugin: &NetPlugin{
				Plugin:            plugin,
				nm:                network.NewMockNetworkmanager(),
				ipamInvoker:       NewMockIpamInvoker(false, false, false),
				hnsEndpointClient: network.NewMockHNSEndpoint(false, false),
				report:            &telemetry.CNIReport{},
				tb:                &telemetry.TelemetryBuffer{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var err error
			for _, method := range tt.methods {
				if method == "ADD" {
					err = tt.plugin.Add(&tt.cniArgs)
				}
			}

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				endpoints, _ := tt.plugin.nm.GetAllEndpoints(nwCfg.Name)
				require.Condition(t, assert.Comparison(func() bool { return len(endpoints) == 1 }), "Expected 2 but got %v", len(endpoints))
			}
		})
	}
}

func TestSetNetworkOptions(t *testing.T) {
	tests := []struct {
		name           string
		cnsNwConfig    cns.GetNetworkContainerResponse
		nwInfo         network.NetworkInfo
		expectedVlanID string
	}{
		{
			name: "set network options vlanid test",
			cnsNwConfig: cns.GetNetworkContainerResponse{
				MultiTenancyInfo: cns.MultiTenancyInfo{
					ID: 1,
				},
			},
			nwInfo: network.NetworkInfo{
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
					cns.IPSubnet{
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
		name  string
		nwCfg cni.NetworkConfig
	}{
		{
			name: "Runtime network polices",
			nwCfg: cni.NetworkConfig{
				RuntimeConfig: cni.RuntimeConfig{
					PortMappings: []cni.PortMapping{
						cni.PortMapping{
							Protocol:      "tcp",
							HostIp:        "19.268.0.4",
							HostPort:      8000,
							ContainerPort: 80,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			policies := getPoliciesFromRuntimeCfg(&tt.nwCfg)
			require.Condition(t, assert.Comparison(func() bool {
				return len(policies) > 0 && policies[0].Type == policy.EndpointPolicy
			}))
		})
	}
}
