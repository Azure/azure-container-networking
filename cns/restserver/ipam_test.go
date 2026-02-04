package restserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/middlewares"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/cns/types"
	nma "github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testNCID   = "06867cf3-332d-409d-8819-ed70d2c116b0"
	testNCIDv6 = "a69b9217-3d89-4b73-a052-1e8baa453cb0"

	ipPrefixBitsv4 = uint8(24)
	ipPrefixBitsv6 = uint8(120)
	prefixes       = []uint8{ipPrefixBitsv4, ipPrefixBitsv6}

	testIP1      = "10.0.0.1"
	testIP1v6    = "fd12:1234::1"
	testIPID1    = "39a9eb9c-1b29-3def-780a-9876f89a0989"
	testIPID1v6  = "a8989e08-1039-0b90-0284-01939b0937c1"
	testPod1GUID = "898fb8f1-f93e-4c96-9c31-6b89098949a3"
	testPod1Info = cns.NewPodInfo("898fb8-eth0", testPod1GUID, "testpod1", "testpod1namespace")

	testIP2      = "10.0.0.2"
	testIP2v6    = "fd12:1234::2"
	testIPID2    = "92849d9a-2940-3dd0-29a8-1395010da092"
	testIPID2v6  = "492104ba-3021-a329-2849-1038e9430112"
	testPod2GUID = "b21e1ee1-fb7e-4e6d-8c68-22ee5049944e"
	testPod2Info = cns.NewPodInfo("b21e1e-eth0", testPod2GUID, "testpod2", "testpod2namespace")

	testIP3      = "10.0.0.3"
	testIP3v6    = "fd12:1234::3"
	testIPID3    = "3829abc9-3289-3208-2112-fac849ad8492"
	testIPID3v6  = "84fe04ba-fc23-3bc4-2ab4-3423058392ab"
	testPod3GUID = "718e04ac-5a13-4dce-84b3-040accaa9b41"
	testPod3Info = cns.NewPodInfo("718e04-eth0", testPod3GUID, "testpod3", "testpod3namespace")

	testIP4      = "10.0.0.4"
	testPod4GUID = "718e04ac-5a13-4dce-84b3-040accaa9b42"

	testPod8GUID = "2006cad4-e54d-472e-863d-c4bac66200a7"
	testPod8Info = cns.NewPodInfo("2006cad4-eth0", testPod8GUID, "testpod8", "testpod8namespace")

	testPod9GUID = "2006cad4-e54d-472e-863d-c4bac66200a9"
	testPod9Info = cns.NewPodInfo("2006cad4-eth0", testPod9GUID, "testpod9", "testpod9namespace")

	testPod10GUID = "2006cad4-e54d-472e-863d-c4bac66200a9"
	testPod10Info = cns.NewPodInfo("2006cad4-eth0", testPod10GUID, "testpod10", "testpod10namespace")

	ipIDs = [][]string{{testIPID1, testIPID2, testIPID3}, {testIPID1v6, testIPID2v6, testIPID3v6}}
)

// Struct that holds information for NCs that will be used in tests
type ncState struct {
	ncID string
	ips  []string
}

func getTestService(orchestratorType string) *HTTPRestService {
	var config common.ServiceConfig
	httpsvc, _ := NewHTTPRestService(&config, &fakes.WireserverClientFake{}, &fakes.WireserverProxyFake{},
		&IPtablesProvider{}, &fakes.NMAgentClientFake{}, store.NewMockStore(""), nil, nil,
		fakes.NewMockIMDSClient())
	svc = httpsvc
	setOrchestratorTypeInternal(orchestratorType)

	return httpsvc
}

func newSecondaryIPConfig(ipAddress string, ncVersion int) cns.SecondaryIPConfig {
	return cns.SecondaryIPConfig{
		IPAddress: ipAddress,
		NCVersion: ncVersion,
	}
}

func newPodState(ipaddress, id, ncid string, state types.IPState, ncVersion int) cns.IPConfigurationStatus { //nolint:unparam // ignore unused parameter
	ipconfig := newSecondaryIPConfig(ipaddress, ncVersion)
	status := &cns.IPConfigurationStatus{
		IPAddress: ipconfig.IPAddress,
		ID:        id,
		NCID:      ncid,
	}
	status.SetState(state)
	return *status
}

func requestIPAddressAndGetState(t *testing.T, req cns.IPConfigsRequest) ([]cns.IPConfigurationStatus, error) {
	podIPInfo, err := requestIPConfigsHelper(svc, req)
	if err != nil {
		return []cns.IPConfigurationStatus{}, err
	}

	for i := range podIPInfo {
		assert.Equal(t, primaryIP, podIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress)
		assert.Equal(t, subnetPrfixLength, int(podIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength))
		assert.Equal(t, dnsservers, podIPInfo[i].NetworkContainerPrimaryIPConfig.DNSServers)
		assert.Equal(t, gatewayIP, podIPInfo[i].NetworkContainerPrimaryIPConfig.GatewayIPAddress)
		assert.Equal(t, subnetPrfixLength, int(podIPInfo[i].PodIPConfig.PrefixLength))
		assert.Equal(t, fakes.HostPrimaryIP, podIPInfo[i].HostPrimaryIPInfo.PrimaryIP)
		assert.Equal(t, fakes.HostSubnet, podIPInfo[i].HostPrimaryIPInfo.Subnet)
	}

	// retrieve podinfo from orchestrator context
	podInfo, err := cns.NewPodInfoFromIPConfigsRequest(req)
	if err != nil {
		return []cns.IPConfigurationStatus{}, errors.Wrap(err, "failed to unmarshal pod info")
	}

	ipConfigStatus := make([]cns.IPConfigurationStatus, 0)
	for _, ipID := range svc.PodIPIDByPodInterfaceKey[podInfo.Key()] {
		ipConfigStatus = append(ipConfigStatus, svc.PodIPConfigState[ipID])
	}
	return ipConfigStatus, nil
}

// nolint:unparam // ignore unused inputs
func newPodStateWithOrchestratorContext(ipaddress, id, ncid string, state types.IPState, _ uint8, ncVersion int, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) {
	ipconfig := newSecondaryIPConfig(ipaddress, ncVersion)
	status := &cns.IPConfigurationStatus{
		IPAddress: ipconfig.IPAddress,
		ID:        id,
		NCID:      ncid,
		PodInfo:   podInfo,
	}
	status.SetState(state)
	return *status, nil
}

// Test function to populate the IPConfigState
// nolint: unparam // ignore unused return
func updatePodIPConfigState(t *testing.T, svc *HTTPRestService, ipconfigs map[string]cns.IPConfigurationStatus, ncID string) error {
	// Create the NC
	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	// Get each of the ipconfigs associated with that NC
	for _, ipconfig := range ipconfigs { //nolint:gocritic // ignore copy
		secIPConfig := cns.SecondaryIPConfig{
			IPAddress: ipconfig.IPAddress,
			NCVersion: -1,
		}

		ipID := ipconfig.ID
		secondaryIPConfigs[ipID] = secIPConfig
	}

	createAndValidateNCRequest(t, secondaryIPConfigs, ncID, "-1")

	// update ipconfigs to expected state
	for ipID, ipconfig := range ipconfigs { //nolint:gocritic // ignore copy
		if ipconfig.GetState() == types.Assigned {
			svc.PodIPIDByPodInterfaceKey[ipconfig.PodInfo.Key()] = append(svc.PodIPIDByPodInterfaceKey[ipconfig.PodInfo.Key()], ipID)
			svc.PodIPConfigState[ipID] = ipconfig
		}
	}
	return nil
}

