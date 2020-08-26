// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/ipampoolmonitor"
	"github.com/google/uuid"
)

const (
	primaryIp           = "10.0.0.5"
	gatewayIp           = "10.0.0.1"
	subnetPrfixLength   = 24
	dockerContainerType = cns.Docker
)

var (
	dnsservers = []string{"8.8.8.8", "8.8.4.4"}
)

func TestCreateOrUpdateNetworkContainerInternal(t *testing.T) {
	restartService()

	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	validateCreateOrUpdateNCInternal(t, 2)
}

func TestReconcileNCWithEmptyState(t *testing.T) {
	restartService()
	var scalarUnits cns.ScalarUnits
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	expectedNcCount := len(svc.state.ContainerStatus)
	expectedAllocatedPods := make(map[string]cns.KubernetesPodInfo)
	returnCode := svc.ReconcileNCState(nil, expectedAllocatedPods, scalarUnits)
	if returnCode != Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}

	validateNCStateAfterReconcile(t, nil, expectedNcCount, expectedAllocatedPods)
}

func TestReconcileNCWithExistingState(t *testing.T) {
	restartService()
	var scalarUnits cns.ScalarUnits
	svc.PoolMonitor = ipampoolmonitor.NewCNSIPAMPoolMonitor(nil, nil)
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	var startingIndex = 6
	for i := 0; i < 4; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}
	req := generateNetworkContainerRequest(secondaryIPConfigs, "reconcileNc1")

	expectedAllocatedPods := make(map[string]cns.KubernetesPodInfo)
	expectedAllocatedPods["10.0.0.6"] = cns.KubernetesPodInfo{
		PodName:      "reconcilePod1",
		PodNamespace: "PodNS1",
	}

	expectedAllocatedPods["10.0.0.7"] = cns.KubernetesPodInfo{
		PodName:      "reconcilePod2",
		PodNamespace: "PodNS1",
	}

	expectedNcCount := len(svc.state.ContainerStatus)
	returnCode := svc.ReconcileNCState(&req, expectedAllocatedPods, scalarUnits)
	if returnCode != Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}

	validateNCStateAfterReconcile(t, &req, expectedNcCount+1, expectedAllocatedPods)
}

func TestReconcileNCWithSystemPods(t *testing.T) {
	restartService()
	svc.PoolMonitor = ipampoolmonitor.NewCNSIPAMPoolMonitor(nil, nil)
	var scalarUnits cns.ScalarUnits
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	var startingIndex = 6
	for i := 0; i < 4; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}
	req := generateNetworkContainerRequest(secondaryIPConfigs, uuid.New().String())

	expectedAllocatedPods := make(map[string]cns.KubernetesPodInfo)
	expectedAllocatedPods["10.0.0.6"] = cns.KubernetesPodInfo{
		PodName:      "customerpod1",
		PodNamespace: "PodNS1",
	}

	// Allocate non-vnet IP for system  pod
	expectedAllocatedPods["192.168.0.1"] = cns.KubernetesPodInfo{
		PodName:      "systempod",
		PodNamespace: "kube-system",
	}

	expectedNcCount := len(svc.state.ContainerStatus)
	returnCode := svc.ReconcileNCState(&req, expectedAllocatedPods, scalarUnits)
	if returnCode != Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}

	delete(expectedAllocatedPods, "192.168.0.1")
	validateNCStateAfterReconcile(t, &req, expectedNcCount, expectedAllocatedPods)
}

func setOrchestratorTypeInternal(orchestratorType string) {
	fmt.Println("setOrchestratorTypeInternal")
	svc.state.OrchestratorType = orchestratorType
}

func validateCreateOrUpdateNCInternal(t *testing.T, secondaryIpCount int) {
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	ncId := "testNc1"

	var startingIndex = 6
	for i := 0; i < secondaryIpCount; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncId)

	// now Validate Update, add more secondaryIpConfig and it should handle the update
	fmt.Println("Validate Scaleup")
	for i := 0; i < secondaryIpCount; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncId)

	// now Scale down, delete 3 ipaddresses from secondaryIpConfig req
	fmt.Println("Validate Scale down")
	var count = 0
	for ipid := range secondaryIPConfigs {
		delete(secondaryIPConfigs, ipid)
		count++

		if count > secondaryIpCount {
			break
		}
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncId)

	// Cleanup all SecondaryIps
	fmt.Println("Validate no SecondaryIpconfigs")
	for ipid := range secondaryIPConfigs {
		delete(secondaryIPConfigs, ipid)
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncId)
}

