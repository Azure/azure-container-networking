package middlewares

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/google/go-cmp/cmp"
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
	const policyType = "ACL"
	const action = "Block"
	const ingressDir = "In"
	const egressDir = "Out"
	const priority = 10000

	valueIn := []byte(fmt.Sprintf(`{
		"Type": "%s",
		"Action": "%s",
		"Direction": "%s",
		"Priority": %d
	}`,
		policyType,
		action,
		ingressDir,
		priority,
	))

	valueOut := []byte(fmt.Sprintf(`{
		"Type": "%s",
		"Action": "%s",
		"Direction": "%s",
		"Priority": %d
	}`,
		policyType,
		action,
		egressDir,
		priority,
	))

	expectedDefaultDenyEndpoint := []policy.Policy{
		{
			Type: policy.EndpointPolicy,
			Data: valueOut,
		},
		{
			Type: policy.EndpointPolicy,
			Data: valueIn,
		},
	}
	var allEndpoints []policy.Policy
	var defaultDenyEgressPolicy, defaultDenyIngressPolicy policy.Policy
	var err error

	defaultDenyEgressPolicy = mustGetEndpointPolicy("Out")
	defaultDenyIngressPolicy = mustGetEndpointPolicy("In")

	allEndpoints = append(allEndpoints, defaultDenyEgressPolicy, defaultDenyIngressPolicy)

	// Normalize both slices so there is no extra spacing, new lines, etc
	normalizedExpected := normalizeKVPairs(t, expectedDefaultDenyEndpoint)
	normalizedActual := normalizeKVPairs(t, allEndpoints)
	if !cmp.Equal(normalizedExpected, normalizedActual) {
		t.Error("received policy differs from expectation: diff", cmp.Diff(normalizedExpected, normalizedActual))
	}
	assert.Equal(t, err, nil)
}

// normalizeKVPairs normalizes the JSON values in the KV pairs by unmarshaling them into a map, then marshaling them back to compact JSON to remove any extra space, new lines, etc
func normalizeKVPairs(t *testing.T, policies []policy.Policy) []policy.Policy {
	normalized := make([]policy.Policy, len(policies))

	for i, kv := range policies {
		var unmarshaledValue map[string]interface{}
		// Unmarshal the Value into a map
		err := json.Unmarshal(kv.Data, &unmarshaledValue)
		require.NoError(t, err, "Failed to unmarshal JSON value")

		// Marshal it back to compact JSON
		normalizedValue, err := json.Marshal(unmarshaledValue)
		require.NoError(t, err, "Failed to re-marshal JSON value")

		// Replace Value with the normalized compact JSON
		normalized[i] = policy.Policy{
			Type: policy.EndpointPolicy,
			Data: normalizedValue,
		}
	}

	return normalized
}