func updatePnpIDMacAddressState(svc *HTTPRestService) {
	svc.state.PnpIDByMacAddress = map[string]string{
		"00:00:00:00:00:00": "pnpid/pciid",
	}
}

func TestEndpointStateReadAndWrite(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		endpointStateReadAndWrite(t, ncState)
	}
}

// Tests the creation of an endpoint using the NCs and IPs as input and then tests the deletion of that endpoint
func endpointStateReadAndWrite(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail update service with config: %+v", err)
		}
	}
	t.Log(ipconfigs)

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.Ifname = "eth0"
	podIPInfo, err := requestIPConfigsHelper(svc, req)
	if err != nil {
		t.Fatalf("Expected to not fail getting pod ip info: %+v", err)
	}

	ipInfo := &IPInfo{}
	for i := range podIPInfo {
		ip, ipnet, errIP := net.ParseCIDR(podIPInfo[i].PodIPConfig.IPAddress + "/" + fmt.Sprint(podIPInfo[i].PodIPConfig.PrefixLength))
		if errIP != nil {
			t.Fatalf("failed to parse pod ip address: %+v", errIP)
		}
		ipconfig := net.IPNet{IP: ip, Mask: ipnet.Mask}
		if ip.To4() == nil { // is an ipv6 address
			ipInfo.IPv6 = append(ipInfo.IPv6, ipconfig)
		} else {
			ipInfo.IPv4 = append(ipInfo.IPv4, ipconfig)
		}
	}

	// add
	desiredState := map[string]*EndpointInfo{req.InfraContainerID: {PodName: testPod1Info.Name(), PodNamespace: testPod1Info.Namespace(), IfnameToIPMap: map[string]*IPInfo{req.Ifname: ipInfo}}}
	err = svc.updateEndpointState(req, testPod1Info, podIPInfo)
	if err != nil {
		t.Fatalf("Expected to not fail updating endpoint state: %+v", err)
	}
	assert.Equal(t, desiredState, svc.EndpointState)

	// consecutive add of same endpoint should not change state or cause error
	err = svc.updateEndpointState(req, testPod1Info, podIPInfo)
	if err != nil {
		t.Fatalf("Expected to not fail updating existing endpoint state: %+v", err)
	}
	assert.Equal(t, desiredState, svc.EndpointState)

	// delete
	desiredState = map[string]*EndpointInfo{}
	err = svc.removeEndpointState(testPod1Info)
	if err != nil {
		t.Fatalf("Expected to not fail removing endpoint state: %+v", err)
	}
	assert.Equal(t, desiredState, svc.EndpointState)

	// delete non-existent endpoint should not change state or cause error
	err = svc.removeEndpointState(testPod1Info)
	if err != nil {
		t.Fatalf("Expected to not fail removing non existing key: %+v", err)
	}
	assert.Equal(t, desiredState, svc.EndpointState)
}

// assign the available IP to the new pod
func TestIPAMGetAvailableIPConfig(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamGetAvailableIPConfig(t, ncState)
	}
}

// Add one IP per NC to the pool and request those IPs
func ipamGetAvailableIPConfig(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b

	actualState, err := requestIPAddressAndGetState(t, req)
	if err != nil {
		t.Fatal("Expected IP retrieval error to be nil")
	}

	desiredState := make([]cns.IPConfigurationStatus, len(ncStates))
	for i := range ncStates {
		desiredState[i] = newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, 0)
		desiredState[i].PodInfo = testPod1Info
	}

	// desiredState is expecting IPv4 to be first so if we get IPv6 first then we need to switch them
	firstAddress, _ := netip.ParseAddr(actualState[0].IPAddress)
	if firstAddress.Is4() == false && len(actualState) > 1 {
		actualState[0], actualState[1] = actualState[1], actualState[0]
	}
	for i := range actualState {
		assert.Equal(t, desiredState[i].GetState(), actualState[i].GetState())
		assert.Equal(t, desiredState[i].ID, actualState[i].ID)
		assert.Equal(t, desiredState[i].IPAddress, actualState[i].IPAddress)
		assert.Equal(t, desiredState[i].NCID, actualState[i].NCID)
		assert.Equal(t, desiredState[i].PodInfo, actualState[i].PodInfo)
	}
}

func TestIPAMGetNextAvailableIPConfig(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
					testIP2v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
					testIP1v6,
					testIP2v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		getNextAvailableIPConfig(t, ncState)
	}
}

// First IP is already assigned to a pod, want second IP
func getNextAvailableIPConfig(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	// Add already assigned pod ip to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()] = append(svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()], ncStates[i].ips[0])
		state1, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		state2 := newPodState(ncStates[i].ips[1], ipIDs[i][1], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ := testPod2Info.OrchestratorContext()
	req.OrchestratorContext = b

	actualState, err := requestIPAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}
	// want second available Pod IP State as first has been assigned
	desiredState := make([]cns.IPConfigurationStatus, len(ncStates))
	for i := range ncStates {
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[1], ipIDs[i][1], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod2Info)
		desiredState[i] = state
	}

	// desiredState is expecting IPv4 to be first so if we get IPv6 first then we need to switch them
	firstAddress, _ := netip.ParseAddr(actualState[0].IPAddress)
	if firstAddress.Is4() == false && len(actualState) > 1 {
		actualState[0], actualState[1] = actualState[1], actualState[0]
	}
	for i := range actualState {
		assert.Equal(t, desiredState[i].GetState(), actualState[i].GetState())
		assert.Equal(t, desiredState[i].ID, actualState[i].ID)
		assert.Equal(t, desiredState[i].IPAddress, actualState[i].IPAddress)
		assert.Equal(t, desiredState[i].NCID, actualState[i].NCID)
		assert.Equal(t, desiredState[i].PodInfo, actualState[i].PodInfo)
	}
}

func TestIPAMGetAlreadyAssignedIPConfigForSamePod(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamGetAlreadyAssignedIPConfigForSamePod(t, ncState)
	}
}

func ipamGetAlreadyAssignedIPConfigForSamePod(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	// Add Assigned Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b

	actualState, err := requestIPAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected not error: %+v", err)
	}
	desiredState := make([]cns.IPConfigurationStatus, len(ncStates))
	for i := range ncStates {
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		desiredState[i] = state
	}

	// desiredState is expecting IPv4 to be first so if we get IPv6 first then we need to switch them
	firstAddress, _ := netip.ParseAddr(actualState[0].IPAddress)
	if firstAddress.Is4() == false && len(actualState) > 1 {
		actualState[0], actualState[1] = actualState[1], actualState[0]
	}
	for i := range actualState {
		assert.Equal(t, desiredState[i].GetState(), actualState[i].GetState())
		assert.Equal(t, desiredState[i].ID, actualState[i].ID)
		assert.Equal(t, desiredState[i].IPAddress, actualState[i].IPAddress)
		assert.Equal(t, desiredState[i].NCID, actualState[i].NCID)
		assert.Equal(t, desiredState[i].PodInfo, actualState[i].PodInfo)
	}
}

func TestIPAMAttemptToRequestIPNotFoundInPool(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
					testIP2v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
					testIP1v6,
					testIP2v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamAttemptToRequestIPNotFoundInPool(t, ncState)
	}
}

func ipamAttemptToRequestIPNotFoundInPool(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ := testPod2Info.OrchestratorContext()
	req.OrchestratorContext = b
	for i := range ncStates {
		req.DesiredIPAddresses = append(req.DesiredIPAddresses, ncStates[i].ips[1])
	}

	_, err := requestIPAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected to fail as IP not found in pool")
	}
}

