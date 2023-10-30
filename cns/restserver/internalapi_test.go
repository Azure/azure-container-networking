// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	nma "github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/store"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
)

const (
	primaryIp           = "10.0.0.5"
	gatewayIp           = "10.0.0.1"
	subnetPrfixLength   = 24
	dockerContainerType = cns.Docker
	releasePercent      = 50
	requestPercent      = 100
	batchSize           = 10
	initPoolSize        = 10
	ncID                = "6a07155a-32d7-49af-872f-1e70ee366dc0"
)

var dnsservers = []string{"8.8.8.8", "8.8.4.4"}

func TestCreateOrUpdateNetworkContainerInternal(t *testing.T) {
	restartService()

	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)
	// NC version set as -1 which is the same as default host version value.
	validateCreateOrUpdateNCInternal(t, 2, "-1")
}

// TestReconcileNCStatePrimaryIPChangeShouldFail tests that reconciling NC state with
// a NC whose IP has changed should fail
func TestReconcileNCStatePrimaryIPChangeShouldFail(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)
	svc.state.ContainerStatus = make(map[string]containerstatus)

	// start with a NC in state
	ncID := "555ac5c9-89f2-4b5d-b8d0-616894d6d151"
	svc.state.ContainerStatus[ncID] = containerstatus{
		ID:          ncID,
		VMVersion:   "0",
		HostVersion: "0",
		CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
			NetworkContainerid: ncID,
			IPConfiguration: cns.IPConfiguration{
				IPSubnet: cns.IPSubnet{
					IPAddress:    "10.0.1.0",
					PrefixLength: 24,
				},
			},
		},
	}

	ncReqs := []*cns.CreateNetworkContainerRequest{
		{
			NetworkContainerid: ncID,
			IPConfiguration: cns.IPConfiguration{
				IPSubnet: cns.IPSubnet{
					IPAddress:    "10.0.2.0", // note this IP has changed
					PrefixLength: 24,
				},
			},
		},
	}

	// now try to reconcile the state where the NC primary IP has changed
	resp := svc.ReconcileIPAMState(ncReqs, map[string]cns.PodInfo{}, &v1alpha.NodeNetworkConfig{})

	assert.Equal(t, types.PrimaryCANotSame, resp)
}

// TestReconcileNCStateGatewayChange tests that NC state gets updated when reconciled
// if the NC's gateway IP has changed
func TestReconcileNCStateGatewayChange(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)
	svc.state.ContainerStatus = make(map[string]containerstatus)

	// start with a NC in state
	ncID := "555ac5c9-89f2-4b5d-b8d0-616894d6d151"
	oldGW := "10.0.0.0"
	newGW := "10.0.1.0"
	ncPrimaryIP := cns.IPSubnet{
		IPAddress:    "10.0.1.1",
		PrefixLength: 24,
	}
	svc.state.ContainerStatus[ncID] = containerstatus{
		ID:          ncID,
		VMVersion:   "0",
		HostVersion: "0",
		CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
			NetworkContainerid:   ncID,
			NetworkContainerType: cns.Kubernetes,
			IPConfiguration: cns.IPConfiguration{
				IPSubnet:         ncPrimaryIP,
				GatewayIPAddress: oldGW,
			},
		},
	}

	ncReqs := []*cns.CreateNetworkContainerRequest{
		{
			NetworkContainerid:   ncID,
			NetworkContainerType: cns.Kubernetes,
			IPConfiguration: cns.IPConfiguration{
				IPSubnet:         ncPrimaryIP,
				GatewayIPAddress: newGW, // note this IP has changed
			},
		},
	}

	// now try to reconcile the state where the NC gateway has changed
	resp := svc.ReconcileIPAMState(ncReqs, map[string]cns.PodInfo{}, &v1alpha.NodeNetworkConfig{})

	assert.Equal(t, types.Success, resp)
	// assert the new state reflects the gateway update
	assert.Equal(t, newGW, svc.state.ContainerStatus[ncID].CreateNetworkContainerRequest.IPConfiguration.GatewayIPAddress)
}

func TestCreateOrUpdateNCWithLargerVersionComparedToNMAgent(t *testing.T) {
	restartService()

	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)
	// NC version set as 1 which is larger than NC version get from mock nmagent.
	validateCreateNCInternal(t, 2, "1")
}