func createAndValidateNCRequest(t *testing.T, secondaryIPConfigs map[string]cns.SecondaryIPConfig, ncId string) {
	req := generateNetworkContainerRequest(secondaryIPConfigs, ncId)
	var scalarUnits cns.ScalarUnits
	svc.PoolMonitor = ipampoolmonitor.NewCNSIPAMPoolMonitor(nil, nil)
	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req, scalarUnits)
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
	}
	validateNetworkRequest(t, req)
}

// Validate the networkRequest is persisted.
func validateNetworkRequest(t *testing.T, req cns.CreateNetworkContainerRequest) {
	containerStatus := svc.state.ContainerStatus[req.NetworkContainerid]

	if containerStatus.ID != req.NetworkContainerid {
		t.Fatalf("Failed as NCId is not persisted, expected:%s, actual %s", req.NetworkContainerid, containerStatus.ID)
	}

	actualReq := containerStatus.CreateNetworkContainerRequest
	if actualReq.NetworkContainerType != req.NetworkContainerType {
		t.Fatalf("Failed as ContainerTyper doesnt match, expected:%s, actual %s", req.NetworkContainerType, actualReq.NetworkContainerType)
	}

	if actualReq.IPConfiguration.IPSubnet.IPAddress != req.IPConfiguration.IPSubnet.IPAddress {
		t.Fatalf("Failed as Primary IPAddress doesnt match, expected:%s, actual %s", req.IPConfiguration.IPSubnet.IPAddress, actualReq.IPConfiguration.IPSubnet.IPAddress)
	}

	// Validate Secondary ips are added in the PodMap
	if len(svc.PodIPConfigState) != len(req.SecondaryIPConfigs) {
		t.Fatalf("Failed as Secondary IP count doesnt match in PodIpConfig state, expected:%d, actual %d", len(req.SecondaryIPConfigs), len(svc.PodIPConfigState))
	}

	var alreadyValidated = make(map[string]string)
	for ipid, ipStatus := range svc.PodIPConfigState {
		if ipaddress, found := alreadyValidated[ipid]; !found {
			if secondaryIpConfig, ok := req.SecondaryIPConfigs[ipid]; !ok {
				t.Fatalf("PodIpConfigState has stale ipId: %s, config: %+v", ipid, ipStatus)
			} else {
				if ipStatus.IPAddress != secondaryIpConfig.IPAddress {
					t.Fatalf("IPId: %s IPSubnet doesnt match: expected %+v, actual: %+v", ipid, secondaryIpConfig.IPAddress, ipStatus.IPAddress)
				}

				// Validate IP state
				if ipStatus.OrchestratorContext != nil {
					var podInfo cns.KubernetesPodInfo
					if err := json.Unmarshal(ipStatus.OrchestratorContext, &podInfo); err != nil {
						t.Fatalf("Failed to add IPConfig to state: %+v with error: %v", ipStatus, err)
					}

					if _, exists := svc.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]; exists {
						if ipStatus.State != cns.Allocated {
							t.Fatalf("IPId: %s State is not Allocated, ipStatus: %+v", ipid, ipStatus)
						}
					} else {
						t.Fatalf("Failed to find podContext for allocated ip: %+v, podinfo :%+v", ipStatus, podInfo)
					}
				} else if ipStatus.State != cns.Available {
					// Todo: Validate for pendingRelease as well
					t.Fatalf("IPId: %s State is not Available, ipStatus: %+v", ipid, ipStatus)
				}

				alreadyValidated[ipid] = ipStatus.IPAddress
			}
		} else {
			// if ipaddress is not same, then fail
			if ipaddress != ipStatus.IPAddress {
				t.Fatalf("Added the same IP guid :%s with different ipaddress, expected:%s, actual %s", ipid, ipStatus.IPAddress, ipaddress)
			}
		}
	}
}