func TestIPAMGetDesiredIPConfigWithSpecfiedIP(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamGetDesiredIPConfigWithSpecfiedIP(t, ncState)
	}
}

func ipamGetDesiredIPConfigWithSpecfiedIP(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	for i := range ncStates {
		req.DesiredIPAddresses = append(req.DesiredIPAddresses, ncStates[i].ips[0])
	}

	actualState, err := requestIPAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}

	desiredState := make([]cns.IPConfigurationStatus, len(ncStates))
	for i := range ncStates {
		desiredState[i] = newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, 0)
		desiredState[i].PodInfo = testPod1Info
	}

	// desiredState is expecting IPv4 to be first so if we get IPv6 first then we need to switch them
	firstAddress, _ := netip.ParseAddr(actualState[0].IPAddress)
	if firstAddress.Is4() == false && len(actualState) > 1 {
		actualState[0], actualState[1] = actualState[1], actualState[0]
	}
	for i := range actualState {
		assert.Equal(t, desiredState[i].GetState(), actualState[i].GetState())
		assert.Equal(t, desiredState[i].ID, actualState[i].ID)
		assert.Equal(t, desiredState[i].IPAddress, actualState[i].IPAddress)
		assert.Equal(t, desiredState[i].NCID, actualState[i].NCID)
		assert.Equal(t, desiredState[i].PodInfo, actualState[i].PodInfo)
	}
}

func TestIPAMFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIP(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIP(t, ncState)
	}
}

func ipamFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIP(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	// request the already assigned ip with a new context
	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ := testPod2Info.OrchestratorContext()
	req.OrchestratorContext = b
	for i := range ncStates {
		req.DesiredIPAddresses = append(req.DesiredIPAddresses, ncStates[i].ips[0])
	}

	_, err := requestIPAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected failure requesting already assigned IP: %+v", err)
	}
}

func TestIPAMFailToGetIPWhenAllIPsAreAssigned(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
					testIP2v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
					testIP1v6,
					testIP2v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamFailToGetIPWhenAllIPsAreAssigned(t, ncState)
	}
}

func ipamFailToGetIPWhenAllIPsAreAssigned(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	// Add already assigned pod ip to state
	for i := range ncStates {
		state1, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		state2, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[1], ipIDs[i][1], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod2Info)
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	// request the already assigned ip with a new context
	req := cns.IPConfigsRequest{}
	b, _ := testPod3Info.OrchestratorContext()
	req.OrchestratorContext = b

	_, err := requestIPAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected failure requesting IP when there are no more IPs: %+v", err)
	}
}

func TestIPAMRequestThenReleaseThenRequestAgain(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamRequestThenReleaseThenRequestAgain(t, ncState)
	}
}

// 10.0.0.1 = PodInfo1
// Request 10.0.0.1 with PodInfo2 (Fail)
// Release PodInfo1
// Request 10.0.0.1 with PodInfo2 (Success)
func ipamRequestThenReleaseThenRequestAgain(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	// set state as already assigned
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	// Use TestPodInfo2 to request TestIP1, which has already been assigned
	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ := testPod2Info.OrchestratorContext()
	req.OrchestratorContext = b
	for i := range ncStates {
		req.DesiredIPAddresses = append(req.DesiredIPAddresses, ncStates[i].ips[0])
	}

	_, err := requestIPAddressAndGetState(t, req)
	if err == nil {
		t.Fatal("Expected failure requesting IP when there are no more IPs")
	}

	// Release Test Pod 1
	err = svc.releaseIPConfigs(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Rerequest
	req = cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ = testPod2Info.OrchestratorContext()
	req.OrchestratorContext = b
	for i := range ncStates {
		req.DesiredIPAddresses = append(req.DesiredIPAddresses, ncStates[i].ips[0])
	}

	actualState, err := requestIPAddressAndGetState(t, req)
	if err != nil {
		t.Fatalf("Expected IP retrieval to be nil: %+v", err)
	}

	desiredState := make([]cns.IPConfigurationStatus, len(ncStates))
	for i := range ncStates {
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		// want first available Pod IP State
		desiredState[i] = state
		desiredState[i].IPAddress = ncStates[i].ips[0]
		desiredState[i].PodInfo = testPod2Info
	}

	// desiredState is expecting IPv4 to be first so if we get IPv6 first then we need to switch them
	firstAddress, _ := netip.ParseAddr(actualState[0].IPAddress)
	if firstAddress.Is4() == false && len(actualState) > 1 {
		actualState[0], actualState[1] = actualState[1], actualState[0]
	}
	for i := range actualState {
		assert.Equal(t, desiredState[i].GetState(), actualState[i].GetState())
		assert.Equal(t, desiredState[i].ID, actualState[i].ID)
		assert.Equal(t, desiredState[i].IPAddress, actualState[i].IPAddress)
		assert.Equal(t, desiredState[i].NCID, actualState[i].NCID)
		assert.Equal(t, desiredState[i].PodInfo, actualState[i].PodInfo)
	}
}

func TestIPAMReleaseIPIdempotency(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamReleaseIPIdempotency(t, ncState)
	}
}

func ipamReleaseIPIdempotency(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)
	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	// Release Test Pod 1
	err := svc.releaseIPConfigs(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Call release again, should be fine
	err = svc.releaseIPConfigs(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}
}

func TestIPAMAllocateIPIdempotency(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamAllocateIPIdempotency(t, ncState)
	}
}

func ipamAllocateIPIdempotency(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)
	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}

		err = updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}
}

func TestAvailableIPConfigs(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
					testIP3,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
					testIP3,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
					testIP2v6,
					testIP3v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
					testIP3,
					testIP1v6,
					testIP2v6,
					testIP3v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		availableIPConfigs(t, ncState)
	}
}

func availableIPConfigs(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	IDsToBeDeleted := make([]string, len(ncStates))
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	// Add already assigned pod ip to state
	for i := range ncStates {
		state1 := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		state2 := newPodState(ncStates[i].ips[1], ipIDs[i][1], ncStates[i].ncID, types.Available, 0)
		state3 := newPodState(ncStates[i].ips[2], ipIDs[i][2], ncStates[i].ncID, types.Available, 0)
		IDsToBeDeleted[i] = state1.ID
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		ipconfigs[state3.ID] = state3
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	desiredAvailableIps := make(map[string]cns.IPConfigurationStatus, 0)
	for ID := range ipconfigs {
		desiredAvailableIps[ID] = ipconfigs[ID]
	}

	availableIps := svc.GetAvailableIPConfigs()
	validateIpState(t, availableIps, desiredAvailableIps)

	desiredAssignedIPConfigs := make(map[string]cns.IPConfigurationStatus)
	assignedIPs := svc.GetAssignedIPConfigs()
	validateIpState(t, assignedIPs, desiredAssignedIPConfigs)

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	for i := range ncStates {
		req.DesiredIPAddresses = append(req.DesiredIPAddresses, ncStates[i].ips[0])
	}

	_, err := requestIPAddressAndGetState(t, req)
	if err != nil {
		t.Fatal("Expected IP retrieval to be nil")
	}
	for i := range IDsToBeDeleted {
		delete(desiredAvailableIps, IDsToBeDeleted[i])
	}
	availableIps = svc.GetAvailableIPConfigs()
	validateIpState(t, availableIps, desiredAvailableIps)

	for i := range ncStates {
		desiredState := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, 0)
		desiredState.PodInfo = testPod1Info
		desiredAssignedIPConfigs[desiredState.ID] = desiredState
	}

	assignedIPs = svc.GetAssignedIPConfigs()
	validateIpState(t, assignedIPs, desiredAssignedIPConfigs)
}

