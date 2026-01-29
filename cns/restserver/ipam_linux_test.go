package restserver

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/middlewares"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/stretchr/testify/assert"
)

func TestIPAMGetK8sInfinibandSuccess(t *testing.T) {
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
		PodInterfaceID:   testPod8Info.InterfaceID(),
		InfraContainerID: testPod8Info.InfraContainerID(),
	}
	b, _ := testPod8Info.OrchestratorContext()
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

	if len(podIPInfo) != 4 {
		t.Fatalf("Expected to get 4 pod IP info (IPv4, IPv6, Multitenant IP, Backend Nic), actual %d", len(podIPInfo))
	}

	// Asserting that SWIFT v2 IP is returned
	assert.Equal(t, SWIFTv2IP, podIPInfo[3].PodIPConfig.IPAddress)
	assert.Equal(t, SWIFTv2MAC, podIPInfo[3].MacAddress)
	assert.Equal(t, cns.DelegatedVMNIC, podIPInfo[3].NICType)
	assert.False(t, podIPInfo[3].SkipDefaultRoutes)
}
