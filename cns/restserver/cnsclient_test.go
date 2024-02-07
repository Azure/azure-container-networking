package restserver

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/client"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	podnametest      = "testpodname"
	podnamespacetest = "testpodnamespace"
	ncid             = "440515e4-bf48-4997-8e4c-d650d29b462d"
)

var dnsServers = []string{"8.8.8.8", "8.8.4.4"}

func setUpRestserver() {
	config := common.ServiceConfig{}

	httpRestService, err := NewHTTPRestService(&config, &fakes.WireserverClientFake{}, &fakes.WireserverProxyFake{}, &fakes.NMAgentClientFake{}, nil, nil, nil)
	httpRestService.Name = "cns-test-server"
	fakeNNC := v1alpha.NodeNetworkConfig{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: 16,
			IPsNotInUse:      []string{"abc"},
		},
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               10,
				ReleaseThresholdPercent: 150,
				RequestThresholdPercent: 50,
				MaxIPCount:              250,
			},
			NetworkContainers: []v1alpha.NetworkContainer{
				{
					ID:         "nc1",
					PrimaryIP:  "10.0.0.11",
					SubnetName: "sub1",
					IPAssignments: []v1alpha.IPAssignment{
						{
							Name: "ip1",
							IP:   "10.0.0.10",
						},
					},
					DefaultGateway:     "10.0.0.1",
					SubnetAddressSpace: "10.0.0.0/24",
					Version:            2,
				},
			},
		},
	}
	httpRestService.IPAMPoolMonitor = &fakes.MonitorFake{IPsNotInUseCount: 13, NodeNetworkConfig: &fakeNNC}

	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
		return
	}

	svc = httpRestService
	svc = service.(*HTTPRestService)
	svc.state.OrchestratorType = cns.KubernetesCRD
	svc.IPAMPoolMonitor = &fakes.MonitorFake{IPsNotInUseCount: 13, NodeNetworkConfig: &fakeNNC}
}

func TestCNSClientRequestAndRelease(t *testing.T) {
	desiredIPAddress := primaryIP
	ip := net.ParseIP(desiredIPAddress)
	_, ipnet, _ := net.ParseCIDR("10.0.0.5/24")
	desired := net.IPNet{
		IP:   ip,
		Mask: ipnet.Mask,
	}

	secondaryIps := make([]string, 0)
	secondaryIps = append(secondaryIps, desiredIPAddress)
	cnsClient, _ := client.New("", 2*time.Hour)
	setUpRestserver()
	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.NewPodInfo("some-guid-1", "abc-eth0", podnametest, podnamespacetest)
	orchestratorContext, err := json.Marshal(podInfo)
	require.NoError(t, err)

	// no IP reservation found with that context, expect no failure.
	err = cnsClient.ReleaseIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	require.NoError(t, err, "Release ip idempotent call failed")

	// request IP address
	resp, err := cnsClient.RequestIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	require.NoError(t, err, "get IP from CNS failed")

	podIPInfo := resp.PodIpInfo
	assert.Equal(t, primaryIP, podIPInfo.NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress, "PrimaryIP is not added as epected ipConfig")
	assert.EqualValues(t, podIPInfo.NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength, subnetPrfixLength, "Primary IP Prefix length is not added as expected ipConfig")

	// validate DnsServer and Gateway Ip as the same configured for Primary IP
	assert.Equal(t, dnsServers, podIPInfo.NetworkContainerPrimaryIPConfig.DNSServers, "DnsServer is not added as expected ipConfig")
	assert.Equal(t, gatewayIP, podIPInfo.NetworkContainerPrimaryIPConfig.GatewayIPAddress, "Gateway is not added as expected ipConfig")

	resultIPnet, err := getIPNetFromResponse(resp)
	require.NoError(t, err, "getIPNetFromResponse failed")

	assert.Equal(t, desired, resultIPnet, "Desired result not matching actual result")

	// checking for assigned IP address and pod context printing before ReleaseIPAddress is called
	ipaddresses, err := cnsClient.GetIPAddressesMatchingStates(context.TODO(), types.Assigned)
	require.NoError(t, err, "Get assigned IP addresses failed")

	assert.Len(t, ipaddresses, 1, "Number of available IP addresses expected to be 1")
	assert.Equal(t, desiredIPAddress, ipaddresses[0].IPAddress, "Available IP address does not match expected, address state")
	assert.Equal(t, types.Assigned, ipaddresses[0].GetState(), "Available IP address does not match expected, address state")

	t.Log(ipaddresses)

	addresses := make([]string, len(ipaddresses))
	for i := range ipaddresses {
		addresses[i] = ipaddresses[i].IPAddress
	}

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(context.TODO(), cns.IPConfigRequest{DesiredIPAddress: ipaddresses[0].IPAddress, OrchestratorContext: orchestratorContext})
	require.NoError(t, err, "Expected to not fail when releasing IP reservation found with context")
	service.Stop()
}

