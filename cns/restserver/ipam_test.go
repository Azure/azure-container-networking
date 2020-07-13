// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
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

	testPod3GUID = "718e04ac-5a13-4dce-84b3-040accaa9b41"
	testPod3Info = cns.KubernetesPodInfo{
		PodName:      "testpod3",
		PodNamespace: "testpod3namespace",
	}
)

func getTestService() *HTTPRestService {
	var config common.ServiceConfig
	httpsvc, _ := NewHTTPRestService(&config)
	svc := httpsvc.(*HTTPRestService)
	svc.state.OrchestratorType = cns.Kubernetes

	return svc
}

// Want first IP
func TestIPAMGetAvailableIPConfig(t *testing.T) {
	svc := getTestService()

	testState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Available)
	ipconfigs := []*cns.ContainerIPConfigState{
		testState,
	}
	svc.AddIPConfigsToState(ipconfigs)

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b

	actualstate, err := requestIPConfigHelper(svc, req)
	if err != nil {
		t.Fatal("Expected IP retrieval to be nil")
	}

	desiredState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Allocated)
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
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	state2 := NewPodState(testIP2, 24, testPod2GUID, testNCID, cns.Available)

	ipconfigs := []*cns.ContainerIPConfigState{
		state1,
		state2,
	}
	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b

	actualstate, err := requestIPConfigHelper(svc, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}
	// want second available Pod IP State as first has been allocated
	desiredState, _ := NewPodStateWithOrchestratorContext(testIP2, 24, testPod2GUID, testNCID, cns.Allocated, testPod2Info)

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", desiredState, actualstate)
	}
}

func TestIPAMGetAlreadyAllocatedIPConfigForSamePod(t *testing.T) {
	svc := getTestService()

	// Add Allocated Pod IP to state
	testState, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	ipconfigs := []*cns.ContainerIPConfigState{
		testState,
	}
	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b

	actualstate, err := requestIPConfigHelper(svc, req)
	if err != nil {
		t.Fatalf("Expected not error: %+v", err)
	}

	desiredState, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", desiredState, actualstate)
	}
}

func TestIPAMAttemptToRequestIPNotFoundInPool(t *testing.T) {
	svc := getTestService()

	// Add Available Pod IP to state
	testState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Available)
	ipconfigs := []*cns.ContainerIPConfigState{
		testState,
	}

	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPConfig = newIPConfig(testIP2, 24)

	_, err = requestIPConfigHelper(svc, req)
	if err == nil {
		t.Fatalf("Expected to fail as IP not found in pool")
	}
}

func TestIPAMGetDesiredIPConfigWithSpecfiedIP(t *testing.T) {
	svc := getTestService()

	// Add Available Pod IP to state
	testState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Available)
	ipconfigs := []*cns.ContainerIPConfigState{
		testState,
	}

	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b
	req.DesiredIPConfig = newIPConfig(testIP1, 24)

	actualstate, err := requestIPConfigHelper(svc, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}

	desiredState := NewPodState(testIP1, 24, testPod1GUID, testNCID, cns.Allocated)
	desiredState.OrchestratorContext = b

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", desiredState, actualstate)
	}
}

func TestIPAMFailToGetDesiredIPConfigWithAlreadyAllocatedSpecfiedIP(t *testing.T) {
	svc := getTestService()

	// set state as already allocated
	testState, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	ipconfigs := []*cns.ContainerIPConfigState{
		testState,
	}
	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// request the already allocated ip with a new context
	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPConfig = newIPConfig(testIP1, 24)

	_, err = requestIPConfigHelper(svc, req)
	if err == nil {
		t.Fatalf("Expected failure requesting already IP: %+v", err)
	}
}

func TestIPAMFailToGetIPWhenAllIPsAreAllocated(t *testing.T) {
	svc := getTestService()

	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	state2, _ := NewPodStateWithOrchestratorContext(testIP2, 24, testPod2GUID, testNCID, cns.Allocated, testPod2Info)

	ipconfigs := []*cns.ContainerIPConfigState{
		state1,
		state2,
	}
	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// request the already allocated ip with a new context
	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod3Info)
	req.OrchestratorContext = b

	_, err = requestIPConfigHelper(svc, req)
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
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	ipconfigs := []*cns.ContainerIPConfigState{
		state1,
	}

	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	desiredIPConfig := newIPConfig(testIP1, 24)

	// Use TestPodInfo2 to request TestIP1, which has already been allocated
	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPConfig = desiredIPConfig

	_, err = requestIPConfigHelper(svc, req)
	if err == nil {
		t.Fatal("Expected failure requesting IP when there are no more IP's")
	}

	// Release Test Pod 1
	err = svc.ReleaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Rerequest
	req = cns.GetIPConfigRequest{}
	b, _ = json.Marshal(testPod2Info)
	req.OrchestratorContext = b
	req.DesiredIPConfig = desiredIPConfig

	actualstate, err := requestIPConfigHelper(svc, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}

	desiredState, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	// want first available Pod IP State
	desiredState.IPConfig = desiredIPConfig
	desiredState.OrchestratorContext = b

	if reflect.DeepEqual(desiredState, actualstate) != true {
		t.Fatalf("Desired state not matching actual state, expected: %+v, actual: %+v", state1, actualstate)
	}
}

func TestIPAMFailToAddThenCleanThenRequestExpectFail(t *testing.T) {
	svc := getTestService()

	var err error

	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Available, testPod1Info)
	state2, _ := NewPodStateWithOrchestratorContext("", 24, "", testNCID, cns.Available, testPod1Info)

	ipconfigs := []*cns.ContainerIPConfigState{
		state1,
		state2,
	}

	err = svc.AddIPConfigsToState(ipconfigs)
	if err == nil {
		t.Fatalf("Expected add to fail when bad ipconfig is added.")
	}

	req := cns.GetIPConfigRequest{}
	b, _ := json.Marshal(testPod1Info)
	req.OrchestratorContext = b

	_, err = requestIPConfigHelper(svc, req)
	if err == nil {
		t.Fatalf("Expected state to be clean when one ipconfig is bad in batch add.")
	}
}

func TestIPAMReleaseIPIdempotency(t *testing.T) {
	svc := getTestService()
	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	ipconfigs := []*cns.ContainerIPConfigState{
		state1,
	}

	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	// Release Test Pod 1
	err = svc.ReleaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Call release again, should be fine
	err = svc.ReleaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}
}

func TestIPAMAllocateIPIdempotency(t *testing.T) {
	svc := getTestService()
	// set state as already allocated
	state1, _ := NewPodStateWithOrchestratorContext(testIP1, 24, testPod1GUID, testNCID, cns.Allocated, testPod1Info)
	ipconfigs := []*cns.ContainerIPConfigState{
		state1,
	}

	err := svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}

	err = svc.AddIPConfigsToState(ipconfigs)
	if err != nil {
		t.Fatalf("Expected to not fail adding IP's to state: %+v", err)
	}
}