func TestCreateAndUpdateNCWithSecondaryIPNCVersion(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)
	// NC version set as 0 which is the default initial value.
	ncVersion := 0
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	// Build secondaryIPConfig, it will have one item as {IPAddress:"10.0.0.16", NCVersion: 0}
	ipAddress := "10.0.0.16"
	secIPConfig := newSecondaryIPConfig(ipAddress, ncVersion)
	ipId := uuid.New()
	secondaryIPConfigs[ipId.String()] = secIPConfig
	req := createNCReqInternal(t, secondaryIPConfigs, ncID, strconv.Itoa(ncVersion))
	containerStatus := svc.state.ContainerStatus[req.NetworkContainerid]
	// Validate secondary IPs' NC version has been updated by NC request
	receivedSecondaryIPConfigs := containerStatus.CreateNetworkContainerRequest.SecondaryIPConfigs
	if len(receivedSecondaryIPConfigs) != 1 {
		t.Fatalf("receivedSecondaryIPConfigs lenth must be 1, but recieved %d", len(receivedSecondaryIPConfigs))
	}
	for _, secIPConfig := range receivedSecondaryIPConfigs {
		if secIPConfig.IPAddress != "10.0.0.16" || secIPConfig.NCVersion != 0 {
			t.Fatalf("nc request version is %d, secondary ip %s nc version is %d, expected nc version is 0",
				ncVersion, secIPConfig.IPAddress, secIPConfig.NCVersion)
		}
	}

	// now Validate Update, simulate the CRD status where have 2 IP addresses as "10.0.0.16" and "10.0.0.17" with NC version 1
	// The secondaryIPConfigs build from CRD will be {[IPAddress:"10.0.0.16", NCVersion: 1,]; [IPAddress:"10.0.0.17", NCVersion: 1,]}
	// However, when CNS saveNetworkContainerGoalState, since "10.0.0.16" already with NC version 0, it will set it back to 0.
	ncVersion++
	secIPConfig = newSecondaryIPConfig(ipAddress, ncVersion)
	// "10.0.0.16" will be update to NC version 1 in CRD, reuse the same uuid with it.
	secondaryIPConfigs[ipId.String()] = secIPConfig

	// Add {IPAddress:"10.0.0.17", NCVersion: 1} in secondaryIPConfig
	ipAddress = "10.0.0.17"
	secIPConfig = newSecondaryIPConfig(ipAddress, ncVersion)
	ipId = uuid.New()
	secondaryIPConfigs[ipId.String()] = secIPConfig
	req = createNCReqInternal(t, secondaryIPConfigs, ncID, strconv.Itoa(ncVersion))
	// Validate secondary IPs' NC version has been updated by NC request
	containerStatus = svc.state.ContainerStatus[req.NetworkContainerid]
	receivedSecondaryIPConfigs = containerStatus.CreateNetworkContainerRequest.SecondaryIPConfigs
	if len(receivedSecondaryIPConfigs) != 2 {
		t.Fatalf("receivedSecondaryIPConfigs must be 2, but received %d", len(receivedSecondaryIPConfigs))
	}
	for _, secIPConfig := range receivedSecondaryIPConfigs {
		switch secIPConfig.IPAddress {
		case "10.0.0.16":
			// Though "10.0.0.16" IP exists in NC version 1, secodanry IP still keep its original NC version 0
			if secIPConfig.NCVersion != 0 {
				t.Fatalf("nc request version is %d, secondary ip %s nc version is %d, expected nc version is 0",
					ncVersion, secIPConfig.IPAddress, secIPConfig.NCVersion)
			}
		case "10.0.0.17":
			if secIPConfig.NCVersion != 1 {
				t.Fatalf("nc request version is %d, secondary ip %s nc version is %d, expected nc version is 1",
					ncVersion, secIPConfig.IPAddress, secIPConfig.NCVersion)
			}
		default:
			t.Fatalf("nc request version is %d, secondary ip %s nc version is %d should not exist in receivedSecondaryIPConfigs map",
				ncVersion, secIPConfig.IPAddress, secIPConfig.NCVersion)
		}
	}
}

func TestSyncHostNCVersion(t *testing.T) {
	// cns.KubernetesCRD has one more logic compared to other orchestrator type, so test both of them
	orchestratorTypes := []string{cns.Kubernetes, cns.KubernetesCRD}
	for _, orchestratorType := range orchestratorTypes {
		orchestratorType := orchestratorType
		t.Run(orchestratorType, func(t *testing.T) {
			req := createNCReqeustForSyncHostNCVersion(t)
			containerStatus := svc.state.ContainerStatus[req.NetworkContainerid]
			if containerStatus.HostVersion != "-1" {
				t.Errorf("Unexpected containerStatus.HostVersion %s, expected host version should be -1 in string", containerStatus.HostVersion)
			}
			if containerStatus.CreateNetworkContainerRequest.Version != "0" {
				t.Errorf("Unexpected nc version in containerStatus as %s, expected VM version should be 0 in string", containerStatus.CreateNetworkContainerRequest.Version)
			}

			mnma := &fakes.NMAgentClientFake{
				GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
					return nma.NCVersionList{
						Containers: []nma.NCVersion{
							{
								NetworkContainerID: req.NetworkContainerid,
								Version:            "0",
							},
						},
					}, nil
				},
			}
			cleanup := setMockNMAgent(svc, mnma)
			defer cleanup()

			// When syncing the host NC version, it will use the orchestratorType passed
			// in.
			svc.SyncHostNCVersion(context.Background(), orchestratorType)
			containerStatus = svc.state.ContainerStatus[req.NetworkContainerid]
			if containerStatus.HostVersion != "0" {
				t.Errorf("Unexpected containerStatus.HostVersion %s, expected host version should be 0 in string", containerStatus.HostVersion)
			}
			if containerStatus.CreateNetworkContainerRequest.Version != "0" {
				t.Errorf("Unexpected nc version in containerStatus as %s, expected VM version should be 0 in string", containerStatus.CreateNetworkContainerRequest.Version)
			}
		})
	}
}

func TestPendingIPsGotUpdatedWhenSyncHostNCVersion(t *testing.T) {
	req := createNCReqeustForSyncHostNCVersion(t)
	containerStatus := svc.state.ContainerStatus[req.NetworkContainerid]

	receivedSecondaryIPConfigs := containerStatus.CreateNetworkContainerRequest.SecondaryIPConfigs
	if len(receivedSecondaryIPConfigs) != 1 {
		t.Errorf("Unexpected receivedSecondaryIPConfigs length %d, expeted length is 1", len(receivedSecondaryIPConfigs))
	}
	for i := range receivedSecondaryIPConfigs {
		podIPConfigState := svc.PodIPConfigState[i]
		if podIPConfigState.GetState() != types.PendingProgramming {
			t.Errorf("Unexpected State %s, expected State is %s, IP address is %s", podIPConfigState.GetState(), types.PendingProgramming, podIPConfigState.IPAddress)
		}
	}

	mnma := &fakes.NMAgentClientFake{
		GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
			return nma.NCVersionList{
				Containers: []nma.NCVersion{
					{
						NetworkContainerID: req.NetworkContainerid,
						Version:            "0",
					},
				},
			}, nil
		},
	}
	cleanup := setMockNMAgent(svc, mnma)
	defer cleanup()

	svc.SyncHostNCVersion(context.Background(), cns.CRD)
	containerStatus = svc.state.ContainerStatus[req.NetworkContainerid]

	receivedSecondaryIPConfigs = containerStatus.CreateNetworkContainerRequest.SecondaryIPConfigs
	if len(receivedSecondaryIPConfigs) != 1 {
		t.Errorf("Unexpected receivedSecondaryIPConfigs length %d, expeted length is 1", len(receivedSecondaryIPConfigs))
	}
	for i := range receivedSecondaryIPConfigs {
		podIPConfigState := svc.PodIPConfigState[i]
		if podIPConfigState.GetState() != types.Available {
			t.Errorf("Unexpected State %s, expeted State is %s, IP address is %s", podIPConfigState.GetState(), types.Available, podIPConfigState.IPAddress)
		}
	}
}

