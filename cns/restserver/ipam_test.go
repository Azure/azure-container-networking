// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
)

var (
	testNCID = "06867cf3-332d-409d-8819-ed70d2c116b0"

	testIP1      = "10.0.0.1"
	testPod1GUID = "898fb8f1-f93e-4c96-9c31-6b89098949a3"
	testPod1Info = cns.KubernetesPodInfo{
		PodName:      "testpod1",
		PodNamespace: "testpod1namespace",
	}

	testIP2      = "10.0.0.2"
	testPod2GUID = "b21e1ee1-fb7e-4e6d-8c68-22ee5049944e"
	testPod2Info = cns.KubernetesPodInfo{
		PodName:      "testpod2",
		PodNamespace: "testpod2namespace",
	}

	testIP3      = "10.0.0.3"
	testPod3GUID = "718e04ac-5a13-4dce-84b3-040accaa9b41"
	testPod3Info = cns.KubernetesPodInfo{
		PodName:      "testpod3",
		PodNamespace: "testpod3namespace",
	}
)

func getTestService() *HTTPRestService {
	var config common.ServiceConfig
	httpsvc, _ := NewHTTPRestService(&config, fakes.NewFakeImdsClient())
	svc = httpsvc.(*HTTPRestService)
	svc.IPAMPoolMonitor = fakes.NewIPAMPoolMonitorFake()
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	return svc
}

func newSecondaryIPConfig(ipAddress string, ncVersion int) cns.SecondaryIPConfig {
	return cns.SecondaryIPConfig{
		IPAddress: ipAddress,
		NCVersion: ncVersion,
	}
}

func NewPodState(ipaddress string, prefixLength uint8, id, ncid, state string, ncVersion int) cns.IPConfigurationStatus {
	ipconfig := newSecondaryIPConfig(ipaddress, ncVersion)

	return cns.IPConfigurationStatus{
		IPAddress: ipconfig.IPAddress,
		ID:        id,
		NCID:      ncid,
		State:     state,
	}
}

func requestIpAddressAndGetState(t *testing.T, req cns.GetIPConfigRequest) (cns.IPConfigurationStatus, error) {
	var (
		podInfo   cns.KubernetesPodInfo
		ipState   cns.IPConfigurationStatus
		PodIpInfo cns.PodIpInfo
		err       error
	)

	PodIpInfo, err = requestIPConfigHelper(svc, req)
	if err != nil {
		return ipState, err
	}

	if reflect.DeepEqual(PodIpInfo.NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress, primaryIp) != true {
		t.Fatalf("PrimarIP is not added as expected ipConfig %+v, expected primaryIP: %+v", PodIpInfo.NetworkContainerPrimaryIPConfig, primaryIp)
	}

	if PodIpInfo.NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength != subnetPrfixLength {
		t.Fatalf("Primary IP Prefix length is not added as expected ipConfig %+v, expected: %+v", PodIpInfo.NetworkContainerPrimaryIPConfig, subnetPrfixLength)
	}

	// validate DnsServer and Gateway Ip as the same configured for Primary IP
	if reflect.DeepEqual(PodIpInfo.NetworkContainerPrimaryIPConfig.DNSServers, dnsservers) != true {
		t.Fatalf("DnsServer is not added as expected ipConfig %+v, expected dnsServers: %+v", PodIpInfo.NetworkContainerPrimaryIPConfig, dnsservers)
	}

	if reflect.DeepEqual(PodIpInfo.NetworkContainerPrimaryIPConfig.GatewayIPAddress, gatewayIp) != true {
		t.Fatalf("Gateway is not added as expected ipConfig %+v, expected GatewayIp: %+v", PodIpInfo.NetworkContainerPrimaryIPConfig, gatewayIp)
	}

	if PodIpInfo.PodIPConfig.PrefixLength != subnetPrfixLength {
		t.Fatalf("Pod IP Prefix length is not added as expected ipConfig %+v, expected: %+v", PodIpInfo.PodIPConfig, subnetPrfixLength)
	}

	if reflect.DeepEqual(PodIpInfo.HostPrimaryIPInfo.PrimaryIP, fakes.HostPrimaryIpTest) != true {
		t.Fatalf("Host PrimaryIP is not added as expected ipConfig %+v, expected primaryIP: %+v", PodIpInfo.HostPrimaryIPInfo, fakes.HostPrimaryIpTest)
	}

	if reflect.DeepEqual(PodIpInfo.HostPrimaryIPInfo.Subnet, fakes.HostSubnetTest) != true {
		t.Fatalf("Host Subnet is not added as expected ipConfig %+v, expected Host subnet: %+v", PodIpInfo.HostPrimaryIPInfo, fakes.HostSubnetTest)
	}

	// retrieve podinfo from orchestrator context
	if err := json.Unmarshal(req.OrchestratorContext, &podInfo); err != nil {
		return ipState, err
	}

	ipId := svc.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]
	ipState = svc.PodIPConfigState[ipId]

	return ipState, err
}