func generateNetworkContainerRequest(secondaryIps map[string]cns.SecondaryIPConfig, ncId string) cns.CreateNetworkContainerRequest {
	var ipConfig cns.IPConfiguration
	ipConfig.DNSServers = dnsservers
	ipConfig.GatewayIPAddress = gatewayIp
	var ipSubnet cns.IPSubnet
	ipSubnet.IPAddress = primaryIp
	ipSubnet.PrefixLength = subnetPrfixLength
	ipConfig.IPSubnet = ipSubnet

	req := cns.CreateNetworkContainerRequest{
		NetworkContainerType: dockerContainerType,
		NetworkContainerid:   ncId,
		IPConfiguration:      ipConfig,
	}

	req.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig)
	for k, v := range secondaryIps {
		req.SecondaryIPConfigs[k] = v
	}

	fmt.Printf("NC Request %+v", req)

	return req
}

func validateNCStateAfterReconcile(t *testing.T, ncRequest *cns.CreateNetworkContainerRequest, expectedNcCount int, expectedAllocatedPods map[string]cns.KubernetesPodInfo) {
	if ncRequest == nil {
		// check svc ContainerStatus will be empty
		if len(svc.state.ContainerStatus) != expectedNcCount {
			t.Fatalf("CNS has some stale ContainerStatus, count: %d, state: %+v", len(svc.state.ContainerStatus), svc.state.ContainerStatus)
		}
	} else {
		validateNetworkRequest(t, *ncRequest)
	}

	if len(expectedAllocatedPods) != len(svc.PodIPIDByOrchestratorContext) {
		t.Fatalf("Unexpected allocated pods, actual: %d, expected: %d", len(svc.PodIPIDByOrchestratorContext), len(expectedAllocatedPods))
	}

	for ipaddress, podInfo := range expectedAllocatedPods {
		ipId := svc.PodIPIDByOrchestratorContext[podInfo.GetOrchestratorContextKey()]
		ipConfigstate := svc.PodIPConfigState[ipId]

		if ipConfigstate.State != cns.Allocated {
			t.Fatalf("IpAddress %s is not marked as allocated for Pod: %+v, ipState: %+v", ipaddress, podInfo, ipConfigstate)
		}

		// Validate if IPAddress matches
		if ipConfigstate.IPAddress != ipaddress {
			t.Fatalf("IpAddress %s is not same, for Pod: %+v, actual ipState: %+v", ipaddress, podInfo, ipConfigstate)
		}

		// Valdate pod context
		var expectedPodInfo cns.KubernetesPodInfo
		json.Unmarshal(ipConfigstate.OrchestratorContext, &expectedPodInfo)
		if reflect.DeepEqual(expectedPodInfo, podInfo) != true {
			t.Fatalf("OrchestrationContext: is not same, expected: %+v, actual %+v", expectedPodInfo, podInfo)
		}

		// Validate this IP belongs to a valid NCRequest
		nc := svc.state.ContainerStatus[ipConfigstate.NCID]
		if _, exists := nc.CreateNetworkContainerRequest.SecondaryIPConfigs[ipConfigstate.ID]; !exists {
			t.Fatalf("Secondary IP config doest exist in NC, ncid: %s, ipId %s", ipConfigstate.NCID, ipConfigstate.ID)
		}
	}

	// validate rest of Secondary IPs in Available state
	if ncRequest != nil {
		for secIpId, secIpConfig := range ncRequest.SecondaryIPConfigs {
			if _, exists := expectedAllocatedPods[secIpConfig.IPAddress]; exists {
				continue
			}

			// Validate IP state
			if secIpConfigState, found := svc.PodIPConfigState[secIpId]; found {
				if secIpConfigState.State != cns.Available {
					t.Fatalf("IPId: %s State is not Available, ipStatus: %+v", secIpId, secIpConfigState)
				}
			} else {
				t.Fatalf("IPId: %s, IpAddress: %+v State doesnt exists in PodIp Map", secIpId, secIpConfig)
			}
		}
	}
}

func restartService() {
	fmt.Println("Restart Service")

	service.Stop()
	startService()
}
