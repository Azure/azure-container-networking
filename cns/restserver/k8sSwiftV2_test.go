package restserver

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/cns/types"
	"gotest.tools/v3/assert"
)

var (
	testPod4Info = cns.NewPodInfo("b21e1e-eth0", testPod4GUID, "testpod4", "testpod4namespace")
)

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

	ipInfo, err := middleware.getIPConfig(context.TODO(), testPod1Info)
	assert.Equal(t, err, nil)
	assert.Equal(t, ipInfo.NICType, cns.DelegatedVMNIC)
	assert.Equal(t, ipInfo.SkipDefaultRoutes, false)
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
