// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package state

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	v1alpha "github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNetworkContainerRecord(t *testing.T) {
	request := completeNetworkContainerRequest()
	request.NetworkContainerid = "old-id"
	request.AuthorizationToken = testSecretToken
	request.SecondaryIPConfigs = map[string]cns.SecondaryIPConfig{
		testIPID1: {IPAddress: testIPv4Address, NCVersion: 2},
	}

	record := NewNetworkContainerRecord(testNCID, "2", "1", true, request)

	expectedRequest := request
	expectedRequest.NetworkContainerid = testNCID
	expectedRequest.AuthorizationToken = ""
	expectedRequest.SecondaryIPConfigs = nil
	assert.Equal(t, NetworkContainerRecord{
		ID:                testNCID,
		VMVersion:         "2",
		HostVersion:       "1",
		VFPUpdateComplete: true,
		Request:           expectedRequest,
	}, record)
	assert.Equal(t, "old-id", request.NetworkContainerid)
	assert.Equal(t, testSecretToken, request.AuthorizationToken)
	assert.NotNil(t, request.SecondaryIPConfigs)
}

func TestNetworkContainerRecordJSONRoundTrip(t *testing.T) {
	input := NewNetworkContainerRecord(testNCID, "2", "1", true, completeNetworkContainerRequest())

	//nolint:musttag // CreateNetworkContainerRequest intentionally retains its legacy untagged JSON field names.
	data, err := json.Marshal(input)
	require.NoError(t, err)
	var got NetworkContainerRecord
	require.NoError(t, decodeJSONValue(data, &got))
	assert.Equal(t, input, got)
	assert.Equal(t, v1alpha.NCUpdateSuccess, got.Request.NCStatus)
	assert.NotEmpty(t, got.Request.IPv6Configuration.IPSubnetV6.IPAddress)
	assert.NotEmpty(t, got.Request.EndpointPolicies)
}

func TestIPAndEndpointRecordsJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input any
		run   func([]byte) any
	}{
		{
			name: "IP record",
			input: IPRecord{
				ID:        testIPID1,
				IPAddress: testIPv4Address,
				NCID:      testNCID,
				NCVersion: 3,
			},
			run: func(data []byte) any {
				var got IPRecord
				require.NoError(t, decodeJSONValue(data, &got))
				return got
			},
		},
		{
			name:  "dual stack multi-NIC endpoint",
			input: completeEndpointRecord(),
			run: func(data []byte) any {
				var got EndpointRecord
				require.NoError(t, decodeJSONValue(data, &got))
				return got
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.input, tt.run(data))
		})
	}
}

func completeNetworkContainerRequest() cns.CreateNetworkContainerRequest {
	return cns.CreateNetworkContainerRequest{
		HostPrimaryIP:              "192.0.2.10",
		Version:                    "7",
		NetworkContainerType:       "VNet",
		NetworkContainerid:         testNCID,
		PrimaryInterfaceIdentifier: "192.0.2.11",
		LocalIPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "192.0.2.10",
				PrefixLength: 24,
			},
			DNSServers:       []string{"168.63.129.16"},
			GatewayIPAddress: "192.0.2.1",
		},
		OrchestratorContext: json.RawMessage(`{"podName":"pod-1","podNamespace":"ns-1"}`),
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    "10.0.0.0",
				PrefixLength: 24,
			},
			DNSServers:       []string{"10.0.0.10"},
			GatewayIPAddress: testGatewayAddress,
		},
		IPv6Configuration: cns.IPConfiguration{
			IPSubnetV6: cns.IPSubnet{
				IPAddress:    "fd00::",
				PrefixLength: 64,
			},
			DNSServers:         []string{"fd00::10"},
			GatewayIPv6Address: "fd00::1",
		},
		MultiTenancyInfo: cns.MultiTenancyInfo{
			EncapType: cns.Vlan,
			ID:        101,
		},
		CnetAddressSpace: []cns.IPSubnet{{
			IPAddress:    "10.240.0.0",
			PrefixLength: 16,
		}},
		Routes: []cns.Route{{
			IPAddress:        "0.0.0.0/0",
			GatewayIPAddress: testGatewayAddress,
			InterfaceToUse:   testEth0,
		}},
		AllowHostToNCCommunication: true,
		AllowNCToHostCommunication: true,
		SkipDefaultRoutes:          true,
		EndpointPolicies: []cns.NetworkContainerRequestPolicies{{
			Type:         "ACL",
			EndpointType: "L2",
			Settings:     json.RawMessage(`{"action":"allow"}`),
		}},
		NCStatus: v1alpha.NCUpdateSuccess,
		NetworkInterfaceInfo: cns.NetworkInterfaceInfo{
			NICType:    cns.InfraNIC,
			MACAddress: "00:11:22:33:44:55",
		},
	}
}

func completeEndpointRecord() EndpointRecord {
	return EndpointRecord{
		PodName:      testPodName,
		PodNamespace: testPodNamespace,
		IfnameToIPMap: map[string]*IPInfoRecord{
			testEth0: {
				IPv4: []net.IPNet{{
					IP:   net.ParseIP(testIPv4Address),
					Mask: net.CIDRMask(24, 32),
				}},
				IPv6: []net.IPNet{{
					IP:   net.ParseIP("fd00::4"),
					Mask: net.CIDRMask(64, 128),
				}},
				HNSEndpointID:      "hns-endpoint-1",
				HNSNetworkID:       "hns-network-1",
				HostVethName:       "veth-1",
				MACAddress:         "00:11:22:33:44:55",
				NetworkContainerID: testNCID,
				NICType:            cns.InfraNIC,
			},
			"net1": {
				IPv4: []net.IPNet{{
					IP:   net.ParseIP("10.1.0.4"),
					Mask: net.CIDRMask(24, 32),
				}},
				HNSEndpointID:      "hns-endpoint-2",
				HNSNetworkID:       "hns-network-2",
				HostVethName:       "veth-2",
				MACAddress:         "00:11:22:33:44:66",
				NetworkContainerID: testNCID,
				NICType:            cns.DelegatedVMNIC,
			},
		},
	}
}