func createNCReqeustForSyncHostNCVersion(t *testing.T) cns.CreateNetworkContainerRequest {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	// NC version set as 0 which is the default initial value.
	ncVersion := 0
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	// Build secondaryIPConfig, it will have one item as {IPAddress:"10.0.0.16", NCVersion: 0}
	ipAddress := "10.0.0.16"
	secIPConfig := newSecondaryIPConfig(ipAddress, ncVersion)
	ipID := uuid.New()
	secondaryIPConfigs[ipID.String()] = secIPConfig
	req := createNCReqInternal(t, secondaryIPConfigs, ncID, strconv.Itoa(ncVersion))
	return req
}

func TestReconcileNCWithEmptyState(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	expectedNcCount := len(svc.state.ContainerStatus)
	expectedAssignedPods := make(map[string]cns.PodInfo)
	returnCode := svc.ReconcileIPAMState(nil, expectedAssignedPods, &v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})
	if returnCode != types.Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}

	validateNCStateAfterReconcile(t, nil, expectedNcCount, expectedAssignedPods, nil)
}

// TestReconcileNCWithEmptyStateAndPendingRelease tests the case where there is
// no state (node reboot) and there are pending release IPs in the NNC that
// may have been deallocated and should not be made available for assignment
// to pods.
func TestReconcileNCWithEmptyStateAndPendingRelease(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	expectedAssignedPods := make(map[string]cns.PodInfo)

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	for i := 6; i < 22; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(i)
		secIPConfig := newSecondaryIPConfig(ipaddress, -1)
		ipID := uuid.New()
		secondaryIPConfigs[ipID.String()] = secIPConfig
	}
	pending := func() []string {
		numPending := rand.Intn(len(secondaryIPConfigs)) + 1 //nolint:gosec // weak rand is sufficient in test
		pendingIPs := []string{}
		for k := range secondaryIPConfigs {
			if numPending == 0 {
				break
			}
			pendingIPs = append(pendingIPs, k)
			numPending--
		}
		return pendingIPs
	}()
	req := generateNetworkContainerRequest(secondaryIPConfigs, "reconcileNc1", "-1")
	returnCode := svc.ReconcileIPAMState([]*cns.CreateNetworkContainerRequest{req}, expectedAssignedPods, &v1alpha.NodeNetworkConfig{
		Spec: v1alpha.NodeNetworkConfigSpec{
			IPsNotInUse: pending,
		},
	})
	if returnCode != types.Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}
	validateNetworkRequest(t, *req)
	// confirm that the correct number of IPs are now PendingRelease
	assert.EqualValues(t, len(pending), len(svc.GetPendingReleaseIPConfigs()))
}

func TestReconcileNCWithExistingStateAndPendingRelease(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	for i := 6; i < 22; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(i)
		secIPConfig := newSecondaryIPConfig(ipaddress, -1)
		ipID := uuid.New()
		secondaryIPConfigs[ipID.String()] = secIPConfig
	}
	expectedAssignedPods := map[string]cns.PodInfo{
		"10.0.0.6": cns.NewPodInfo("some-guid-1", "58a0b427-eth0", "reconcilePod1", "PodNS1"),
		"10.0.0.7": cns.NewPodInfo("some-guid-2", "45b9b555-eth0", "reconcilePod2", "PodNS1"),
	}
	pendingIPIDs := func() map[string]cns.PodInfo {
		numPending := rand.Intn(len(secondaryIPConfigs)) + 1 //nolint:gosec // weak rand is sufficient in test
		pendingIPs := map[string]cns.PodInfo{}
		for k := range secondaryIPConfigs {
			if numPending == 0 {
				break
			}
			if _, ok := expectedAssignedPods[secondaryIPConfigs[k].IPAddress]; ok {
				continue
			}
			pendingIPs[k] = nil
			numPending--
		}
		return pendingIPs
	}()
	req := generateNetworkContainerRequest(secondaryIPConfigs, "reconcileNc1", "-1")

	expectedNcCount := len(svc.state.ContainerStatus)
	returnCode := svc.ReconcileIPAMState([]*cns.CreateNetworkContainerRequest{req}, expectedAssignedPods, &v1alpha.NodeNetworkConfig{
		Spec: v1alpha.NodeNetworkConfigSpec{
			IPsNotInUse: maps.Keys(pendingIPIDs),
		},
	})
	if returnCode != types.Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}
	validateNetworkRequest(t, *req)
	// confirm that the correct number of IPs are now PendingRelease
	assert.EqualValues(t, len(pendingIPIDs), len(svc.GetPendingReleaseIPConfigs()))
	validateNCStateAfterReconcile(t, req, expectedNcCount+1, expectedAssignedPods, pendingIPIDs)
}

func TestReconcileNCWithExistingState(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	startingIndex := 6
	for i := 0; i < 4; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress, -1)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}
	req := generateNetworkContainerRequest(secondaryIPConfigs, "reconcileNc1", "-1")

	expectedAssignedPods := map[string]cns.PodInfo{
		"10.0.0.6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
		"10.0.0.7": cns.NewPodInfo("some-guid-2", "def-eth0", "reconcilePod2", "PodNS1"),
	}

	expectedNcCount := len(svc.state.ContainerStatus)
	returnCode := svc.ReconcileIPAMState([]*cns.CreateNetworkContainerRequest{req}, expectedAssignedPods, &v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})
	if returnCode != types.Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}

	validateNCStateAfterReconcile(t, req, expectedNcCount+1, expectedAssignedPods, nil)
}