func TestCNSClientPodContextApi(t *testing.T) {
	desiredIPAddress := primaryIP
	secondaryIps := []string{desiredIPAddress}
	cnsClient, _ := client.New("", 2*time.Second)

	setUpRestserver()
	addTestStateToRestServer(t, secondaryIps)

	podInfo := cns.NewPodInfo("some-guid-1", "abc-eth0", podnametest, podnamespacetest)
	orchestratorContext, err := json.Marshal(podInfo)
	require.NoError(t, err)

	// request IP address
	_, err = cnsClient.RequestIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	require.NoError(t, err, "get IP from CNS failed")

	// test for pod ip by orch context map
	podcontext, err := cnsClient.GetPodOrchestratorContext(context.TODO())
	require.NoError(t, err, "Get pod ip by orchestrator context failed")
	assert.NotEmpty(t, podcontext, "Expected at least 1 entry in map for podcontext")
	t.Log(podcontext)

	// release requested IP address, expect success
	err = cnsClient.ReleaseIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	require.NoError(t, err, "Expected to not fail when releasing IP reservation found with context")
	service.Stop()
}

func TestCNSClientDebugAPI(t *testing.T) {
	desiredIPAddress := primaryIP

	secondaryIPs := []string{desiredIPAddress}
	cnsClient, _ := client.New("", 2*time.Hour)

	setUpRestserver()
	addTestStateToRestServer(t, secondaryIPs)

	podInfo := cns.NewPodInfo("some-guid-1", "abc-eth0", podnametest, podnamespacetest)
	orchestratorContext, err := json.Marshal(podInfo)
	require.NoError(t, err)

	// request IP address
	_, err1 := cnsClient.RequestIPAddress(context.TODO(), cns.IPConfigRequest{OrchestratorContext: orchestratorContext})
	require.NoError(t, err1, "get IP from CNS failed")

	// test for debug api/cmd to get inmemory data from HTTPRestService
	inmemory, err := cnsClient.GetHTTPServiceData(context.TODO())
	require.NoError(t, err, "Get in-memory http REST Struct failed")
	assert.NotEmpty(t, inmemory.HTTPRestServiceData.PodIPIDByPodInterfaceKey, "OrchestratorContext map is expected but not returned")

	// testing Pod IP Configuration Status values set for test
	podConfig := inmemory.HTTPRestServiceData.PodIPConfigState
	for _, v := range podConfig {
		assert.Equal(t, primaryIP, v.IPAddress, "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
		assert.Equal(t, types.Assigned, v.GetState(), "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
		assert.Equal(t, ncid, v.NCID, "Not the expected set values for testing IPConfigurationStatus, %+v", podConfig)
	}
	assert.NotEmpty(t, inmemory.HTTPRestServiceData.PodIPConfigState, "PodIpConfigState with at least 1 entry expected")

	testIpamPoolMonitor := inmemory.HTTPRestServiceData.IPAMPoolMonitor
	assert.EqualValues(t, 5, testIpamPoolMonitor.MinimumFreeIps, "IPAMPoolMonitor state is not reflecting the initial set values")
	assert.EqualValues(t, 15, testIpamPoolMonitor.MaximumFreeIps, "IPAMPoolMonitor state is not reflecting the initial set values")
	assert.EqualValues(t, 13, testIpamPoolMonitor.UpdatingIpsNotInUseCount, "IPAMPoolMonitor state is not reflecting the initial set values")

	// check for cached NNC Spec struct values
	assert.EqualValues(t, 16, testIpamPoolMonitor.CachedNNC.Spec.RequestedIPCount, "IPAMPoolMonitor cached NNC Spec is not reflecting the initial set values")
	assert.Len(t, testIpamPoolMonitor.CachedNNC.Spec.IPsNotInUse, 1, "IPAMPoolMonitor cached NNC Spec is not reflecting the initial set values")

	// check for cached NNC Status struct values
	assert.EqualValues(t, 10, testIpamPoolMonitor.CachedNNC.Status.Scaler.BatchSize, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.EqualValues(t, 150, testIpamPoolMonitor.CachedNNC.Status.Scaler.ReleaseThresholdPercent, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.EqualValues(t, 50, testIpamPoolMonitor.CachedNNC.Status.Scaler.RequestThresholdPercent, "IPAMPoolMonitor cached NNC Status is not reflecting the initial set values")
	assert.Len(t, testIpamPoolMonitor.CachedNNC.Status.NetworkContainers, 1, "Expected only one Network Container in the list")

	t.Logf("In-memory Data: ")
	for i := range inmemory.HTTPRestServiceData.PodIPIDByPodInterfaceKey {
		t.Logf("PodIPIDByOrchestratorContext: %+v", inmemory.HTTPRestServiceData.PodIPIDByPodInterfaceKey[i])
	}
	t.Logf("PodIPConfigState: %+v", inmemory.HTTPRestServiceData.PodIPConfigState)
	t.Logf("IPAMPoolMonitor: %+v", inmemory.HTTPRestServiceData.IPAMPoolMonitor)
	service.Stop()
}

func addTestStateToRestServer(t *testing.T, secondaryIps []string) {
	var ipConfig cns.IPConfiguration
	ipConfig.DNSServers = dnsServers
	ipConfig.GatewayIPAddress = gatewayIP
	var ipSubnet cns.IPSubnet
	ipSubnet.IPAddress = primaryIP
	ipSubnet.PrefixLength = subnetPrfixLength
	ipConfig.IPSubnet = ipSubnet
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	for _, secIPAddress := range secondaryIps {
		secIPConfig := cns.SecondaryIPConfig{
			IPAddress: secIPAddress,
			NCVersion: -1,
		}
		ipID := uuid.New()
		secondaryIPConfigs[ipID.String()] = secIPConfig
	}

	req := &cns.CreateNetworkContainerRequest{
		NetworkContainerType: dockerContainerType,
		NetworkContainerid:   ncid,
		IPConfiguration:      ipConfig,
		SecondaryIPConfigs:   secondaryIPConfigs,
		// Set it as -1 to be same as default host version.
		// It will allow secondary IPs status to be set as available.
		Version: "-1",
	}

	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req)
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
	}

	_ = svc.IPAMPoolMonitor.Update(&v1alpha.NodeNetworkConfig{
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: 16,
			IPsNotInUse:      []string{"abc"},
		},
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: 150,
				RequestThresholdPercent: 50,
				MaxIPCount:              250,
			},
			NetworkContainers: []v1alpha.NetworkContainer{
				{},
			},
		},
	})
}

func getIPNetFromResponse(resp *cns.IPConfigResponse) (net.IPNet, error) {
	var (
		resultIPnet net.IPNet
		err         error
	)

	// set result ipconfig from CNS Response Body
	prefix := strconv.Itoa(int(resp.PodIpInfo.PodIPConfig.PrefixLength))
	ip, ipnet, err := net.ParseCIDR(resp.PodIpInfo.PodIPConfig.IPAddress + "/" + prefix)
	if err != nil {
		return resultIPnet, err //nolint:wrapcheck // we don't need error wrapping for tests
	}

	// construct ipnet for result
	resultIPnet = net.IPNet{
		IP:   ip,
		Mask: ipnet.Mask,
	}
	return resultIPnet, err //nolint:wrapcheck // we don't need error wrapping for tests
}
