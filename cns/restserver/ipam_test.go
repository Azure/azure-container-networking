// Copyright 2020 Microsoft. All rights reserved.
// MIT License

package restserver

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/fakes"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var (
	testNCID   = "06867cf3-332d-409d-8819-ed70d2c116b0"
	testNCIDv6 = "a69b9217-3d89-4b73-a052-1e8baa453cb0"

	ipPrefixBitsv4 = uint8(24)
	ipPrefixBitsv6 = uint8(120)
	prefixes       = []uint8{ipPrefixBitsv4, ipPrefixBitsv6}

	testIP1      = "10.0.0.1"
	testIP1v6    = "fd12:1234::1"
	testPod1GUID = "898fb8f1-f93e-4c96-9c31-6b89098949a3"
	testPod1Info = cns.NewPodInfo("898fb8-eth0", testPod1GUID, "testpod1", "testpod1namespace")

	testIP2      = "10.0.0.2"
	testIP2v6    = "fd12:1234::2"
	testPod2GUID = "b21e1ee1-fb7e-4e6d-8c68-22ee5049944e"
	testPod2Info = cns.NewPodInfo("b21e1e-eth0", testPod2GUID, "testpod2", "testpod2namespace")

	testIP3      = "10.0.0.3"
	testIP3v6    = "fd12:1234::3"
	testPod3GUID = "718e04ac-5a13-4dce-84b3-040accaa9b41"
	testPod3Info = cns.NewPodInfo("718e04-eth0", testPod3GUID, "testpod3", "testpod3namespace")

	testIP4      = "10.0.0.4"
	testPod4GUID = "718e04ac-5a13-4dce-84b3-040accaa9b42"
)

// Struct that holds information for NCs that will be used in tests
type ncState struct {
	ncID string
	ips  []string
}

func getTestService() *HTTPRestService {
	var config common.ServiceConfig
	httpsvc, _ := NewHTTPRestService(&config, &fakes.WireserverClientFake{}, &fakes.WireserverProxyFake{}, &fakes.NMAgentClientFake{}, store.NewMockStore(""), nil, nil)
	svc = httpsvc.(*HTTPRestService)
	svc.IPAMPoolMonitor = &fakes.MonitorFake{}
	setOrchestratorTypeInternal(cns.KubernetesCRD)

	return svc
}

func newSecondaryIPConfig(ipAddress string, ncVersion int) cns.SecondaryIPConfig {
	return cns.SecondaryIPConfig{
		IPAddress: ipAddress,
		NCVersion: ncVersion,
	}
}

func NewPodState(ipaddress string, id, ncid string, state types.IPState, ncVersion int) cns.IPConfigurationStatus {
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
		assert.Equal(t, primaryIp, podIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.IPAddress)
		assert.Equal(t, subnetPrfixLength, int(podIPInfo[i].NetworkContainerPrimaryIPConfig.IPSubnet.PrefixLength))
		assert.Equal(t, dnsservers, podIPInfo[i].NetworkContainerPrimaryIPConfig.DNSServers)
		assert.Equal(t, gatewayIp, podIPInfo[i].NetworkContainerPrimaryIPConfig.GatewayIPAddress)
		assert.Equal(t, subnetPrfixLength, int(podIPInfo[i].PodIPConfig.PrefixLength))
		assert.Equal(t, fakes.HostPrimaryIP, podIPInfo[i].HostPrimaryIPInfo.PrimaryIP)
		assert.Equal(t, fakes.HostSubnet, podIPInfo[i].HostPrimaryIPInfo.Subnet)
	}

	// retrieve podinfo from orchestrator context
	podInfo, err := cns.UnmarshalPodInfo(req.OrchestratorContext)
	if err != nil {
		return []cns.IPConfigurationStatus{}, errors.Wrap(err, "failed to unmarshal pod info")
	}

	ipConfigStatus := make([]cns.IPConfigurationStatus, 0)
	for _, ipID := range svc.PodIPIDByPodInterfaceKey[podInfo.Key()] {
		ipConfigStatus = append(ipConfigStatus, svc.PodIPConfigState[ipID])
	}
	return ipConfigStatus, nil
}