func validateIpState(t *testing.T, actualIps []cns.IPConfigurationStatus, expectedList map[string]cns.IPConfigurationStatus) {
	if len(actualIps) != len(expectedList) {
		t.Fatalf("Actual and expected  count doesnt match, expected %d, actual %d", len(actualIps), len(expectedList))
	}

	for _, actualIP := range actualIps { //nolint:gocritic // ignore copy
		var expectedIP cns.IPConfigurationStatus
		var found bool
		for _, expectedIP = range expectedList { //nolint:gocritic // ignore copy
			if expectedIP.Equals(actualIP) {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("Actual and expected list doesnt match actual: %+v, expected: %+v", actualIP, expectedIP)
		}
	}
}

func TestIPAMMarkIPCountAsPending(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP1v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamMarkIPCountAsPending(t, ncState)
	}
}

func ipamMarkIPCountAsPending(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)
	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	// Release Test Pod 1
	ips, err := svc.MarkIPAsPendingRelease(len(ncStates))
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	for i := range ncStates {
		if _, exists := ips[ipIDs[i][0]]; !exists {
			t.Fatalf("Expected ID not marked as pending: %+v", err)
		}
	}

	// Release Test Pod 1
	pendingrelease := svc.GetPendingReleaseIPConfigs()
	if len(pendingrelease) != len(ncStates) {
		t.Fatal("Expected pending release slice to be nonzero after pending release")
	}

	available := svc.GetAvailableIPConfigs()
	if len(available) != 0 {
		t.Fatal("Expected available ips to be zero after marked as pending")
	}

	// Call release again, should be fine
	err = svc.releaseIPConfigs(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Try to release IP when no IP can be released. It will not return error and return 0 IPs
	ips, err = svc.MarkIPAsPendingRelease(1)
	if err != nil || len(ips) != 0 {
		t.Fatalf("We are not either expecting err [%v] or ips as non empty [%v]", err, ips)
	}
}

func TestIPAMMarkIPAsPendingWithPendingProgrammingIPs(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)

	secondaryIPConfigs := make(map[string]cns.SecondaryIPConfig)
	// Default Programmed NC version is -1, set nc version as 0 will result in pending programming state.
	constructSecondaryIPConfigs(testIP1, testPod1GUID, 0, secondaryIPConfigs)
	constructSecondaryIPConfigs(testIP3, testPod3GUID, 0, secondaryIPConfigs)
	// Default Programmed NC version is -1, set nc version as -1 will result in available state.
	constructSecondaryIPConfigs(testIP2, testPod2GUID, -1, secondaryIPConfigs)
	constructSecondaryIPConfigs(testIP4, testPod4GUID, -1, secondaryIPConfigs)

	// createNCRequest with NC version 0
	req := generateNetworkContainerRequest(secondaryIPConfigs, testNCID, strconv.Itoa(0))
	returnCode := svc.CreateOrUpdateNetworkContainerInternal(req)
	if returnCode != 0 {
		t.Fatalf("Failed to createNetworkContainerRequest, req: %+v, err: %d", req, returnCode)
	}
	// Release pending programming IPs
	ips, err := svc.MarkIPAsPendingRelease(2)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}
	// Check returning released IPs are from pod 1 and 3
	if _, exists := ips[testPod1GUID]; !exists {
		t.Fatalf("Expected ID not marked as pending: %+v, ips is %v", err, ips)
	}
	if _, exists := ips[testPod3GUID]; !exists {
		t.Fatalf("Expected ID not marked as pending: %+v, ips is %v", err, ips)
	}

	pendingRelease := svc.GetPendingReleaseIPConfigs()
	if len(pendingRelease) != 2 {
		t.Fatalf("Expected 2 pending release IPs but got %d pending release IP", len(pendingRelease))
	}
	// Check pending release IDs are from pod 1 and 3
	for _, config := range pendingRelease {
		if config.ID != testPod1GUID && config.ID != testPod3GUID {
			t.Fatalf("Expected pending release ID is either from pod 1 or pod 3 but got ID as %s ", config.ID)
		}
	}

	available := svc.GetAvailableIPConfigs()
	if len(available) != 2 {
		t.Fatalf("Expected 1 available IP with test pod 2 but got available %d IP", len(available))
	}

	// Call release again, should be fine
	err = svc.releaseIPConfigs(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Release 2 more IPs
	ips, err = svc.MarkIPAsPendingRelease(2)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}
	// Make sure newly released IPs are from pod 2 and pod 4
	if _, exists := ips[testPod2GUID]; !exists {
		t.Fatalf("Expected ID not marked as pending: %+v, ips is %v", err, ips)
	}
	if _, exists := ips[testPod4GUID]; !exists {
		t.Fatalf("Expected ID not marked as pending: %+v, ips is %v", err, ips)
	}

	// Get all pending release IPs and check total number is 4
	pendingRelease = svc.GetPendingReleaseIPConfigs()
	if len(pendingRelease) != 4 {
		t.Fatalf("Expected 4 pending release IPs but got %d pending release IP", len(pendingRelease))
	}
}

func constructSecondaryIPConfigs(ipAddress, uuid string, ncVersion int, secondaryIPConfigs map[string]cns.SecondaryIPConfig) {
	secIPConfig := cns.SecondaryIPConfig{
		IPAddress: ipAddress,
		NCVersion: ncVersion,
	}
	secondaryIPConfigs[uuid] = secIPConfig
}

func TestIPAMMarkExistingIPConfigAsPending(t *testing.T) {
	testNcs := [][]ncState{
		// single stack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
		},
		// two NCs (one V4 and one V6)
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
				},
			},
			{
				ncID: testNCIDv6,
				ips: []string{
					testIP1v6,
					testIP2v6,
				},
			},
		},
		// dualstack NC
		{
			{
				ncID: testNCID,
				ips: []string{
					testIP1,
					testIP2,
					testIP1v6,
					testIP2v6,
				},
			},
		},
	}
	for _, ncState := range testNcs {
		ipamMarkExistingIPConfigAsPending(t, ncState)
	}
}

