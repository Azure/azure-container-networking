package middlewares

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/network/policy"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMain(m *testing.M) {
	logger.InitLogger("testlogs", 0, 0, "./")
	os.Exit(m.Run())
}

func TestIPConfigsRequestHandlerWrapperScheduledWithDRA(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.0/24")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.0/16")

	defaultHandler := func(context.Context, cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		return &cns.IPConfigsResponse{
			PodIPInfo: []cns.PodIpInfo{
				{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    "10.0.1.10",
						PrefixLength: 32,
					},
					NICType: cns.InfraNIC,
				},
			},
		}, nil
	}
	failureHandler := func(context.Context, cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		return nil, nil
	}
	podInfo := cns.NewPodInfo(
		"5006cad4-eth0",
		"5006cad4-e54d-472e-863d-c4bac66200a7",
		"testpod12",
		"testpod12namespace",
	)
	req := cns.IPConfigsRequest{
		PodInterfaceID:   podInfo.InterfaceID(),
		InfraContainerID: podInfo.InfraContainerID(),
	}
	req.OrchestratorContext, _ = podInfo.OrchestratorContext()

	resp, err := middleware.IPConfigsRequestHandlerWrapper(defaultHandler, failureHandler)(context.TODO(), req)

	require.NoError(t, err)
	require.Len(t, resp.PodIPInfo, 1)
	require.Equal(t, cns.InfraNIC, resp.PodIPInfo[0].NICType)
	require.True(t, resp.PodIPInfo[0].SkipDefaultRoutes)
	require.True(t, resp.PodConfigurations.SkipDefaultRouteProgramming)
}

func TestGetSwiftV2IPConfigForDRANET(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	podInfo := cns.NewPodInfo(
		"5006cad4-eth0",
		"5006cad4-e54d-472e-863d-c4bac66200a7",
		"testpod12",
		"testpod12namespace",
	)

	result, err := middleware.getSwiftV2IpConfigHelper(context.TODO(), podInfo, true)

	require.NoError(t, err)
	require.Len(t, result.podIPInfos, 1)
	require.False(t, result.podConfigurations.SkipDefaultRouteProgramming)
}

func TestSetRoutesSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.10/16")
	t.Setenv(configuration.EnvPodCIDRs, "10.1.0.10/24") // make sure windows swiftv2 does not set pod cidr route

	podIPInfo := []cns.PodIpInfo{
		{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    "10.0.1.100",
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

	desiredPodIPInfo := []cns.PodIpInfo{
		{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    "10.0.1.100",
				PrefixLength: 32,
			},
			NICType: cns.InfraNIC,
			Routes: []cns.Route{
				{
					IPAddress:        "10.0.0.0/16",
					GatewayIPAddress: "0.0.0.0",
				},
				{
					IPAddress:        "10.240.0.10/16",
					GatewayIPAddress: "0.0.0.0",
				},
				{
					IPAddress:        "0.0.0.0/0",
					GatewayIPAddress: "0.0.0.0",
				},
			},
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

	// check if the routes are set as expected
	reflect.DeepEqual(podIPInfo[0].Routes, desiredPodIPInfo[0].Routes)
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

// TestApplyDefaultDenyToInfraNIC covers the prototype path that forces SwiftV2-style
// default-deny ACLs on the InfraNIC of a non-SwiftV2 pod. Label detection is handled
// upstream during request validation, so this function unconditionally decorates the
// InfraNIC entry.
func TestApplyDefaultDenyToInfraNIC(t *testing.T) {
	const (
		ns   = "default"
		name = "labelled-pod"
	)
	podInfo := cns.NewPodInfo("cid", "iid", name, ns)

	t.Run("applies deny ACLs to InfraNIC", func(t *testing.T) {
		resp := &cns.IPConfigsResponse{
			PodIPInfo: []cns.PodIpInfo{
				{NICType: cns.InfraNIC, PodIPConfig: cns.IPSubnet{IPAddress: "10.0.0.5", PrefixLength: 24}},
			},
		}
		applyDefaultDenyToInfraNIC(podInfo, resp)

		require.Len(t, resp.PodIPInfo[0].EndpointPolicies, 2)
		normalized := normalizeKVPairs(t, resp.PodIPInfo[0].EndpointPolicies)
		expected := normalizeKVPairs(t, []policy.Policy{defaultDenyEgressPolicy, defaultDenyIngressPolicy})
		require.True(t, cmp.Equal(expected, normalized), "applied policies differ: %s", cmp.Diff(expected, normalized))
	})

	t.Run("empty response is a no-op", func(t *testing.T) {
		resp := &cns.IPConfigsResponse{}
		applyDefaultDenyToInfraNIC(podInfo, resp)
		require.Empty(t, resp.PodIPInfo)
	})
}

// TestPodHasDefaultDenyLabel verifies the label detection used during request
// validation to decide whether a non-SwiftV2 pod opts into default-deny ACLs.
// The label value is parsed with strconv.ParseBool; an unparseable value returns
// false and an error.
func TestPodHasDefaultDenyLabel(t *testing.T) {
	labelledPod := func(val string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{configuration.LabelPodDefaultDeny: val},
			},
		}
	}

	enabled, err := defaultDenyEnabledOnPod(labelledPod("true"))
	require.NoError(t, err)
	require.True(t, enabled)

	enabled, err = defaultDenyEnabledOnPod(labelledPod("True"))
	require.NoError(t, err)
	require.True(t, enabled)

	enabled, err = defaultDenyEnabledOnPod(labelledPod("1"))
	require.NoError(t, err)
	require.True(t, enabled)

	enabled, err = defaultDenyEnabledOnPod(labelledPod("false"))
	require.NoError(t, err)
	require.False(t, enabled)

	enabled, err = defaultDenyEnabledOnPod(labelledPod("0"))
	require.NoError(t, err)
	require.False(t, enabled)

	// Present but unparseable value returns false and an error.
	enabled, err = defaultDenyEnabledOnPod(labelledPod("notabool"))
	require.Error(t, err)
	require.False(t, enabled)

	enabled, err = defaultDenyEnabledOnPod(labelledPod(""))
	require.Error(t, err)
	require.False(t, enabled)

	// Absent label is opt-out with no error.
	enabled, err = defaultDenyEnabledOnPod(corev1.Pod{ObjectMeta: metav1.ObjectMeta{}})
	require.NoError(t, err)
	require.False(t, enabled)
}