func NewPodStateWithOrchestratorContext(ipaddress, id, ncid string, state types.IPState, prefixLength uint8, ncVersion int, podInfo cns.PodInfo) (cns.IPConfigurationStatus, error) {
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
func UpdatePodIPConfigState(t *testing.T, svc *HTTPRestService, ipconfigs map[string]cns.IPConfigurationStatus, ncID string) error {
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

// create an endpoint with only one IP
func TestEndpointStateReadAndWriteSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	EndpointStateReadAndWrite(t, ncStates)
}

// create an endpoint with one IP from each NC
func TestEndpointStateReadAndWriteMultipleNCs(t *testing.T) {
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
	EndpointStateReadAndWrite(t, ncStates)
}

// Tests the creation of an endpoint using the NCs and IPs as input and then tests the deletion of that endpoint
func EndpointStateReadAndWrite(t *testing.T, ncStates []ncState) {
	svc := getTestService()
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state := NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
func TestIPAMGetAvailableIPConfigSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMGetAvailableIPConfig(t, ncStates)
}

// assign one IP per NC to the pod
func TestIPAMGetAvailableIPConfigMultipleNCs(t *testing.T) {
	nsStates := []ncState{
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
	IPAMGetAvailableIPConfig(t, nsStates)
}

// Add one IP per NC to the pool and request those IPs
func IPAMGetAvailableIPConfig(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
		desiredState[i] = NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, 0)
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

func TestIPAMGetNextAvailableIPConfigSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
				testIP2,
			},
		},
	}
	IPAMGetNextAvailableIPConfig(t, ncStates)
}

func TestIPAMGetNextAvailableIPConfigMultipleNCs(t *testing.T) {
	ncStates := []ncState{
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
	}
	IPAMGetNextAvailableIPConfig(t, ncStates)
}

// First IP is already assigned to a pod, want second IP
func IPAMGetNextAvailableIPConfig(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	// Add already assigned pod ip to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()] = append(svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()], ncStates[i].ips[0])
		state1, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		state2 := NewPodState(ncStates[i].ips[1], ncStates[i].ips[1], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[1], ncStates[i].ips[1], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod2Info)
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

func TestIPAMGetAlreadyAssignedIPConfigForSamePodSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMGetAlreadyAssignedIPConfigForSamePod(t, ncStates)
}

func TestIPAMGetAlreadyAssignedIPConfigForSamePodMultipleNCs(t *testing.T) {
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
	IPAMGetAlreadyAssignedIPConfigForSamePod(t, ncStates)
}

func IPAMGetAlreadyAssignedIPConfigForSamePod(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	// Add Assigned Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
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

func TestIPAMAttemptToRequestIPNotFoundInPoolSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
				testIP2,
			},
		},
	}
	IPAMAttemptToRequestIPNotFoundInPool(t, ncStates)
}

func TestIPAMAttemptToRequestIPNotFoundInPoolMultipleNCs(t *testing.T) {
	ncStates := []ncState{
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
	}
	IPAMAttemptToRequestIPNotFoundInPool(t, ncStates)
}

func IPAMAttemptToRequestIPNotFoundInPool(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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

func TestIPAMGetDesiredIPConfigWithSpecfiedIPSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMGetDesiredIPConfigWithSpecfiedIP(t, ncStates)
}

func TestIPAMGetDesiredIPConfigWithSpecfiedIPMultipleNCs(t *testing.T) {
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
	IPAMGetDesiredIPConfigWithSpecfiedIP(t, ncStates)
}

func IPAMGetDesiredIPConfigWithSpecfiedIP(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	// Add Available Pod IP to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state := NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
		desiredState[i] = NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, 0)
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

func TestIPAMFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIPSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIP(t, ncStates)
}

func TestIPAMFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIPMultipleNCs(t *testing.T) {
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
	IPAMFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIP(t, ncStates)
}

func IPAMFailToGetDesiredIPConfigWithAlreadyAssignedSpecfiedIP(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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

func TestIPAMFailToGetIPWhenAllIPsAreAssignedSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
				testIP2,
			},
		},
	}
	IPAMFailToGetIPWhenAllIPsAreAssigned(t, ncStates)
}

func TestIPAMFailToGetIPWhenAllIPsAreAssignedMultipleNCs(t *testing.T) {
	ncStates := []ncState{
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
	}
	IPAMFailToGetIPWhenAllIPsAreAssigned(t, ncStates)
}