func NewPodStateWithOrchestratorContext(ipaddress, id, ncid, state string, prefixLength uint8, ncVersion int, orchestratorContext cns.KubernetesPodInfo) (cns.IPConfigurationStatus, error) {
	ipconfig := newSecondaryIPConfig(ipaddress, ncVersion)
	b, err := json.Marshal(orchestratorContext)
	return cns.IPConfigurationStatus{
		IPAddress:           ipconfig.IPAddress,
		ID:                  id,
		NCID:                ncid,
		State:               state,
		OrchestratorContext: b,
	}, err
}

// Test function to populate the IPConfigState
func UpdatePodIpConfigState(t *testing.T, svc *HTTPRestService, ipconfigs map[string]cns.IPConfigurationStatus) error {
	// Create NC
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	for _, ipconfig := range ipconfigs {
		secIpConfig := cns.SecondaryIPConfig{
			IPAddress: ipconfig.IPAddress,
		}

		ipId := ipconfig.ID
		secondaryIPConfigs[ipId] = secIpConfig
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, testNCID, "0")

	// update ipconfigs to expected state
	for ipId, ipconfig := range ipconfigs {
		if ipconfig.State == cns.Allocated {
			var podInfo cns.KubernetesPodInfo

			if err := json.Unmarshal(ipconfig.OrchestratorContext, &podInfo); err != nil {
				return fmt.Errorf("Failed to add IPConfig to state: %+v with error: %v", ipconfig, err)
			}

			svc.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()] = ipId
			svc.PodIPConfigState[ipId] = ipconfig
		}
	}
	return nil
}

// Want first IP
func TestIPAMGetAvailableIPConfig(t *testing.T) {
	svc := getTestService()

	testState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Available, 0)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}
	UpdatePodIpConfigState(t, svc, ipconfigs)

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b

	actualstate, err := requestIpAddressAndGetState(t, req)
	if err != nil {
		t.Fatal("Expected IP retrieval to be nil")
	}

	desiredState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, 0)
	desiredState.OrchestratorContext = b

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", desiredState, actualstate)
	}
}

// First IP is already assigned to a pod, want second IP
func TestIPAMGetNextAvailableIPConfig(t *testing.T) {
	svc := getTestService()

	// Add already allocated pod ip to state
	svc.PodIPIDByOrchestratorContext[testPod1Info.GetOrchestratorContextKey()] = testPod1GUID
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	state2 := NewPodState(testIP2, 24, testPod2GUID, testNCID, cns.Available, 0)

	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
		state2.ID: state2,
	}
	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b

	actualstate, err := requestIpAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}
	// want second available Pod IP State as first has been allocated
	desiredState, _ := NewPodStateWithOrchestratorContext(testIP2, testPod2GUID, testNCID, cns.Allocated, 24, 0, testPod2Info)

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", desiredState, actualstate)
	}
}