func ipamMarkExistingIPConfigAsPending(t *testing.T, ncStates []ncState) {
	svc := getTestService(cns.KubernetesCRD)

	// Add already assigned pod ip to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()] = append(svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()], ncStates[i].ips[0])
		state1, _ := newPodStateWithOrchestratorContext(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		state2 := newPodState(ncStates[i].ips[1], ipIDs[i][1], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	pendingIPIDs := make([]string, len(ncStates))
	// mark available ip as as pending
	for i := range ncStates {
		pendingIPIDs[i] = ipIDs[i][1]
	}
	err := svc.MarkExistingIPsAsPendingRelease(pendingIPIDs)
	if err != nil {
		t.Fatalf("Expected to successfully mark available ip as pending")
	}

	pendingIPConfigs := svc.GetPendingReleaseIPConfigs()
	firstAddress, _ := netip.ParseAddr(pendingIPConfigs[0].IPAddress)
	if firstAddress.Is4() == false && len(pendingIPConfigs) > 1 {
		pendingIPConfigs[0], pendingIPConfigs[1] = pendingIPConfigs[1], pendingIPConfigs[0]
	}
	for i := range ncStates {
		if pendingIPConfigs[i].ID != ipIDs[i][1] {
			t.Fatalf("Expected to see ID %v in pending release ipconfigs, actual %+v", ipIDs[i][1], pendingIPConfigs)
		}
	}

	// attempt to mark assigned ipconfig as pending, expect fail
	for i := range ncStates {
		pendingIPIDs[i] = ipIDs[i][0]
	}
	err = svc.MarkExistingIPsAsPendingRelease(pendingIPIDs)
	if err == nil {
		t.Fatalf("Expected to fail when marking assigned ip as pending")
	}

	assignedIPConfigs := svc.GetAssignedIPConfigs()
	for i := range ncStates {
		found := false
		for j := range assignedIPConfigs {
			if assignedIPConfigs[j].ID == ipIDs[i][0] {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Expected to see ID %v in pending release ipconfigs, actual %+v", ipIDs[i][0], assignedIPConfigs)
		}
	}
}

func TestIPAMFailToRequestIPsWithNoNCsSpecificIP(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 1)
	req.DesiredIPAddresses[0] = testIP1

	_, err := requestIPConfigsHelper(svc, req)
	if err == nil {
		t.Fatalf("Expected error. Should not be able to request IPs when there are no NCs")
	}
	assert.ErrorIs(t, err, ErrNoNCs)
}

func TestIPAMFailToRequestIPsWithNoNCsAnyIP(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b

	_, err := requestIPConfigsHelper(svc, req)
	if err == nil {
		t.Fatalf("Expected error. Should not be able to request IPs when there are no NCs")
	}
	assert.ErrorIs(t, err, ErrNoNCs)
}

func TestIPAMReleaseOneIPWhenExpectedToHaveTwo(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)

	// set state as already assigned
	testState, _ := newPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, types.Assigned, 24, 0, testPod1Info)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}
	emptyIpconfigs := map[string]cns.IPConfigurationStatus{}

	err := updatePodIPConfigState(t, svc, ipconfigs, testNCID)
	if err != nil {
		t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
	}
	err = updatePodIPConfigState(t, svc, emptyIpconfigs, testNCIDv6)
	if err != nil {
		t.Fatalf("Expected to not fail adding empty NC to state: %+v", err)
	}

	err = svc.releaseIPConfigs(testPod1Info)
	if err != nil {
		t.Fatalf("Expected success releasing IP")
	}

	available := svc.GetAvailableIPConfigs()
	if len(available) == 0 {
		t.Fatal("Expected available ips to be one since we release partial IPs")
	}
}

func TestIPAMFailToRequestOneIPWhenExpectedToHaveTwo(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	// set state as already assigned
	testState1, _ := newPodStateWithOrchestratorContext(testIP1, testIPID1, testNCID, types.Assigned, 24, 0, testPod1Info)
	testState2 := newPodState(testIP2, testIPID2, testNCID, types.Available, 0)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState1.ID: testState1,
		testState2.ID: testState2,
	}
	testState1v6, _ := newPodStateWithOrchestratorContext(testIP1v6, testIPID1v6, testNCIDv6, types.Assigned, 120, 0, testPod1Info)
	// setting v6 IPs to Assigned so there are none available
	ipconfigsV6 := map[string]cns.IPConfigurationStatus{
		testState1v6.ID: testState1v6,
	}

	err := updatePodIPConfigState(t, svc, ipconfigs, testNCID)
	if err != nil {
		t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
	}

	err = updatePodIPConfigState(t, svc, ipconfigsV6, testNCIDv6)
	if err != nil {
		t.Fatalf("Expected to not fail adding empty NC to state: %+v", err)
	}

	// request should expect 2 IPs but there is only 1 in the pool
	req := cns.IPConfigsRequest{}
	b, _ := testPod2Info.OrchestratorContext()
	req.OrchestratorContext = b

	_, err = requestIPAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected failure requesting IP when there are not enough IPs: %+v", err)
	}

	available := svc.GetAvailableIPConfigs()
	if len(available) != 1 {
		t.Fatal("Expected available ips to be one since we expect the IP to not be assigned")
	}
}

func TestIPAMFailToReleasePartialIPsInPool(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)

	// set state as already assigned
	testState, _ := newPodStateWithOrchestratorContext(testIP1, testIPID1, testNCID, types.Assigned, 24, 0, testPod1Info)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}
	testStatev6, _ := newPodStateWithOrchestratorContext(testIP1v6, testIPID1v6, testNCIDv6, types.Assigned, 120, 0, testPod1Info)
	ipconfigsv6 := map[string]cns.IPConfigurationStatus{
		testStatev6.ID: testStatev6,
	}

	err := updatePodIPConfigState(t, svc, ipconfigs, testNCID)
	if err != nil {
		t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
	}
	err = updatePodIPConfigState(t, svc, ipconfigsv6, testNCIDv6)
	if err != nil {
		t.Fatalf("Expected to not fail adding empty NC to state: %+v", err)
	}
	// remove the IP from the from the ipconfig map so that it throws an error when trying to release one of the IPs
	delete(svc.PodIPConfigState, testStatev6.ID)

	err = svc.releaseIPConfigs(testPod1Info)
	if err == nil {
		t.Fatalf("Expected fail releasing IP due to only having one in the ipconfig map, IPs will be reassigned back to the pod")
	}
}

func TestIPAMFailToRequestPartialIPsInPool(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)

	// set state as already assigned
	testState := newPodState(testIP1, testIPID1, testNCID, types.Available, 0)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}
	testStatev6 := newPodState(testIP1v6, testIPID1v6, testNCIDv6, types.Available, 0)
	ipconfigsv6 := map[string]cns.IPConfigurationStatus{
		testStatev6.ID: testStatev6,
	}

	err := updatePodIPConfigState(t, svc, ipconfigs, testNCID)
	if err != nil {
		t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
	}
	err = updatePodIPConfigState(t, svc, ipconfigsv6, testNCIDv6)
	if err != nil {
		t.Fatalf("Expected to not fail adding empty NC to state: %+v", err)
	}
	// remove the IP from the from the ipconfig map so that it throws an error when trying to release one of the IPs
	delete(svc.PodIPConfigState, testStatev6.ID)

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 2)
	req.DesiredIPAddresses[0] = testIP1
	req.DesiredIPAddresses[1] = testIP1v6

	_, err = requestIPAddressAndGetState(t, req)
	if err == nil {
		t.Fatalf("Expected fail requesting IPs due to only having one in the ipconfig map, IPs in the pool will not be assigned")
	}
}

func TestIPAMReleaseSWIFTV2PodIPSuccess(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	middleware := middlewares.K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	svc.AttachIPConfigsHandlerMiddleware(&middleware)

	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.2.10/24")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.0.3.10/24")

	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
		{
			ncID: testNCIDv6,
			ips: []string{
				testIP1v6,
			},
		},
	}

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 2)
	req.DesiredIPAddresses[0] = testIP1
	req.DesiredIPAddresses[1] = testIP1v6
	// Requesting release ip config for SWIFT V2 pod when mtpnc is not ready, should be a no-op
	_, err := svc.ReleaseIPConfigHandlerHelper(context.TODO(), req)
	if err != nil {
		t.Fatalf("Expected not to fail when requesting to release SWIFT V2 pod due to MTPNC not ready")
	}
}