func TestReconcileCNSIPAMWithDualStackPods(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	secIPv4Configs := make(map[string]cns.SecondaryIPConfig)
	secIPv6Configs := make(map[string]cns.SecondaryIPConfig)

	offset := 6
	for i := 0; i < 4; i++ {
		ipv4 := fmt.Sprintf("10.0.0.%d", offset)
		secIPv4Configs[uuid.New().String()] = newSecondaryIPConfig(ipv4, -1)

		ipv6 := fmt.Sprintf("fe80::%d", offset)
		secIPv6Configs[uuid.New().String()] = newSecondaryIPConfig(ipv6, -1)

		offset++
	}

	ipv4NC := generateNetworkContainerRequest(secIPv4Configs, "reconcileNc1", "-1")
	ipv6NC := generateNetworkContainerRequest(secIPv6Configs, "reconcileNc2", "-1")

	podByIP := map[string]cns.PodInfo{
		"10.0.0.6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
		"fe80::6":  cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),

		"10.0.0.7": cns.NewPodInfo("some-guid-2", "def-eth0", "reconcilePod2", "PodNS1"),
		"fe80::7":  cns.NewPodInfo("some-guid-2", "def-eth0", "reconcilePod2", "PodNS1"),
	}

	ncReqs := []*cns.CreateNetworkContainerRequest{ipv4NC, ipv6NC}

	returnCode := svc.ReconcileIPAMState(ncReqs, podByIP, &v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})

	require.Equal(t, types.Success, returnCode)

	validateIPAMStateAfterReconcile(t, ncReqs, podByIP)
}

func TestReconcileCNSIPAMWithMultipleIPsPerFamilyPerPod(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	secIPv4Configs := make(map[string]cns.SecondaryIPConfig)
	secIPv6Configs := make(map[string]cns.SecondaryIPConfig)

	offset := 6
	for i := 0; i < 4; i++ {
		ipv4 := fmt.Sprintf("10.0.0.%d", offset)
		secIPv4Configs[uuid.New().String()] = newSecondaryIPConfig(ipv4, -1)

		ipv6 := fmt.Sprintf("fe80::%d", offset)
		secIPv6Configs[uuid.New().String()] = newSecondaryIPConfig(ipv6, -1)

		offset++
	}

	ipv4NC := generateNetworkContainerRequest(secIPv4Configs, "reconcileNc1", "-1")
	ipv6NC := generateNetworkContainerRequest(secIPv6Configs, "reconcileNc2", "-1")

	podByIP := map[string]cns.PodInfo{
		"10.0.0.6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
		"10.0.0.7": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
		"fe80::6":  cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
	}

	ncReqs := []*cns.CreateNetworkContainerRequest{ipv4NC, ipv6NC}

	returnCode := svc.ReconcileIPAMState(ncReqs, podByIP, &v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})

	require.Equal(t, types.UnexpectedError, returnCode)
}

func TestPodIPsIndexedByInterface(t *testing.T) {
	tests := []struct {
		name           string
		input          map[string]cns.PodInfo
		expectedErr    error
		expectedOutput map[string]podIPs
	}{
		{
			name: "happy path",
			input: map[string]cns.PodInfo{
				"10.0.0.6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"fe80::6":  cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"10.0.0.7": cns.NewPodInfo("some-guid-2", "def-eth0", "reconcilePod2", "PodNS2"),
				"fe80::7":  cns.NewPodInfo("some-guid-2", "def-eth0", "reconcilePod2", "PodNS2"),
			},
			expectedOutput: map[string]podIPs{
				"reconcilePod1:PodNS1": {
					PodInfo: cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
					v4IP:    net.IPv4(10, 0, 0, 6),
					v6IP:    []byte{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x06},
				},
				"reconcilePod2:PodNS2": {
					PodInfo: cns.NewPodInfo("some-guid-2", "def-eth0", "reconcilePod2", "PodNS2"),
					v4IP:    net.IPv4(10, 0, 0, 7),
					v6IP:    []byte{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x07},
				},
			},
		},
		{
			name: "multiple ipv4 on single stack pod",
			input: map[string]cns.PodInfo{
				"10.0.0.6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"10.0.0.7": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
			},
			expectedErr: errMultipleIPPerFamily,
		},
		{
			name: "multiple ipv4 on dual stack pod",
			input: map[string]cns.PodInfo{
				"10.0.0.6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"10.0.0.7": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"fe80::6":  cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
			},
			expectedErr: errMultipleIPPerFamily,
		},
		{
			name: "multiple ipv6 on single stack pod",
			input: map[string]cns.PodInfo{
				"fe80::6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"fe80::7": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
			},
			expectedErr: errMultipleIPPerFamily,
		},
		{
			name: "multiple ipv6 on dual stack pod",
			input: map[string]cns.PodInfo{
				"10.0.0.6": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"fe80::6":  cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
				"fe80::7":  cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
			},
			expectedErr: errMultipleIPPerFamily,
		},
		{
			name: "malformed ip",
			input: map[string]cns.PodInfo{
				"10.0.0.": cns.NewPodInfo("some-guid-1", "abc-eth0", "reconcilePod1", "PodNS1"),
			},
			expectedErr: errIPParse,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			out, err := newPodKeyToPodIPsMap(tt.input)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}

			require.Equal(t, tt.expectedOutput, out)
		})
	}
}

func TestReconcileNCWithExistingStateFromInterfaceID(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)
	cns.GlobalPodInfoScheme = cns.InterfaceIDPodInfoScheme
	defer func() { cns.GlobalPodInfoScheme = cns.KubernetesPodInfoScheme }()

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	startingIndex := 6
	for i := 0; i < 4; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress, -1)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}
	req := generateNetworkContainerRequest(secondaryIPConfigs, "reconcileNc1", "-1")

	expectedAssignedPods := map[string]cns.PodInfo{
		"10.0.0.6": cns.NewPodInfo("abcdef", "recon1-eth0", "reconcilePod1", "PodNS1"),
		"10.0.0.7": cns.NewPodInfo("abcxyz", "recon2-eth0", "reconcilePod2", "PodNS1"),
	}

	expectedNcCount := len(svc.state.ContainerStatus)
	returnCode := svc.ReconcileIPAMState([]*cns.CreateNetworkContainerRequest{req}, expectedAssignedPods, &v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})
	if returnCode != types.Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}

	validateNCStateAfterReconcile(t, req, expectedNcCount+1, expectedAssignedPods, nil)
}