func IPAMFailToGetIPWhenAllIPsAreAssigned(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	// Add already assigned pod ip to state
	for i := range ncStates {
		state1, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		state2, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod2Info)
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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

func TestIPAMRequestThenReleaseThenRequestAgainSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMRequestThenReleaseThenRequestAgain(t, ncStates)
}

func TestIPAMRequestThenReleaseThenRequestAgainMultipleNCs(t *testing.T) {
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
	IPAMRequestThenReleaseThenRequestAgain(t, ncStates)
}

// 10.0.0.1 = PodInfo1
// Request 10.0.0.1 with PodInfo2 (Fail)
// Release PodInfo1
// Request 10.0.0.1 with PodInfo2 (Success)
func IPAMRequestThenReleaseThenRequestAgain(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	// set state as already assigned
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
	err = svc.releaseIPConfig(testPod1Info)
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
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
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

func TestIPAMReleaseIPIdempotencySingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMReleaseIPIdempotency(t, ncStates)
}

func TestIPAMReleaseIPIdempotencyMultipleNCs(t *testing.T) {
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
	IPAMReleaseIPIdempotency(t, ncStates)
}

func IPAMReleaseIPIdempotency(t *testing.T, ncStates []ncState) {
	svc := getTestService()
	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	// Release Test Pod 1
	err := svc.releaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}

	// Call release again, should be fine
	err = svc.releaseIPConfig(testPod1Info)
	if err != nil {
		t.Fatalf("Unexpected failure releasing IP: %+v", err)
	}
}

func TestIPAMAllocateIPIdempotencySingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMAllocateIPIdempotency(t, ncStates)
}

func TestIPAMAllocateIPIdempotencyMultipleNCs(t *testing.T) {
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
	IPAMAllocateIPIdempotency(t, ncStates)
}

func IPAMAllocateIPIdempotency(t *testing.T, ncStates []ncState) {
	svc := getTestService()
	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}

		err = UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}
}

func TestAvailableIPConfigsSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
				testIP2,
				testIP3,
			},
		},
	}
	AvailableIPConfigs(t, ncStates)
}

func TestAvailableIPConfigsMultipleNCs(t *testing.T) {
	ncStates := []ncState{
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
	}
	AvailableIPConfigs(t, ncStates)
}

func AvailableIPConfigs(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	IDsToBeDeleted := make([]string, len(ncStates))
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	// Add already assigned pod ip to state
	for i := range ncStates {
		state1 := NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Available, 0)
		state2 := NewPodState(ncStates[i].ips[1], ncStates[i].ips[1], ncStates[i].ncID, types.Available, 0)
		state3 := NewPodState(ncStates[i].ips[2], ncStates[i].ips[2], ncStates[i].ncID, types.Available, 0)
		IDsToBeDeleted[i] = state1.ID
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		ipconfigs[state3.ID] = state3
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
		desiredState := NewPodState(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, 0)
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

func TestIPAMMarkIPCountAsPendingSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
			},
		},
	}
	IPAMMarkIPCountAsPending(t, ncStates)
}

func TestIPAMMarkIPCountAsPendingMultipleNCs(t *testing.T) {
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
	IPAMMarkIPCountAsPending(t, ncStates)
}

