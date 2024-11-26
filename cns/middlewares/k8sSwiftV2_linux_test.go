package middlewares

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"gotest.tools/v3/assert"
)

var (
	testPod1GUID = "898fb8f1-f93e-4c96-9c31-6b89098949a3"
	testPod1Info = cns.NewPodInfo("898fb8-eth0", testPod1GUID, "testpod1", "testpod1namespace")

	testPod2GUID = "b21e1ee1-fb7e-4e6d-8c68-22ee5049944e"
	testPod2Info = cns.NewPodInfo("b21e1e-eth0", testPod2GUID, "testpod2", "testpod2namespace")

	testPod3GUID = "718e04ac-5a13-4dce-84b3-040accaa9b41"
	testPod3Info = cns.NewPodInfo("718e04-eth0", testPod3GUID, "testpod3", "testpod3namespace")

	testPod4GUID = "b21e1ee1-fb7e-4e6d-8c68-22ee5049944e"
	testPod4Info = cns.NewPodInfo("b21e1e-eth0", testPod4GUID, "testpod4", "testpod4namespace")

	testPod6GUID = "898fb8f1-f93e-4c96-9c31-6b89098949a3"
	testPod6Info = cns.NewPodInfo("898fb8-eth0", testPod6GUID, "testpod6", "testpod6namespace")

	testPod7GUID = "123e4567-e89b-12d3-a456-426614174000"
	testPod7Info = cns.NewPodInfo("123e45-eth0", testPod7GUID, "testpod7", "testpod7namespace")

	testPod8GUID = "2006cad4-e54d-472e-863d-c4bac66200a7"
	testPod8Info = cns.NewPodInfo("2006cad4-eth0", testPod8GUID, "testpod8", "testpod8namespace")

	testPod9GUID = "2006cad4-e54d-472e-863d-c4bac66200a7"
	testPod9Info = cns.NewPodInfo("2006cad4-eth0", testPod9GUID, "testpod9", "testpod9namespace")
)

func TestMain(m *testing.M) {
	logger.InitLogger("testlogs", 0, 0, "./")
	m.Run()
}

func TestIPConfigsRequestHandlerWrapperSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24,16A0:0010:AB00:001E::2/32")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16,16A0:0010:AB00:0000::/32")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.1/16,16A0:0020:AB00:0000::/32")
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
				{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    "2001:0db8:abcd:0015::0",
						PrefixLength: 64,
					},
					NICType: cns.InfraNIC,
				},
			},
		}, nil
	}
	failureHandler := func(context.Context, cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		return nil, nil
	}
	wrappedHandler := middleware.IPConfigsRequestHandlerWrapper(defaultHandler, failureHandler)
	happyReq := cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	happyReq.OrchestratorContext = b
	resp, err := wrappedHandler(context.TODO(), happyReq)
	assert.Equal(t, err, nil)
	assert.Equal(t, resp.PodIPInfo[2].PodIPConfig.IPAddress, "192.168.0.1")
	assert.Equal(t, resp.PodIPInfo[2].MacAddress, "00:00:00:00:00:00")
}

func TestIPConfigsRequestHandlerWrapperFailure(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
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
				{
					PodIPConfig: cns.IPSubnet{
						IPAddress:    "2001:0db8:abcd:0015::0",
						PrefixLength: 64,
					},
					NICType: cns.InfraNIC,
				},
			},
		}, nil
	}
	failureHandler := func(context.Context, cns.IPConfigsRequest) (*cns.IPConfigsResponse, error) {
		return nil, nil
	}
	wrappedHandler := middleware.IPConfigsRequestHandlerWrapper(defaultHandler, failureHandler)
	// MTPNC not ready test
	failReq := cns.IPConfigsRequest{
		PodInterfaceID:   testPod4Info.InterfaceID(),
		InfraContainerID: testPod4Info.InfraContainerID(),
	}
	b, _ := testPod4Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	resp, _ := wrappedHandler(context.TODO(), failReq)
	assert.Equal(t, resp.Response.Message, errMTPNCNotReady.Error())

	// Failed to set routes
	failReq = cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ = testPod1Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	_, err := wrappedHandler(context.TODO(), failReq)
	assert.ErrorContains(t, err, "failed to set routes for pod")
}

func TestValidateMultitenantIPConfigsRequestSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	happyReq := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	happyReq.OrchestratorContext = b
	happyReq.SecondaryInterfacesExist = false

	_, respCode, err := middleware.validateIPConfigsRequest(context.TODO(), happyReq)
	assert.Equal(t, err, "")
	assert.Equal(t, respCode, types.Success)
	assert.Equal(t, happyReq.SecondaryInterfacesExist, true)

	happyReq2 := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod8Info.InterfaceID(),
		InfraContainerID: testPod8Info.InfraContainerID(),
	}

	b, _ = testPod8Info.OrchestratorContext()
	happyReq2.OrchestratorContext = b
	happyReq2.SecondaryInterfacesExist = false

	_, respCode, err = middleware.validateIPConfigsRequest(context.TODO(), happyReq2)
	assert.Equal(t, err, "")
	assert.Equal(t, respCode, types.Success)
	assert.Equal(t, happyReq.SecondaryInterfacesExist, true)

	happyReq3 := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod9Info.InterfaceID(),
		InfraContainerID: testPod9Info.InfraContainerID(),
	}

	b, _ = testPod9Info.OrchestratorContext()
	happyReq3.OrchestratorContext = b
	happyReq3.SecondaryInterfacesExist = false

	_, respCode, err = middleware.validateIPConfigsRequest(context.TODO(), happyReq3)
	assert.Equal(t, err, "")
	assert.Equal(t, respCode, types.Success)
	assert.Equal(t, happyReq3.SecondaryInterfacesExist, false)
	assert.Equal(t, happyReq3.BackendInterfaceExist, true)
}

func TestValidateMultitenantIPConfigsRequestFailure(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	// Fail to unmarshal pod info test
	failReq := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	failReq.OrchestratorContext = []byte("invalid")
	_, respCode, _ := middleware.validateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)

	// Pod doesn't exist in cache test
	failReq = &cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ := testPod2Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	_, respCode, _ = middleware.validateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)

	// Failed to get MTPNC
	b, _ = testPod3Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	_, respCode, _ = middleware.validateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)

	// MTPNC not ready
	b, _ = testPod4Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	_, respCode, _ = middleware.validateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)
}

func TestGetSWIFTv2IPConfigSuccess(t *testing.T) {
	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24,16A0:0010:AB00:001E::2/32")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16,16A0:0010:AB00:0000::/32")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.1/16,16A0:0020:AB00:0000::/32")

	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	ipInfos, err := middleware.getIPConfig(context.TODO(), testPod1Info)
	assert.Equal(t, err, nil)
	// Ensure that the length of ipInfos matches the number of InterfaceInfos
	// Adjust this according to the test setup
	assert.Equal(t, len(ipInfos), 1)
	assert.Equal(t, ipInfos[0].NICType, cns.DelegatedVMNIC)
	assert.Equal(t, ipInfos[0].SkipDefaultRoutes, false)
}

func TestGetSWIFTv2IPConfigFailure(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	// Pod's MTPNC doesn't exist in cache test
	_, err := middleware.getIPConfig(context.TODO(), testPod3Info)
	assert.ErrorContains(t, err, mock.ErrMTPNCNotFound.Error())

	// Pod's MTPNC is not ready test
	_, err = middleware.getIPConfig(context.TODO(), testPod4Info)
	assert.Error(t, err, errMTPNCNotReady.Error())
}

func TestSetRoutesSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24,16A0:0010:AB00:001E::2/32")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16,16A0:0010:AB00:0000::/32")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.1/16,16A0:0020:AB00:0000::/32")

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
				IPAddress:    "2001:0db8:abcd:0015::0",
				PrefixLength: 64,
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
				IPAddress:    "10.0.1.10",
				PrefixLength: 32,
			},
			NICType: cns.InfraNIC,
			Routes: []cns.Route{
				{
					IPAddress:        "10.0.1.10/24",
					GatewayIPAddress: overlayGatewayv4,
				},
				{
					IPAddress:        "10.0.0.0/16",
					GatewayIPAddress: overlayGatewayv4,
				},
				{
					IPAddress:        "10.240.0.1/16",
					GatewayIPAddress: overlayGatewayv4,
				},
			},
		},
		{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    "2001:0db8:abcd:0015::0",
				PrefixLength: 64,
			},
			NICType: cns.InfraNIC,
			Routes: []cns.Route{
				{
					IPAddress:        "16A0:0010:AB00:001E::2/32",
					GatewayIPAddress: overlayGatewayV6,
				},
				{
					IPAddress:        "16A0:0010:AB00:0000::/32",
					GatewayIPAddress: overlayGatewayV6,
				},
				{
					IPAddress:        "16A0:0020:AB00:0000::/32",
					GatewayIPAddress: overlayGatewayV6,
				},
			},
		},
		{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    "20.240.1.242",
				PrefixLength: 32,
			},
			NICType:    cns.DelegatedVMNIC,
			MacAddress: "12:34:56:78:9a:bc",
			Routes: []cns.Route{
				{
					IPAddress: fmt.Sprintf("%s/%d", virtualGW, prefixLength),
				},
				{
					IPAddress:        "0.0.0.0/0",
					GatewayIPAddress: virtualGW,
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
	for i := range podIPInfo {
		assert.DeepEqual(t, podIPInfo[i].Routes, desiredPodIPInfo[i].Routes)
	}
}

func TestSetRoutesFailure(t *testing.T) {
	// Failure due to env var not set
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
				IPAddress:    "2001:0db8:abcd:0015::0",
				PrefixLength: 64,
			},
			NICType: cns.InfraNIC,
		},
	}
	for i := range podIPInfo {
		ipInfo := &podIPInfo[i]
		err := middleware.setRoutes(ipInfo)
		if err == nil {
			t.Errorf("SetRoutes should fail due to env var not set")
		}
	}
}