func TestReconcileCNSIPAMWithKubePodInfoProvider(t *testing.T) {
	restartService()
	setEnv(t)
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)

	startingIndex := 6
	for i := 0; i < 4; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress, -1)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}
	req := generateNetworkContainerRequest(secondaryIPConfigs, uuid.New().String(), "-1")

	// the following pod info constructors leave container id and interface id blank on purpose.
	// this is to simulate the information we get from the kube info provider
	expectedAssignedPods := make(map[string]cns.PodInfo)
	expectedAssignedPods["10.0.0.6"] = cns.NewPodInfo("", "", "customerpod1", "PodNS1")

	// allocate non-vnet IP for pod in host network
	expectedAssignedPods["192.168.0.1"] = cns.NewPodInfo("", "", "systempod", "kube-system")

	expectedNcCount := len(svc.state.ContainerStatus)
	returnCode := svc.ReconcileIPAMState([]*cns.CreateNetworkContainerRequest{req}, expectedAssignedPods, &v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})
	if returnCode != types.Success {
		t.Errorf("Unexpected failure on reconcile with no state %d", returnCode)
	}

	delete(expectedAssignedPods, "192.168.0.1")
	validateNCStateAfterReconcile(t, req, expectedNcCount, expectedAssignedPods, nil)
}

func setOrchestratorTypeInternal(orchestratorType string) {
	fmt.Println("setOrchestratorTypeInternal")
	svc.state.OrchestratorType = orchestratorType
}

func validateCreateNCInternal(t *testing.T, secondaryIpCount int, ncVersion string) {
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	ncVersionInInt, _ := strconv.Atoi(ncVersion)
	startingIndex := 6
	for i := 0; i < secondaryIpCount; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress, ncVersionInInt)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncID, ncVersion)
}

func validateCreateOrUpdateNCInternal(t *testing.T, secondaryIpCount int, ncVersion string) {
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	ncVersionInInt, _ := strconv.Atoi(ncVersion)
	startingIndex := 6
	for i := 0; i < secondaryIpCount; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress, ncVersionInInt)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncID, ncVersion)

	// now Validate Update, add more secondaryIPConfig and it should handle the update
	fmt.Println("Validate Scaleup")
	for i := 0; i < secondaryIpCount; i++ {
		ipaddress := "10.0.0." + strconv.Itoa(startingIndex)
		secIpConfig := newSecondaryIPConfig(ipaddress, ncVersionInInt)
		ipId := uuid.New()
		secondaryIPConfigs[ipId.String()] = secIpConfig
		startingIndex++
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncID, ncVersion)

	// now Scale down, delete 3 ipaddresses from secondaryIPConfig req
	fmt.Println("Validate Scale down")
	count := 0
	for ipid := range secondaryIPConfigs {
		delete(secondaryIPConfigs, ipid)
		count++

		if count > secondaryIpCount {
			break
		}
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncID, ncVersion)

	// Cleanup all SecondaryIps
	fmt.Println("Validate no SecondaryIpconfigs")
	for ipid := range secondaryIPConfigs {
		delete(secondaryIPConfigs, ipid)
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncID, ncVersion)
}