func TestIPAMGetK8sSWIFTv2IPSuccess(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	middleware := middlewares.K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	svc.AttachIPConfigsHandlerMiddleware(&middleware)

	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.2.10/24")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.0.3.10/24")

	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
		{
			ncID: testNCIDv6,
			ips: []string{
				testIP1v6,
			},
		},
	}

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 2)
	req.DesiredIPAddresses[0] = testIP1
	req.DesiredIPAddresses[1] = testIP1v6

	wrappedHandler := svc.IPConfigsHandlerMiddleware.IPConfigsRequestHandlerWrapper(svc.requestIPConfigHandlerHelper, svc.ReleaseIPConfigHandlerHelper)
	resp, err := wrappedHandler(context.TODO(), req)
	if err != nil {
		t.Fatalf("Expected to not fail requesting IPs: %+v", err)
	}
	podIPInfo := resp.PodIPInfo

	if len(podIPInfo) != 3 {
		t.Fatalf("Expected to get 3 pod IP info (IPv4, IPv6, Multitenant IP), actual %d", len(podIPInfo))
	}

	// Asserting that SWIFT v2 IP is returned
	assert.Equal(t, SWIFTv2IP, podIPInfo[2].PodIPConfig.IPAddress)
	assert.Equal(t, SWIFTv2MAC, podIPInfo[2].MacAddress)
	assert.Equal(t, cns.DelegatedVMNIC, podIPInfo[2].NICType)
	assert.False(t, podIPInfo[2].SkipDefaultRoutes)
}

func TestIPAMGetK8sSWIFTv2IPFailure(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	middleware := middlewares.K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	svc.AttachIPConfigsHandlerMiddleware(&middleware)
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
		{
			ncID: testNCIDv6,
			ips: []string{
				testIP1v6,
			},
		},
	}
	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}
	// MTPNC not ready for this pod
	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ := testPod2Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 2)
	req.DesiredIPAddresses[0] = testIP1
	req.DesiredIPAddresses[1] = testIP1v6
	wrappedHandler := svc.IPConfigsHandlerMiddleware.IPConfigsRequestHandlerWrapper(svc.requestIPConfigHandlerHelper, svc.ReleaseIPConfigHandlerHelper)
	_, err := wrappedHandler(context.TODO(), req)
	if err == nil {
		t.Fatalf("Expected failing requesting IPs due to MTPNC not ready")
	}
	available := svc.GetAvailableIPConfigs()
	if len(available) != 2 {
		t.Fatalf("Expected available ips to be 2 since we expect the IP to not be assigned. Available IPs: %d", len(available))
	}

	// MTPNC is ready for this pod but env vars not set
	req = cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ = testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 2)
	req.DesiredIPAddresses[0] = testIP1
	req.DesiredIPAddresses[1] = testIP1v6

	_, err = wrappedHandler(context.TODO(), req)
	if err == nil {
		t.Fatalf("Expected failing requesting IPs due to not able to set routes")
	}

	available = svc.GetAvailableIPConfigs()
	if len(available) != 2 {
		t.Fatal("Expected available ips to be 2 since we expect the IP to not be assigned")
	}
}

// Test intednd to check for on single backend nic without the delegaed nic
func TestIPAMGetK8sInfinibandSuccessOneNic(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	middleware := middlewares.K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	svc.AttachIPConfigsHandlerMiddleware(&middleware)
	updatePnpIDMacAddressState(svc)

	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.2.10/24")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.0.3.10/24")

	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
		{
			ncID: testNCIDv6,
			ips: []string{
				testIP1v6,
			},
		},
	}

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod9Info.InterfaceID(),
		InfraContainerID: testPod9Info.InfraContainerID(),
	}
	b, _ := testPod9Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 2)
	req.DesiredIPAddresses[0] = testIP1
	req.DesiredIPAddresses[1] = testIP1v6

	wrappedHandler := svc.IPConfigsHandlerMiddleware.IPConfigsRequestHandlerWrapper(svc.requestIPConfigHandlerHelper, svc.ReleaseIPConfigHandlerHelper)
	resp, err := wrappedHandler(context.TODO(), req)
	if err != nil {
		t.Fatalf("Expected to not fail requesting IPs: %+v", err)
	}
	podIPInfo := resp.PodIPInfo

	if len(podIPInfo) != 3 {
		t.Fatalf("Expected to get 3 pod IP info (IPv4, IPv6, Multitenant IP), actual %d", len(podIPInfo))
	}

	assert.Equal(t, cns.BackendNIC, podIPInfo[0].NICType)
}

func TestIPAMGetK8sInfinibandFailure(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	middleware := middlewares.K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	svc.AttachIPConfigsHandlerMiddleware(&middleware)
	updatePnpIDMacAddressState(svc)

	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.2.10/24")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.0.3.10/24")

	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
		{
			ncID: testNCIDv6,
			ips: []string{
				testIP1v6,
			},
		},
	}

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod10Info.InterfaceID(),
		InfraContainerID: testPod10Info.InfraContainerID(),
	}
	b, _ := testPod10Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.DesiredIPAddresses = make([]string, 2)
	req.DesiredIPAddresses[0] = testIP1
	req.DesiredIPAddresses[1] = testIP1v6

	wrappedHandler := svc.IPConfigsHandlerMiddleware.IPConfigsRequestHandlerWrapper(svc.requestIPConfigHandlerHelper, svc.ReleaseIPConfigHandlerHelper)
	_, err := wrappedHandler(context.TODO(), req)
	if err == nil {
		t.Fatalf("Expected failing requesting IPs due to not able to set routes")
	}
}