func TestIPAMGetAlreadyAllocatedIPConfigForSamePod(t *testing.T) {
	svc := getTestService()

	// Add Allocated Pod IP to state
	testState, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}
	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b

	actualstate, err := requestIpAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected not error: %+v", err)
	}

	desiredState, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", desiredState, actualstate)
	}
}

func TestIPAMAttemptToRequestIPNotFoundInPool(t *testing.T) {
	svc := getTestService()

	// Add Available Pod IP to state
	testState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Available, 0)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}

	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPAddress = testIP2

	_, err = requestIpAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected to fail as IP not found in pool")
	}
}

func TestIPAMGetDesiredIPConfigWithSpecfiedIP(t *testing.T) {
	svc := getTestService()

	// Add Available Pod IP to state
	testState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Available, 0)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}

	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b
	req.DesiredIPAddress = testIP1

	actualstate, err := requestIpAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}

	desiredState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, 0)
	desiredState.OrchestratorContext = b

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", desiredState, actualstate)
	}
}

func TestIPAMFailToGetDesiredIPConfigWithAlreadyAllocatedSpecfiedIP(t *testing.T) {
	svc := getTestService()

	// set state as already allocated
	testState, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}
	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// request the already allocated ip with a new context
	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPAddress = testIP1

	_, err = requestIpAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected failure requesting already allocated IP: %+v", err)
	}
}

func TestIPAMFailToGetIPWhenAllIPsAreAllocated(t *testing.T) {
	svc := getTestService()

	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	state2, _ := NewPodStateWithOrchestratorContext(testIP2, testPod2GUID, testNCID, cns.Allocated, 24, 0, testPod2Info)

	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
		state2.ID: state2,
	}
	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// request the already allocated ip with a new context
	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod3Info)
	req.OrchestratorContext = b

	_, err = requestIpAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected failure requesting IP when there are no more IP's: %+v", err)
	}
}

// 10.0.0.1 = PodInfo1
// Request 10.0.0.1 with PodInfo2 (Fail)
// Release PodInfo1
// Request 10.0.0.1 with PodInfo2 (Success)
func TestIPAMRequestThenReleaseThenRequestAgain(t *testing.T) {
	svc := getTestService()

	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
	}

	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	desiredIpAddress := testIP1

	// Use TestPodInfo2 to request TestIP1, which has already been allocated
	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPAddress = desiredIpAddress

	_, err = requestIpAddressAndGetState(t, req)
	if err == nil {
		t.Fatal("Expected failure requesting IP when there are no more IP's")
	}

	// Release Test Pod 1
	err = svc.releaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Rerequest
	req = cns.GetIPConfigRequest{}
	b, _ = json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPAddress = desiredIpAddress

	actualstate, err := requestIpAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}

	desiredState, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	// want first available Pod IP State
	desiredState.IPAddress = desiredIpAddress
	desiredState.OrchestratorContext = b

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", state1, actualstate)
	}
}

func TestIPAMReleaseIPIdempotency(t *testing.T) {
	svc := getTestService()
	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
	}

	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// Release Test Pod 1
	err = svc.releaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Call release again, should be fine
	err = svc.releaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}
}

func TestIPAMAllocateIPIdempotency(t *testing.T) {
	svc := getTestService()
	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)

	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
	}

	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	err = UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}
}

func TestAvailableIPConfigs(t *testing.T) {
	svc := getTestService()

	state1 := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Available, 0)
	state2 := NewPodState(testIP2, 24, testPod2GUID, testNCID, cns.Available, 0)
	state3 := NewPodState(testIP3, 24, testPod3GUID, testNCID, cns.Available, 0)

	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
		state2.ID: state2,
		state3.ID: state3,
	}
	UpdatePodIpConfigState(t, svc, ipconfigs)

	desiredAvailableIps := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
		state2.ID: state2,
		state3.ID: state3,
	}
	availableIps := svc.GetAvailableIPConfigs()
	validateIpState(t, availableIps, desiredAvailableIps)

	desiredAllocatedIpConfigs := make(map[string]cns.IPConfigurationStatus)
	allocatedIps := svc.GetAllocatedIPConfigs()
	validateIpState(t, allocatedIps, desiredAllocatedIpConfigs)

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b
	req.DesiredIPAddress = state1.IPAddress

	_, err := requestIpAddressAndGetState(t, req)
	if err != nil {
		t.Fatal("Expected IP retrieval to be nil")
	}

	delete(desiredAvailableIps, state1.ID)
	availableIps = svc.GetAvailableIPConfigs()
	validateIpState(t, availableIps, desiredAvailableIps)

	desiredState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, 0)
	desiredState.OrchestratorContext = b
	desiredAllocatedIpConfigs[desiredState.ID] = desiredState
	allocatedIps = svc.GetAllocatedIPConfigs()
	validateIpState(t, allocatedIps, desiredAllocatedIpConfigs)

}