func createAndValidateNCRequest(t *testing.T, secondaryIPConfigs map[string]cns.SecondaryIPConfig, ncId, ncVersion string) {
	req := generateNetworkContainerRequest(secondaryIPConfigs, ncId, ncVersion)
	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req)
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
	}
	_ = svc.IPAMPoolMonitor.Update(&v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})
	validateNetworkRequest(t, *req)
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

	var expectedIPStatus types.IPState
	// 0 is the default NMAgent version return from fake GetNetworkContainerInfoFromHost
	if containerStatus.CreateNetworkContainerRequest.Version > "0" {
		expectedIPStatus = types.PendingProgramming
	} else {
		expectedIPStatus = types.Available
	}
	t.Logf("NC version in container status is %s, HostVersion is %s", containerStatus.CreateNetworkContainerRequest.Version, containerStatus.HostVersion)
	alreadyValidated := make(map[string]string)
	ncCount := 0
	for ipid, ipStatus := range svc.PodIPConfigState {
		ncCount++
		// ignore any IPs that were added from a previous NC
		if ncCount > (len(svc.state.ContainerStatus) * len(svc.PodIPConfigState)) {
			if ipaddress, found := alreadyValidated[ipid]; !found {
				if secondaryIPConfig, ok := req.SecondaryIPConfigs[ipid]; !ok {
					t.Fatalf("PodIpConfigState has stale ipId: %s, config: %+v", ipid, ipStatus)
				} else {
					if ipStatus.IPAddress != secondaryIPConfig.IPAddress {
						t.Fatalf("IPId: %s IPSubnet doesnt match: expected %+v, actual: %+v", ipid, secondaryIPConfig.IPAddress, ipStatus.IPAddress)
					}

					// Validate IP state
					if ipStatus.PodInfo != nil {
						if _, exists := svc.PodIPIDByPodInterfaceKey[ipStatus.PodInfo.Key()]; exists {
							if ipStatus.GetState() != types.Assigned {
								t.Fatalf("IPId: %s State is not Assigned, ipStatus: %+v", ipid, ipStatus)
							}
						} else {
							t.Fatalf("Failed to find podContext for assigned ip: %+v, podinfo :%+v", ipStatus, ipStatus.PodInfo)
						}
					} else if ipStatus.GetState() != expectedIPStatus {
						// Todo: Validate for pendingRelease as well
						t.Fatalf("IPId: %s State is not as expected, ipStatus is : %+v, expected status is %+v", ipid, ipStatus.GetState(), expectedIPStatus)
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
}

func generateNetworkContainerRequest(secondaryIps map[string]cns.SecondaryIPConfig, ncID, ncVersion string) *cns.CreateNetworkContainerRequest {
	var ipConfig cns.IPConfiguration
	ipConfig.DNSServers = dnsservers
	ipConfig.GatewayIPAddress = gatewayIp
	var ipSubnet cns.IPSubnet
	ipSubnet.IPAddress = primaryIp
	ipSubnet.PrefixLength = subnetPrfixLength
	ipConfig.IPSubnet = ipSubnet

	req := cns.CreateNetworkContainerRequest{
		NetworkContainerType: dockerContainerType,
		NetworkContainerid:   ncID,
		IPConfiguration:      ipConfig,
		Version:              ncVersion,
	}

	ncVersionInInt, _ := strconv.Atoi(ncVersion)
	req.SecondaryIPConfigs = make(map[string]cns.SecondaryIPConfig)
	for k, v := range secondaryIps {
		req.SecondaryIPConfigs[k] = v
		ipconfig := req.SecondaryIPConfigs[k]
		ipconfig.NCVersion = ncVersionInInt
	}

	fmt.Printf("NC Request %+v", req)

	return &req
}

func validateNCStateAfterReconcile(t *testing.T, ncRequest *cns.CreateNetworkContainerRequest, expectedNCCount int, expectedAssignedIPs, expectedPendingIPs map[string]cns.PodInfo) {
	if ncRequest == nil {
		// check svc ContainerStatus will be empty
		if len(svc.state.ContainerStatus) != expectedNCCount {
			t.Fatalf("CNS has some stale ContainerStatus, count: %d, state: %+v", len(svc.state.ContainerStatus), svc.state.ContainerStatus)
		}
	} else {
		validateNetworkRequest(t, *ncRequest)
	}

	if len(expectedAssignedIPs) != len(svc.PodIPIDByPodInterfaceKey) {
		t.Fatalf("Unexpected assigned pods, actual: %d, expected: %d", len(svc.PodIPIDByPodInterfaceKey), len(expectedAssignedIPs))
	}

	for ipaddress, podInfo := range expectedAssignedIPs {
		for _, ipID := range svc.PodIPIDByPodInterfaceKey[podInfo.Key()] {
			ipConfigstate := svc.PodIPConfigState[ipID]
			if ipConfigstate.GetState() != types.Assigned {
				t.Fatalf("IpAddress %s is not marked as assigned to Pod: %+v, ipState: %+v", ipaddress, podInfo, ipConfigstate)
			}

			// Validate if IPAddress matches
			if ipConfigstate.IPAddress != ipaddress {
				t.Fatalf("IpAddress %s is not same, for Pod: %+v, actual ipState: %+v", ipaddress, podInfo, ipConfigstate)
			}

			// Validate pod context
			if reflect.DeepEqual(ipConfigstate.PodInfo, podInfo) != true {
				t.Fatalf("OrchestrationContext: is not same, expected: %+v, actual %+v", ipConfigstate.PodInfo, podInfo)
			}

			// Validate this IP belongs to a valid NCRequest
			nc := svc.state.ContainerStatus[ipConfigstate.NCID]
			if _, exists := nc.CreateNetworkContainerRequest.SecondaryIPConfigs[ipConfigstate.ID]; !exists {
				t.Fatalf("Secondary IP config doest exist in NC, ncid: %s, ipId %s", ipConfigstate.NCID, ipConfigstate.ID)
			}
		}
	}

	// validate rest of Secondary IPs in Available state
	if ncRequest != nil {
		for secIPID, secIPConfig := range ncRequest.SecondaryIPConfigs {
			// Validate IP state
			if secIPConfigState, found := svc.PodIPConfigState[secIPID]; found {
				if _, exists := expectedAssignedIPs[secIPConfig.IPAddress]; exists {
					if secIPConfigState.GetState() != types.Assigned {
						t.Fatalf("IPId: %s State is not Assigned, ipStatus: %+v", secIPID, secIPConfigState)
					}
					continue
				}
				if _, exists := expectedPendingIPs[secIPID]; exists {
					if secIPConfigState.GetState() != types.PendingRelease {
						t.Fatalf("IPId: %s State is not PendingRelease, ipStatus: %+v", secIPID, secIPConfigState)
					}
					continue
				}
				if secIPConfigState.GetState() != types.Available {
					t.Fatalf("IPId: %s State is not Available, ipStatus: %+v", secIPID, secIPConfigState)
				}
			} else {
				t.Fatalf("IPId: %s, IpAddress: %+v State doesnt exists in PodIp Map", secIPID, secIPConfig)
			}
		}
	}
}

func validateIPAMStateAfterReconcile(t *testing.T, ncReqs []*cns.CreateNetworkContainerRequest, expectedAssignedIPs map[string]cns.PodInfo) {
	t.Helper()

	interfaceIdx, err := newPodKeyToPodIPsMap(expectedAssignedIPs)
	require.NoError(t, err, "expected IPs contain incorrect state")

	assert.Len(t, svc.PodIPIDByPodInterfaceKey, len(interfaceIdx), "unexepected quantity of interfaces in CNS")

	for ipAddress, podInfo := range expectedAssignedIPs {
		podIPUUIDs, ok := svc.PodIPIDByPodInterfaceKey[podInfo.Key()]
		assert.Truef(t, ok, "no pod uuids for pod info key %s", podInfo.Key())

		for _, ipID := range podIPUUIDs {
			ipConfigstate, ok := svc.PodIPConfigState[ipID]
			assert.Truef(t, ok, "ip id %s not found in CNS pod ip id index", ipID)
			assert.Equalf(t, types.Assigned, ipConfigstate.GetState(), "ip address %s not marked as assigned to pod: %+v, ip config state: %+v", ipAddress, podInfo, ipConfigstate)

			nc, ok := svc.state.ContainerStatus[ipConfigstate.NCID]
			assert.Truef(t, ok, "nc id %s in ip config state %+v not found in CNS container status index", nc, ipConfigstate)

			_, ok = nc.CreateNetworkContainerRequest.SecondaryIPConfigs[ipID]
			assert.Truef(t, ok, "secondary ip id %s not found in nc request", ipID)
		}
	}

	allSecIPsIdx := make(map[string]*cns.CreateNetworkContainerRequest)
	for i := range ncReqs {
		for _, secIPConfig := range ncReqs[i].SecondaryIPConfigs {
			allSecIPsIdx[secIPConfig.IPAddress] = ncReqs[i]
		}
	}

	// validate rest of secondary IPs in available state
	for _, ncReq := range ncReqs {
		for secIPID, secIPConfig := range ncReq.SecondaryIPConfigs {
			secIPConfigState, ok := svc.PodIPConfigState[secIPID]
			assert.True(t, ok)

			if _, isAssigned := expectedAssignedIPs[secIPConfig.IPAddress]; !isAssigned {
				assert.Equal(t, types.Available, secIPConfigState.GetState())
			}
		}
	}
}

func createNCReqInternal(t *testing.T, secondaryIPConfigs map[string]cns.SecondaryIPConfig, ncID, ncVersion string) cns.CreateNetworkContainerRequest {
	req := generateNetworkContainerRequest(secondaryIPConfigs, ncID, ncVersion)
	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req)
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
	}
	_ = svc.IPAMPoolMonitor.Update(&v1alpha.NodeNetworkConfig{
		Status: v1alpha.NodeNetworkConfigStatus{
			Scaler: v1alpha.Scaler{
				BatchSize:               batchSize,
				ReleaseThresholdPercent: releasePercent,
				RequestThresholdPercent: requestPercent,
			},
		},
		Spec: v1alpha.NodeNetworkConfigSpec{
			RequestedIPCount: initPoolSize,
		},
	})
	return *req
}

func restartService() {
	fmt.Println("Restart Service")

	service.Stop()
	if err := startService(); err != nil {
		fmt.Printf("Failed to restart CNS Service. Error: %v", err)
		os.Exit(1)
	}
}

type mockCNIConflistGenerator struct {
	generatedCount int
	mutex          sync.Mutex
}

func (*mockCNIConflistGenerator) Close() error { return nil }
func (m *mockCNIConflistGenerator) Generate() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.generatedCount++
	return nil
}

func (m *mockCNIConflistGenerator) getGeneratedCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.generatedCount
}