func TestIPAMGetStandaloneSWIFTv2(t *testing.T) {
	svc := getTestService(cns.ServiceFabric)
	middleware := middlewares.StandaloneSWIFTv2Middleware{}
	svc.AttachIPConfigsHandlerMiddleware(&middleware)

	orchestratorContext, _ := testPod1Info.OrchestratorContext()
	mockMACAddress := "00:00:00:00:00:00"
	mockGatewayIP := "10.0.0.1" // from mock wireserver gateway calculation on host subnet

	tt := []struct {
		name             string
		req              cns.IPConfigsRequest
		mockNMAgent      *fakes.NMAgentClientFake
		expectedResponse *cns.IPConfigsResponse
	}{
		{
			name: "Successful single IPAM for Standalone SwiftV2 pod, when NMAgent returns error for GetNCVersionList",
			req: cns.IPConfigsRequest{
				DesiredIPAddresses:  []string{testIP1},
				OrchestratorContext: orchestratorContext,
				PodInterfaceID:      testPod1Info.InterfaceID(),
				InfraContainerID:    testPod1Info.InfraContainerID(),
			},
			mockNMAgent: &fakes.NMAgentClientFake{
				GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
					// NMAgent returns an error, eg. NC is not programmed
					return nma.NCVersionList{
						Containers: []nma.NCVersion{},
					}, errors.New("any NMAgent error")
				},
			},
			expectedResponse: &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.Success,
				},
				PodIPInfo: []cns.PodIpInfo{
					{
						PodIPConfig: cns.IPSubnet{
							IPAddress: testIP1,
						},
						NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
							IPSubnet: cns.IPSubnet{
								IPAddress: testIP1,
							},
							GatewayIPAddress: mockGatewayIP,
						},
						MacAddress: mockMACAddress,
						NICType:    cns.DelegatedVMNIC,
						HostPrimaryIPInfo: cns.HostIPInfo{
							Gateway:   mockGatewayIP,
							PrimaryIP: fakes.HostPrimaryIP,
							Subnet:    fakes.HostSubnet,
						},
					},
				},
			},
		},
		{
			name: "Successful single IPAM for Standalone SwiftV2 pod, when NMAgent returns empty response and no error for GetNCVersionList",
			req: cns.IPConfigsRequest{
				DesiredIPAddresses:  []string{testIP1},
				OrchestratorContext: orchestratorContext,
				PodInterfaceID:      testPod1Info.InterfaceID(),
				InfraContainerID:    testPod1Info.InfraContainerID(),
			},
			mockNMAgent: &fakes.NMAgentClientFake{
				GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
					// NMAgent returns an empty response with no error
					return nma.NCVersionList{
						Containers: []nma.NCVersion{},
					}, nil
				},
			},
			expectedResponse: &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.Success,
				},
				PodIPInfo: []cns.PodIpInfo{
					{
						PodIPConfig: cns.IPSubnet{
							IPAddress: testIP1,
						},
						NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
							IPSubnet: cns.IPSubnet{
								IPAddress: testIP1,
							},
							GatewayIPAddress: mockGatewayIP,
						},
						MacAddress: mockMACAddress,
						NICType:    cns.DelegatedVMNIC,
						HostPrimaryIPInfo: cns.HostIPInfo{
							Gateway:   mockGatewayIP,
							PrimaryIP: fakes.HostPrimaryIP,
							Subnet:    fakes.HostSubnet,
						},
					},
				},
			},
		},
		{
			name: "Successful single IPAM for Standalone SwiftV2 pod, when NMAgent returns an NC for GetNCVersionList even if it's not programmed",
			req: cns.IPConfigsRequest{
				DesiredIPAddresses:  []string{testIP1},
				OrchestratorContext: orchestratorContext,
				PodInterfaceID:      testPod1Info.InterfaceID(),
				InfraContainerID:    testPod1Info.InfraContainerID(),
			},
			mockNMAgent: &fakes.NMAgentClientFake{
				GetNCVersionListF: func(_ context.Context) (nma.NCVersionList, error) {
					// NMAgent returns an NC even if it's not programmed
					return nma.NCVersionList{
						Containers: []nma.NCVersion{
							{
								NetworkContainerID: testNCID,
								Version:            "0",
							},
						},
					}, nil
				},
			},
			expectedResponse: &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.Success,
				},
				PodIPInfo: []cns.PodIpInfo{
					{
						PodIPConfig: cns.IPSubnet{
							IPAddress: testIP1,
						},
						NetworkContainerPrimaryIPConfig: cns.IPConfiguration{
							IPSubnet: cns.IPSubnet{
								IPAddress: testIP1,
							},
							GatewayIPAddress: mockGatewayIP,
						},
						MacAddress: mockMACAddress,
						NICType:    cns.DelegatedVMNIC,
						HostPrimaryIPInfo: cns.HostIPInfo{
							Gateway:   mockGatewayIP,
							PrimaryIP: fakes.HostPrimaryIP,
							Subnet:    fakes.HostSubnet,
						},
					},
				},
			},
		},
		{
			name: "Fail validation when orchestrator context can't be unmarshalled",
			req: cns.IPConfigsRequest{
				DesiredIPAddresses:  []string{testIP1},
				OrchestratorContext: json.RawMessage("invalid"),
				PodInterfaceID:      testPod1Info.InterfaceID(),
				InfraContainerID:    testPod1Info.InfraContainerID(),
			},
			expectedResponse: &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.UnsupportedOrchestratorContext,
				},
			},
		},
		{
			name: "Fail validation when orchestrator context is nil",
			req: cns.IPConfigsRequest{
				DesiredIPAddresses:  []string{testIP1},
				OrchestratorContext: nil,
				PodInterfaceID:      testPod1Info.InterfaceID(),
				InfraContainerID:    testPod1Info.InfraContainerID(),
			},
			expectedResponse: &cns.IPConfigsResponse{
				Response: cns.Response{
					ReturnCode: types.EmptyOrchestratorContext,
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			// setup CNS state with SwiftV2 NC
			createAndSaveMockNCRequest(t, svc, testNCID, orchestratorContext, tc.req.DesiredIPAddresses[0], mockGatewayIP, mockMACAddress)

			// IMPORTANT: although SwiftV2 reuses the concept of NCs, NMAgent doesn't program NCs for SwiftV2, but
			// instead programs NICs. When getting SwiftV2 NCs, we want the NIC type and MAC address of the NCs.
			// TODO: we need another way to verify and sync NMAgent's NIC programming status. currently pending a new NMAgent API or NIC programming status to be passed in the SwiftV2 create NC request.
			setupMockNMAgent(t, svc, tc.mockNMAgent)

			// invoke the SwiftV2 IPAM wrapper handler with the standalone SwiftV2 middleware
			wrappedHandler := svc.IPConfigsHandlerMiddleware.IPConfigsRequestHandlerWrapper(svc.requestIPConfigHandlerHelperStandalone, nil)
			resp, err := wrappedHandler(context.TODO(), tc.req)

			if tc.expectedResponse.Response.ReturnCode == types.Success {
				require.NoError(t, err)

				// assert CNS response code
				require.Equal(t, tc.expectedResponse.Response.ReturnCode, resp.Response.ReturnCode)

				expectedPodIPInfo := tc.expectedResponse.PodIPInfo
				actualPodIPInfo := resp.PodIPInfo

				for i, expected := range expectedPodIPInfo {
					// assert SwiftV2 IP is returned
					assert.Len(t, actualPodIPInfo, len(tc.req.DesiredIPAddresses), "Expected list of IPs returned matches the number of desired IPs from CNI IPAM request")
					assert.Equal(t, expected.PodIPConfig.IPAddress, actualPodIPInfo[i].PodIPConfig.IPAddress)
					assert.Equal(t, expected.MacAddress, actualPodIPInfo[i].MacAddress)
					assert.Equal(t, expected.NICType, actualPodIPInfo[i].NICType)

					// assert that PodIPInfo contains interface information
					assert.Equal(t, expected.HostPrimaryIPInfo.Gateway, actualPodIPInfo[i].HostPrimaryIPInfo.Gateway)
					assert.Equal(t, expected.HostPrimaryIPInfo.PrimaryIP, actualPodIPInfo[i].HostPrimaryIPInfo.PrimaryIP)
					assert.Equal(t, expected.HostPrimaryIPInfo.Subnet, actualPodIPInfo[i].HostPrimaryIPInfo.Subnet)
				}
			} else {
				require.Error(t, err)
				assert.Equal(t, tc.expectedResponse.Response.ReturnCode, resp.Response.ReturnCode)
			}
		})
	}
}

func setupMockNMAgent(t *testing.T, svc *HTTPRestService, mockNMAgent *fakes.NMAgentClientFake) {
	t.Helper()
	t.Log("Started mock NMAgent")
	cleanupNMAgentMock := setMockNMAgent(svc, mockNMAgent)
	t.Cleanup(func() {
		cleanupNMAgentMock()
		t.Log("Stopped mock NMAgent")
	})
}

func TestIPAMRequestIPConfigsTwoFamiliesSameNC(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)

	// Add single NC with both IPv4 and IPv6 IPs
	mixedConfigs := map[string]cns.IPConfigurationStatus{
		"ipv4-1": newPodState("10.0.0.1", "ipv4-1", testNCID, types.Available, 0),
		"ipv4-2": newPodState("10.0.0.2", "ipv4-2", testNCID, types.Available, 0),
		"ipv6-1": newPodState("2001:db8::1", "ipv6-1", testNCID, types.Available, 0),
		"ipv6-2": newPodState("2001:db8::2", "ipv6-2", testNCID, types.Available, 0),
	}

	err := updatePodIPConfigState(t, svc, mixedConfigs, testNCID)
	require.NoError(t, err)

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b

	podIPs, err := requestIPConfigsHelper(svc, req)
	require.NoError(t, err)

	// Should assign 2 IPs (one of each family) of same NC
	assert.Len(t, podIPs, 2)

	// Verify we got one of each family
	hasIPv4 := false
	hasIPv6 := false
	for _, podIP := range podIPs {
		ip := net.ParseIP(podIP.PodIPConfig.IPAddress)
		if ip.To4() != nil {
			hasIPv4 = true
		} else {
			hasIPv6 = true
		}
	}
	assert.True(t, hasIPv4, "Should have IPv4 address")
	assert.True(t, hasIPv6, "Should have IPv6 address")
}