func IPAMMarkIPCountAsPending(t *testing.T, ncStates []ncState) {
	svc := getTestService()
	// set state as already assigned
	ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
	for i := range ncStates {
		state, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Available, prefixes[i], 0, testPod1Info)
		ipconfigs[state.ID] = state
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
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
		if _, exists := ips[ncStates[i].ips[0]]; !exists {
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
	err = svc.releaseIPConfig(testPod1Info)
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
	svc := getTestService()

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
	svc.IPAMPoolMonitor.Update(
		&v1alpha.NodeNetworkConfig{
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
		},
	)
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
	err = svc.releaseIPConfig(testPod1Info)
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

func TestIPAMMarkExistingIPConfigAsPendingSingleNC(t *testing.T) {
	ncStates := []ncState{
		{
			ncID: testNCID,
			ips: []string{
				testIP1,
				testIP2,
			},
		},
	}
	IPAMMarkExistingIPConfigAsPending(t, ncStates)
}

func TestIPAMMarkExistingIPConfigAsPendingMultipleNCs(t *testing.T) {
	ncStates := []ncState{
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
	}
	IPAMMarkExistingIPConfigAsPending(t, ncStates)
}

func IPAMMarkExistingIPConfigAsPending(t *testing.T, ncStates []ncState) {
	svc := getTestService()

	// Add already assigned pod ip to state
	for i := range ncStates {
		ipconfigs := make(map[string]cns.IPConfigurationStatus, 0)
		svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()] = append(svc.PodIPIDByPodInterfaceKey[testPod1Info.Key()], ncStates[i].ips[0])
		state1, _ := NewPodStateWithOrchestratorContext(ncStates[i].ips[0], ncStates[i].ips[0], ncStates[i].ncID, types.Assigned, prefixes[i], 0, testPod1Info)
		state2 := NewPodState(ncStates[i].ips[1], ncStates[i].ips[1], ncStates[i].ncID, types.Available, 0)
		ipconfigs[state1.ID] = state1
		ipconfigs[state2.ID] = state2
		err := UpdatePodIPConfigState(t, svc, ipconfigs, ncStates[i].ncID)
		if err != nil {
			t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
		}
	}

	pendingIPIDs := make([]string, len(ncStates))
	// mark available ip as as pending
	for i := range ncStates {
		pendingIPIDs[i] = ncStates[i].ips[1]
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
		if pendingIPConfigs[i].ID != ncStates[i].ips[1] {
			t.Fatalf("Expected to see ID %v in pending release ipconfigs, actual %+v", ncStates[i].ips[1], pendingIPConfigs)
		}
	}

	// attempt to mark assigned ipconfig as pending, expect fail
	for i := range ncStates {
		pendingIPIDs[i] = ncStates[i].ips[0]
	}
	err = svc.MarkExistingIPsAsPendingRelease(pendingIPIDs)
	if err == nil {
		t.Fatalf("Expected to fail when marking assigned ip as pending")
	}

	assignedIPConfigs := svc.GetAssignedIPConfigs()
	for i := range ncStates {
		found := false
		for j := range assignedIPConfigs {
			if assignedIPConfigs[j].ID == ncStates[i].ips[0] {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Expected to see ID %v in pending release ipconfigs, actual %+v", ncStates[i].ips[0], assignedIPConfigs)
		}
	}
}

// func TestIPAMFailToReleaseOneIPWhenExpectedToHaveTwo(t *testing.T) {
// 	svc := getTestService()

// 	// set state as already assigned
// 	testState, _ := NewPodStateWithOrchestratorContext(testIP1, testPod1GUID, testNCID, types.Assigned, 24, 0, testPod1Info)
// 	ipconfigs := map[string]cns.IPConfigurationStatus{
// 		testState.ID: testState,
// 	}
// 	emptyIpconfigs := map[string]cns.IPConfigurationStatus{}

// 	err := UpdatePodIPConfigState(t, svc, ipconfigs, testNCID)
// 	if err != nil {
// 		t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
// 	}
// 	err = UpdatePodIPConfigState(t, svc, emptyIpconfigs, testNCIDv6)
// 	if err != nil {
// 		t.Fatalf("Expected to not fail adding empty NC to state: %+v", err)
// 	}

// 	err = svc.releaseIPConfig(testPod1Info)
// 	if err == nil {
// 		t.Fatalf("Expected failure releasing IP")
// 	}

// 	available := svc.GetAvailableIPConfigs()
// 	if len(available) != 0 {
// 		t.Fatal("Expected available ips to be zero since we expect the IP to still be assigned")
// 	}
// }

func TestIPAMFailToRequestOneIPWhenExpectedToHaveTwo(t *testing.T) {
	svc := getTestService()

	// set state as already assigned
	testState := NewPodState(testIP1, testIP1, testNCID, types.Available, 0)
	ipconfigs := map[string]cns.IPConfigurationStatus{
		testState.ID: testState,
	}
	emptyIpconfigs := map[string]cns.IPConfigurationStatus{}

	err := UpdatePodIPConfigState(t, svc, ipconfigs, testNCID)
	if err != nil {
		t.Fatalf("Expected to not fail adding IPs to state: %+v", err)
	}

	err = UpdatePodIPConfigState(t, svc, emptyIpconfigs, testNCIDv6)
	if err != nil {
		t.Fatalf("Expected to not fail adding empty NC to state: %+v", err)
	}

	// request should expect 2 IPs but there is only 1 in the pool
	req := cns.IPConfigsRequest{}
	b, _ := testPod1Info.OrchestratorContext()
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