// TestCNIConflistGenerationNewNC tests that discovering a new programmed NC in CNS state will trigger CNI conflist generation
func TestCNIConflistGenerationNewNC(t *testing.T) {
	ncID := "some-new-nc" //nolint:goconst // value not shared across tests, can change without issue
	mockgen := &mockCNIConflistGenerator{}
	service := &HTTPRestService{
		cniConflistGenerator: mockgen,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{
				ncID: {
					ID:          ncID,
					HostVersion: "-1",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						Version: "0",
					},
				},
			},
		},
		nma: &fakes.NMAgentClientFake{
			GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
				return nma.NCVersionList{
					Containers: []nma.NCVersion{
						{
							NetworkContainerID: ncID,
							Version:            "0",
						},
					},
				}, nil
			},
		},
	}

	service.SyncHostNCVersion(context.Background(), cns.CRD)
	// CNI conflist gen happens in goroutine so sleep for a second to let it run
	time.Sleep(time.Second)
	assert.Equal(t, 1, mockgen.getGeneratedCount())
}

// TestCNIConflistGenerationExistingNC tests that if the CNS starts up with a NC already in its state, it will still generate the conflist
func TestCNIConflistGenerationExistingNC(t *testing.T) {
	ncID := "some-existing-nc" //nolint:goconst // value not shared across tests, can change without issue
	mockgen := &mockCNIConflistGenerator{}
	service := &HTTPRestService{
		cniConflistGenerator: mockgen,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{
				ncID: {
					ID:          ncID,
					HostVersion: "0",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						Version: "0",
					},
				},
			},
		},
	}

	service.SyncHostNCVersion(context.Background(), cns.CRD)
	// CNI conflist gen happens in goroutine so sleep for a second to let it run
	time.Sleep(time.Second)
	assert.Equal(t, 1, mockgen.getGeneratedCount())
}

// TestCNIConflistGenerationNewNCTwice tests that discovering a new programmed NC in CNS state will trigger CNI conflist generation, but syncing
// the host NC version a second time does not regenerate the conflist (conflist should only get generated once per binary lifetime)
func TestCNIConflistGenerationNewNCTwice(t *testing.T) {
	ncID := "some-new-nc" //nolint:goconst // value not shared across tests, can change without issue
	mockgen := &mockCNIConflistGenerator{}
	service := &HTTPRestService{
		cniConflistGenerator: mockgen,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{
				ncID: {
					ID:          ncID,
					HostVersion: "-1",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						Version: "0",
					},
				},
			},
		},
		nma: &fakes.NMAgentClientFake{
			GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
				return nma.NCVersionList{
					Containers: []nma.NCVersion{
						{
							NetworkContainerID: ncID,
							Version:            "0",
						},
					},
				}, nil
			},
		},
	}

	service.SyncHostNCVersion(context.Background(), cns.CRD)
	// CNI conflist gen happens in goroutine so sleep for a second to let it run
	time.Sleep(time.Second)
	assert.Equal(t, 1, mockgen.getGeneratedCount())

	service.SyncHostNCVersion(context.Background(), cns.CRD)
	// CNI conflist gen happens in goroutine so sleep for a second to let it run
	time.Sleep(time.Second)
	assert.Equal(t, 1, mockgen.getGeneratedCount()) // should still be one
}

// TestCNIConflistNotGenerated tests that the cni conflist is not generated if no NCs are programmed
func TestCNIConflistNotGenerated(t *testing.T) {
	newNCID := "some-new-nc" //nolint:goconst // value not shared across tests, can change without issue
	mockgen := &mockCNIConflistGenerator{}
	service := &HTTPRestService{
		cniConflistGenerator: mockgen,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{
				newNCID: {
					ID:          newNCID,
					HostVersion: "-1",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						Version: "0",
					},
				},
			},
		},
		nma: &fakes.NMAgentClientFake{
			GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
				return nma.NCVersionList{}, nil
			},
		},
	}

	service.SyncHostNCVersion(context.Background(), cns.CRD)
	// CNI conflist gen happens in goroutine so sleep for a second to let it run
	time.Sleep(time.Second)
	assert.Equal(t, 0, mockgen.getGeneratedCount())
}