func TestIPAMRequestIPConfigsProgrammingPendingFailure(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)

	// Add IPv4 NC with available IP
	ipv4configs := map[string]cns.IPConfigurationStatus{
		"ipv4-1": newPodState("10.0.0.1", "ipv4-1", testNCID, types.Available, 0),
	}

	// Add IPv6 NC with IP initially available
	ipv6configs := map[string]cns.IPConfigurationStatus{
		"ipv6-1": newPodState("2001:db8::1", "ipv6-1", testNCIDv6, types.Available, 0),
	}

	err := updatePodIPConfigState(t, svc, ipv4configs, testNCID)
	require.NoError(t, err)
	err = updatePodIPConfigState(t, svc, ipv6configs, testNCIDv6)
	require.NoError(t, err)

	// Manually set IPv6 IP to PendingProgramming state after NC creation
	ipv6State := svc.PodIPConfigState["ipv6-1"]
	ipv6State.SetState(types.PendingProgramming)
	svc.PodIPConfigState["ipv6-1"] = ipv6State

	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b

	_, err = requestIPConfigsHelper(svc, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enough IPs available")

	// Verify no IPs were assigned
	assignedIPs := svc.GetAssignedIPConfigs()
	assert.Empty(t, assignedIPs)

	// Verify IPv4 IP is still available and ipv6 is not available as the programming failed
	availableIPs := svc.GetAvailableIPConfigs()
	assert.Len(t, availableIPs, 1)
}

func createAndSaveMockNCRequest(t *testing.T, svc *HTTPRestService, ncID string, orchestratorContext json.RawMessage, desiredIP, mockGatewayIP, mockMACAddress string) {
	t.Helper()

	createNCReq := &cns.CreateNetworkContainerRequest{
		NetworkContainerType: "Docker",
		NetworkContainerid:   ncID,
		OrchestratorContext:  orchestratorContext,
		IPConfiguration: cns.IPConfiguration{
			IPSubnet: cns.IPSubnet{
				IPAddress:    desiredIP,
				PrefixLength: ipPrefixBitsv4,
			},
			GatewayIPAddress: mockGatewayIP,
		},
		// SwiftV2 NIC info
		NetworkInterfaceInfo: cns.NetworkInterfaceInfo{
			NICType:    cns.DelegatedVMNIC,
			MACAddress: mockMACAddress,
		},
	}
	err := createNCReq.Validate()
	require.NoError(t, err)

	// save SwiftV2 NC state in CNS
	returnCode, returnMessage := svc.saveNetworkContainerGoalState(*createNCReq)
	require.Equal(t, types.Success, returnCode)
	require.Empty(t, returnMessage)
}

// Validate Statefile in Stateless CNI scenarios
func TestStatelessCNIStateFile(t *testing.T) {
	svc := getTestService(cns.KubernetesCRD)
	svc.EndpointStateStore = store.NewMockStore("")
	// test Case 1 - AKS SIngleTenancy
	endpointInfo1ContainerID := "0a4917617e15d24dc495e407d8eb5c88e4406e58fa209e4eb75a2c2fb7045eea"
	endpointInfo1 := &EndpointInfo{IfnameToIPMap: make(map[string]*IPInfo)}
	endpointInfo1.IfnameToIPMap["eth0"] = &IPInfo{IPv4: []net.IPNet{{IP: net.IPv4(10, 0, 0, 1), Mask: net.IPv4Mask(255, 255, 255, 0)}}}
	req1 := make(map[string]*IPInfo)
	req1["eth0"] = &IPInfo{IPv4: []net.IPNet{{IP: net.IPv4(10, 0, 0, 1), Mask: net.IPv4Mask(255, 255, 255, 0)}}, HnsEndpointID: "5c15cccc-830a-4dff-81f3-4b1e55cb7dcb", NICType: cns.InfraNIC}
	testPod1Info = cns.NewPodInfo(endpointInfo1ContainerID, endpointInfo1ContainerID, "pod1", "default")
	req := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	// test Case 2 - ACI
	endpointInfo2ContainerID := "1b4917617e15d24dc495e407d8eb5c88e4406e58fa209e4eb75a2c2fb7045eea"
	endpointInfo2 := &EndpointInfo{IfnameToIPMap: make(map[string]*IPInfo)}
	endpointInfo2.IfnameToIPMap["eth2"] = &IPInfo{
		IPv4:               nil,
		NICType:            cns.DelegatedVMNIC,
		HnsEndpointID:      "5c15cccc-830a-4dff-81f3-4b1e55cb7dcb",
		HnsNetworkID:       "5c0712cd-824c-4898-b1c0-2fcb16ede4fb",
		MacAddress:         "7c:1e:52:06:d3:4b",
		NetworkContainerID: testNCID,
	}
	// test cases
	tests := []struct {
		name       string
		endpointID string
		req        map[string]*IPInfo
		store      store.KeyValueStore
		want       *EndpointInfo
		wantErr    bool
	}{
		{
			name:       "single-tenancy: update endpoint without error",
			endpointID: endpointInfo1ContainerID,
			req:        req1,
			store:      svc.EndpointStateStore,
			want: &EndpointInfo{
				PodName: "pod1", PodNamespace: "default", IfnameToIPMap: map[string]*IPInfo{
					"eth0": {
						IPv4:          []net.IPNet{{IP: net.IPv4(10, 0, 0, 1), Mask: net.IPv4Mask(255, 255, 255, 0)}},
						HnsEndpointID: "5c15cccc-830a-4dff-81f3-4b1e55cb7dcb",
						NICType:       cns.InfraNIC,
					},
				},
			},
			wantErr: false,
		},
		{
			name:       "ACI: update and create absent endpoint without error",
			endpointID: endpointInfo2ContainerID,
			req:        endpointInfo2.IfnameToIPMap,
			store:      svc.EndpointStateStore,
			want:       endpointInfo2,
			wantErr:    false,
		},
	}
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}

	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state := newPodState(ncStates[i].ips[0], ipIDs[i][0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := updatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail update service with config: %+v", err)
		}
	}
	t.Log(ipconfigs)
	b, _ := testPod1Info.OrchestratorContext()
	req.OrchestratorContext = b
	req.Ifname = "eth0"
	podIPInfo, err := requestIPConfigsHelper(svc, req)
	if err != nil {
		t.Fatalf("Expected to not fail getting pod ip info: %+v", err)
	}

	ipInfo := &IPInfo{}
	for i := range podIPInfo {
		ip, ipnet, errIP := net.ParseCIDR(podIPInfo[i].PodIPConfig.IPAddress + "/" + strconv.FormatUint(uint64(podIPInfo[i].PodIPConfig.PrefixLength), 10))
		if errIP != nil {
			t.Fatalf("failed to parse pod ip address: %+v", errIP)
		}
		ipconfig := net.IPNet{IP: ip, Mask: ipnet.Mask}
		if ip.To4() == nil { // is an ipv6 address
			ipInfo.IPv6 = append(ipInfo.IPv6, ipconfig)
		} else {
			ipInfo.IPv4 = append(ipInfo.IPv4, ipconfig)
		}
	}

	// add goalState
	err = svc.updateEndpointState(req, testPod1Info, podIPInfo)
	if err != nil {
		t.Fatalf("Expected to not fail updating endpoint state: %+v", err)
	}
	// update State
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := svc.UpdateEndpointHelper(tt.endpointID, tt.req)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			got, err := svc.GetEndpointHelper(tt.endpointID)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