func TestAddRoutes(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	cidrs := []string{"10.0.0.0/24", "20.0.0.0/24"}
	gatewayIP := "192.168.1.1"
	routes := middleware.addRoutes(cidrs, gatewayIP)
	expectedRoutes := []cns.Route{
		{
			IPAddress:        "10.0.0.0/24",
			GatewayIPAddress: gatewayIP,
		},
		{
			IPAddress:        "20.0.0.0/24",
			GatewayIPAddress: gatewayIP,
		},
	}
	if len(routes) != len(expectedRoutes) {
		t.Fatalf("expected %d routes, got %d", len(expectedRoutes), len(routes))
	}
	for i := range routes {
		if routes[i] != expectedRoutes[i] {
			t.Errorf("route %d: expected %+v, got %+v", i, expectedRoutes[i], routes[i])
		}
	}
}

func TestNICTypeConfigSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	// Test Backend NIC type
	ipInfos, err := middleware.getIPConfig(context.TODO(), testPod6Info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ipInfos) != 0 {
		t.Fatalf("expected 0 ipInfo, got %d", len(ipInfos))
	}
}

func TestGetSWIFTv2IPConfigMultiInterfaceFailure(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	// Pod's MTPNC doesn't exist in cache test
	_, err := middleware.getIPConfig(context.TODO(), testPod3Info)
	assert.ErrorContains(t, err, mock.ErrMTPNCNotFound.Error())

	// Pod's MTPNC is not ready test
	_, err = middleware.getIPConfig(context.TODO(), testPod4Info)
	assert.Error(t, err, errMTPNCNotReady.Error())
}

func TestGetSWIFTv2IPConfigMultiInterfaceSuccess(t *testing.T) {
	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24,16A0:0010:AB00:001E::2/32")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16,16A0:0010:AB00:0000::/32")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.1/16,16A0:0020:AB00:0000::/32")

	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	ipInfos, err := middleware.getIPConfig(context.TODO(), testPod7Info)
	assert.Equal(t, err, nil)
	// Ensure that the length of ipInfos matches the number of InterfaceInfos
	// Adjust this according to the test setup in mock client
	expectedInterfaceCount := 2
	assert.Equal(t, len(ipInfos), expectedInterfaceCount)

	for _, ipInfo := range ipInfos {
		switch ipInfo.NICType {
		case cns.DelegatedVMNIC:
			assert.Equal(t, ipInfo.NICType, cns.DelegatedVMNIC)
		case cns.NodeNetworkInterfaceBackendNIC:
			assert.Equal(t, ipInfo.NICType, cns.NodeNetworkInterfaceBackendNIC)
		case cns.InfraNIC:
			assert.Equal(t, ipInfo.NICType, cns.InfraNIC)
		default:
			t.Errorf("unexpected NICType: %v", ipInfo.NICType)
		}
		assert.Equal(t, ipInfo.SkipDefaultRoutes, false)
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
	// assert that the function for linux does not modify any fields
	assert.Equal(t, ipInfo.PodIPConfig.PrefixLength, uint8(32))
	assert.Equal(t, ipInfo.HostPrimaryIPInfo.Gateway, "")
	assert.Equal(t, ipInfo.HostPrimaryIPInfo.Subnet, "")
}
