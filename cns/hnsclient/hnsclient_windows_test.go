package hnsclient

import (
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Microsoft/hcsshim/hcn"
	"github.com/stretchr/testify/assert"
)

func TestAdhocAdjustIPConfig(t *testing.T) {
	tests := []struct {
		name     string
		ipConfig cns.IPConfiguration
		expected cns.IPConfiguration
	}{
		{
			name:     "expect no change when gw address is not 169.254.128.1",
			ipConfig: cns.IPConfiguration{GatewayIPAddress: "169.254.128.3"},
			expected: cns.IPConfiguration{GatewayIPAddress: "169.254.128.3"},
		},
		{
			name:     "expect default gw address is set when gw address is 169.254.128.1",
			ipConfig: cns.IPConfiguration{GatewayIPAddress: "169.254.128.1"},
			expected: cns.IPConfiguration{GatewayIPAddress: "169.254.128.2"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			updateGwForLocalIPConfiguration(&tt.ipConfig)
			assert.Equal(t, tt.expected.GatewayIPAddress, tt.ipConfig.GatewayIPAddress)
		})
	}
}

func TestEndpointLogDetails(t *testing.T) {
	endpoint := &hcn.HostComputeEndpoint{
		Id:                 "endpoint-id",
		Name:               "endpoint-name",
		HostComputeNetwork: "network-id",
		IpConfigurations: []hcn.IpConfig{{
			IpAddress:    "169.254.128.6",
			PrefixLength: 17,
		}},
		Dns: hcn.Dns{
			Domain:     "example.com",
			ServerList: []string{"10.0.0.2"},
		},
		Routes: []hcn.Route{{
			NextHop:           "169.254.128.1",
			DestinationPrefix: "0.0.0.0/0",
			Metric:            0,
		}},
		MacAddress: "00-15-5D-E7-DE-A0",
		Flags:      0,
	}

	assert.Equal(t,
		"ID: endpoint-id, Name: endpoint-name, Network: network-id, IpConfigurations: [{IpAddress:169.254.128.6 PrefixLength:17}], Dns: {Domain:example.com Search:[] ServerList:[10.0.0.2] Options:[]}, Routes: [{NextHop:169.254.128.1 DestinationPrefix:0.0.0.0/0 Metric:0}], MacAddress: 00-15-5D-E7-DE-A0, Flags: 0",
		endpointLogDetails(endpoint))
}
