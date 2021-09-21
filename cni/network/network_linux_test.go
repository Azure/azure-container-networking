//+build linux

package network

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/network"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetNetworkOptions(t *testing.T) {
	tests := []struct {
		name             string
		cnsNwConfig      cns.GetNetworkContainerResponse
		nwInfo           network.NetworkInfo
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
			nwInfo: network.NetworkInfo{
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
		result current.Result
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
		result current.Result
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
