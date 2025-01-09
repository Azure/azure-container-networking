package middlewares

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

func TestSetRoutesSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	podIPInfo := []cns.PodIpInfo{
		{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    "10.0.1.10",
				PrefixLength: 32,
			},
			NICType: cns.InfraNIC,
		},
		{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    "20.240.1.242",
				PrefixLength: 32,
			},
			NICType:    cns.DelegatedVMNIC,
			MacAddress: "12:34:56:78:9a:bc",
		},
	}
	for i := range podIPInfo {
		ipInfo := &podIPInfo[i]
		err := middleware.setRoutes(ipInfo)
		assert.Equal(t, err, nil)
		if ipInfo.NICType == cns.InfraNIC {
			assert.Equal(t, ipInfo.SkipDefaultRoutes, true)
		} else {
			assert.Equal(t, ipInfo.SkipDefaultRoutes, false)
		}
	}
}

func TestAssignSubnetPrefixSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	podIPInfo := cns.PodIpInfo{
		PodIPConfig: cns.IPSubnet{
			IPAddress:    "20.240.1.242",
			PrefixLength: 32,
		},
		NICType:    cns.DelegatedVMNIC,
		MacAddress: "12:34:56:78:9a:bc",
	}

	intInfo := v1alpha1.InterfaceInfo{
		GatewayIP:          "20.240.1.1",
		SubnetAddressSpace: "20.240.1.0/16",
	}

	ipInfo := podIPInfo
	err := middleware.assignSubnetPrefixLengthFields(&ipInfo, intInfo, ipInfo.PodIPConfig.IPAddress)
	assert.Equal(t, err, nil)
	// assert that the function for windows modifies all the expected fields with prefix-length
	assert.Equal(t, ipInfo.PodIPConfig.PrefixLength, uint8(16))
	assert.Equal(t, ipInfo.HostPrimaryIPInfo.Gateway, intInfo.GatewayIP)
	assert.Equal(t, ipInfo.HostPrimaryIPInfo.Subnet, intInfo.SubnetAddressSpace)
}

func TestAddDefaultRoute(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	podIPInfo := cns.PodIpInfo{
		PodIPConfig: cns.IPSubnet{
			IPAddress:    "20.240.1.242",
			PrefixLength: 32,
		},
		NICType:    cns.DelegatedVMNIC,
		MacAddress: "12:34:56:78:9a:bc",
	}

	gatewayIP := "20.240.1.1"
	intInfo := v1alpha1.InterfaceInfo{
		GatewayIP:          gatewayIP,
		SubnetAddressSpace: "20.240.1.0/16",
	}

	ipInfo := podIPInfo
	middleware.addDefaultRoute(&ipInfo, intInfo.GatewayIP)

	expectedRoutes := []cns.Route{
		{
			IPAddress:        "0.0.0.0/0",
			GatewayIPAddress: gatewayIP,
		},
	}

	if !reflect.DeepEqual(ipInfo.Routes, expectedRoutes) {
		t.Errorf("got '%+v', expected '%+v'", ipInfo.Routes, expectedRoutes)
	}
}

func TestAddDefaultDenyACL(t *testing.T) {
	valueOut := []byte(`{
		"Type": "ACL",
		"Action": "Block",
		"Direction": "Out",
		"Priority": 10000
	}`)

	valueIn := []byte(`{
		"Type": "ACL",
		"Action": "Block",
		"Direction": "In",
		"Priority": 10000
	}`)

	expectedDefaultDenyACL := []cni.KVPair{
		{
			Name:  "EndpointPolicy",
			Value: valueOut,
		},
		{
			Name:  "EndpointPolicy",
			Value: valueIn,
		},
	}

	podIPInfo := cns.PodIpInfo{
		PodIPConfig: cns.IPSubnet{
			IPAddress:    "20.240.1.242",
			PrefixLength: 32,
		},
		NICType:    cns.DelegatedVMNIC,
		MacAddress: "12:34:56:78:9a:bc",
	}

	err := addDefaultDenyACL(&podIPInfo)
	assert.Equal(t, err, nil)

	// Normalize both slices so there is no extra spacing, new lines, etc
	normalizedExpected := normalizeKVPairs(t, expectedDefaultDenyACL)
	normalizedActual := normalizeKVPairs(t, podIPInfo.DefaultDenyACL)
	if !reflect.DeepEqual(normalizedExpected, normalizedActual) {
		t.Errorf("got '%+v', expected '%+v'", podIPInfo.DefaultDenyACL, expectedDefaultDenyACL)
	}
}

// normalizeKVPairs normalizes the JSON values in the KV pairs by unmarshaling them into a map, then marshaling them back to compact JSON to remove any extra space, new lines, etc
func normalizeKVPairs(t *testing.T, kvPairs []acn.KVPair) []cni.KVPair {
	normalized := make([]cni.KVPair, len(kvPairs))

	for i, kv := range kvPairs {
		var unmarshaledValue map[string]interface{}
		// Unmarshal the Value into a map
		err := json.Unmarshal(kv.Value, &unmarshaledValue)
		require.NoError(t, err, "Failed to unmarshal JSON value")

		// Marshal it back to compact JSON
		normalizedValue, err := json.Marshal(unmarshaledValue)
		require.NoError(t, err, "Failed to re-marshal JSON value")

		// Replace Value with the normalized compact JSON
		normalized[i] = acn.KVPair{
			Name:  kv.Name,
			Value: normalizedValue,
		}
	}

	return normalized
}