// TestCNIConflistGenerationOnNMAError tests that the cni conflist is generated as long as we have at least one programmed NC even if
// the call to NMA to list NCs fails
func TestCNIConflistGenerationOnNMAError(t *testing.T) {
	newNCID := "some-new-nc"           //nolint:goconst // value not shared across tests, can change without issue
	existingNCID := "some-existing-nc" //nolint:goconst // value not shared across tests, can change without issue
	mockgen := &mockCNIConflistGenerator{}
	service := &HTTPRestService{
		cniConflistGenerator: mockgen,
		state: &httpRestServiceState{
			ContainerStatus: map[string]containerstatus{
				newNCID: {
					ID:          newNCID,
					HostVersion: "-1",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						Version: "0",
					},
				},
				existingNCID: {
					ID:          existingNCID,
					HostVersion: "0",
					CreateNetworkContainerRequest: cns.CreateNetworkContainerRequest{
						Version: "0",
					},
				},
			},
		},
		nma: &fakes.NMAgentClientFake{
			GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
				return nma.NCVersionList{}, errors.New("some nma error")
			},
		},
	}

	service.SyncHostNCVersion(context.Background(), cns.CRD)
	// CNI conflist gen happens in goroutine so sleep for a second to let it run
	time.Sleep(time.Second)
	assert.Equal(t, 1, mockgen.getGeneratedCount())
}

func TestMustEnsureNoStaleNCs(t *testing.T) {
	tests := []struct {
		name                 string
		storedNCs            map[string]containerstatus
		ipStates             map[string]cns.IPConfigurationStatus
		ipStatesOverrideFunc func(map[string]cns.IPConfigurationStatus) // defaults to available
		ncsFromReconcile     []string
		expectedRemovedNCs   []string
	}{
		{
			name: "normal reconcile, single nc",
			storedNCs: map[string]containerstatus{
				"nc1": {},
			},
			ipStates: map[string]cns.IPConfigurationStatus{
				"aaaa": {IPAddress: "10.0.0.10", NCID: "nc1"},
			},
			ipStatesOverrideFunc: func(ipStates map[string]cns.IPConfigurationStatus) {
				for k, is := range ipStates {
					is.SetState(types.Assigned)
					ipStates[k] = is
				}
			},
			ncsFromReconcile:   []string{"nc1"},
			expectedRemovedNCs: []string{},
		},
		{
			name: "normal reconcile, dual stack ncs",
			storedNCs: map[string]containerstatus{
				"nc1": {},
				"nc2": {},
			},
			ipStates: map[string]cns.IPConfigurationStatus{
				"aaaa": {IPAddress: "10.0.0.10", NCID: "nc1"},
				"aabb": {IPAddress: "fe80::1ff:fe23:4567:890a", NCID: "nc2"},
			},
			ipStatesOverrideFunc: func(ipStates map[string]cns.IPConfigurationStatus) {
				for k, is := range ipStates {
					is.SetState(types.Assigned)
					ipStates[k] = is
				}
			},
			ncsFromReconcile:   []string{"nc1", "nc2"},
			expectedRemovedNCs: []string{},
		},
		{
			name: "stale nc",
			storedNCs: map[string]containerstatus{
				"nc1": {},
			},
			ipStates: map[string]cns.IPConfigurationStatus{
				"aaaa": {IPAddress: "10.0.0.10", NCID: "nc1"},
			},
			ncsFromReconcile:   []string{"nc2"},
			expectedRemovedNCs: []string{"nc1"},
		},
		{
			name: "stale dual stack ncs",
			storedNCs: map[string]containerstatus{
				"nc1": {},
				"nc2": {},
			},
			ipStates: map[string]cns.IPConfigurationStatus{
				"aaaa": {IPAddress: "10.0.0.10", NCID: "nc1"},
				"aabb": {IPAddress: "fe80::1ff:fe23:4567:890a", NCID: "nc2"},
			},
			ncsFromReconcile:   []string{"nc3", "nc4"},
			expectedRemovedNCs: []string{"nc1", "nc2"},
		},
		{
			name: "stale ncs, ips in flight",
			storedNCs: map[string]containerstatus{
				"nc1": {},
				"nc2": {},
			},
			ipStates: map[string]cns.IPConfigurationStatus{
				"aaaa": {IPAddress: "10.0.0.10", NCID: "nc1"},
				"aabb": {IPAddress: "fe80::1ff:fe23:4567:890a", NCID: "nc2"},
			},
			ipStatesOverrideFunc: func(m map[string]cns.IPConfigurationStatus) {
				ip1 := m["aaaa"]
				ip1.SetState(types.PendingRelease)
				m["aaaa"] = ip1

				ip2 := m["aabb"]
				ip2.SetState(types.PendingProgramming)
				m["aabb"] = ip2
			},
			ncsFromReconcile:   []string{"nc3", "nc4"},
			expectedRemovedNCs: []string{"nc1", "nc2"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.ipStatesOverrideFunc != nil {
				tt.ipStatesOverrideFunc(tt.ipStates)
			} else {
				for k, is := range tt.ipStates {
					is.SetState(types.Available)
					tt.ipStates[k] = is
				}
			}

			svc := HTTPRestService{
				store:            store.NewMockStore("foo"),
				state:            &httpRestServiceState{ContainerStatus: tt.storedNCs},
				PodIPConfigState: tt.ipStates,
			}

			require.NotPanics(t, func() {
				svc.MustEnsureNoStaleNCs(tt.ncsFromReconcile)
			})

			for _, expectedRemovedNCID := range tt.expectedRemovedNCs {
				assert.NotContains(t, svc.state.ContainerStatus, expectedRemovedNCID)
			}
		})
	}
}

func TestMustEnsureNoStaleNCs_PanicsWhenIPsFromStaleNCAreAssigned(t *testing.T) {
	staleNC1 := "nc1"
	staleNC2 := "nc2"

	ncs := map[string]containerstatus{staleNC1: {}, staleNC2: {}}

	ipStates := map[string]cns.IPConfigurationStatus{
		"aaaa": {IPAddress: "10.0.0.10", NCID: staleNC1},
		"aabb": {IPAddress: "10.0.0.11", NCID: staleNC1},
		"aacc": {IPAddress: "10.0.0.12", NCID: staleNC2},
		"aadd": {IPAddress: "10.0.0.13", NCID: staleNC2},
	}
	for k, is := range ipStates {
		is.SetState(types.Assigned)
		ipStates[k] = is
	}

	svc := HTTPRestService{
		store:            store.NewMockStore("foo"),
		state:            &httpRestServiceState{ContainerStatus: ncs},
		PodIPConfigState: ipStates,
	}

	require.Panics(t, func() {
		svc.MustEnsureNoStaleNCs([]string{"nc3", "nc4"})
	})
}