func validateIpState(t *testing.T, actualIps []cns.IPConfigurationStatus, expectedList map[string]cns.IPConfigurationStatus) {
	if len(actualIps) != len(expectedList) {
		t.Fatalf("Actual and expected  count doesnt match, expected %d, actual %d", len(actualIps), len(expectedList))
	}

	for _, actualIp := range actualIps {
		var expectedIp cns.IPConfigurationStatus
		var found bool
		for _, expectedIp = range expectedList {
			if reflect.DeepEqual(actualIp, expectedIp) == true {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("Actual and expected list doesnt match actual: %+v, expected: %+v", actualIp, expectedIp)
		}
	}
}

func TestIPAMMarkIPCountAsPending(t *testing.T) {
	svc := getTestService()
	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Available, 24, 0, testPod1Info)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
	}

	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// Release Test Pod 1
	ips, err := svc.MarkIPsAsPending(1)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	if _, exists := ips[testPod1GUID]; !exists {
		t.Fatalf("Expected ID not marked as pending: %+v", err)
	}

	// Release Test Pod 1
	pendingrelease := svc.GetPendingReleaseIPConfigs()
	if len(pendingrelease) != 1 {
		t.Fatal("Expected pending release slice to be nonzero after pending release")
	}

	available := svc.GetAvailableIPConfigs()
	if len(available) != 0 {
		t.Fatal("Expected available ips to be zero after marked as pending")
	}

	// Call release again, should be fine
	err = svc.releaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}
}

func TestIPAMMarkExistingIPConfigAsPending(t *testing.T) {
	svc := getTestService()

	// Add already allocated pod ip to state
	svc.PodIPIDByOrchestratorContext[testPod1Info.GetOrchestratorContextKey()] = testPod1GUID
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, cns.Allocated, 24, 0, testPod1Info)
	state2 := NewPodState(testIP2, 24, testPod2GUID, testNCID, cns.Available, 0)

	ipconfigs := map[string]cns.IPConfigurationStatus{
		state1.ID: state1,
		state2.ID: state2,
	}
	err := UpdatePodIpConfigState(t, svc, ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// mark available ip as as pending
	pendingIPIDs := []string{testPod2GUID}
	err = svc.MarkExistingIPsAsPending(pendingIPIDs)
	if err != nil {
		t.Fatalf("Expected to successfully mark available ip as pending")
	}

	pendingIPConfigs := svc.GetPendingReleaseIPConfigs()
	if pendingIPConfigs[0].ID != testPod2GUID {
		t.Fatalf("Expected to see ID %v in pending release ipconfigs, actual %+v", testPod2GUID, pendingIPConfigs)
	}

	// attempt to mark allocated ipconfig as pending, expect fail
	pendingIPIDs = []string{testPod1GUID}
	err = svc.MarkExistingIPsAsPending(pendingIPIDs)
	if err == nil {
		t.Fatalf("Expected to fail when marking allocated ip as pending")
	}

	allocatedIPConfigs := svc.GetAllocatedIPConfigs()
	if allocatedIPConfigs[0].ID != testPod1GUID {
		t.Fatalf("Expected to see ID %v in pending release ipconfigs, actual %+v", testPod1GUID, allocatedIPConfigs)
	}
}
