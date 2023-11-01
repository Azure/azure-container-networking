package middlewares

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/cns/types"
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
)

func setEnvVar() {
	os.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24,16A0:0010:AB00:001E::2/32")
	os.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16,16A0:0010:AB00:0000::/32")
	os.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.1/16,16A0:0020:AB00:0000::/32")
}

func unsetEnvVar() {
	os.Unsetenv(configuration.EnvPodCIDRs)
	os.Unsetenv(configuration.EnvServiceCIDRs)
	os.Unsetenv(configuration.EnvInfraVNETCIDRs)
}

func TestMain(m *testing.M) {
	logger.InitLogger("testlogs", 0, 0, "./")
	m.Run()
}

func TestValidateMultitenantIPConfigsRequestSuccess(t *testing.T) {
	middleware := SWIFTv2Middleware{Cli: mock.NewClient()}

	happyReq := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	b, _ := testPod1Info.OrchestratorContext()
	happyReq.OrchestratorContext = b
	happyReq.SecondaryInterfacesExist = false

	respCode, err := middleware.ValidateIPConfigsRequest(context.TODO(), happyReq)
	assert.Equal(t, err, "")
	assert.Equal(t, respCode, types.Success)
	assert.Equal(t, happyReq.SecondaryInterfacesExist, true)
}

func TestValidateMultitenantIPConfigsRequestFailure(t *testing.T) {
	middleware := SWIFTv2Middleware{Cli: mock.NewClient()}

	// Fail to unmarshal pod info test
	failReq := &cns.IPConfigsRequest{
		PodInterfaceID:   testPod1Info.InterfaceID(),
		InfraContainerID: testPod1Info.InfraContainerID(),
	}
	failReq.OrchestratorContext = []byte("invalid")
	respCode, _ := middleware.ValidateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)

	// Pod doesn't exist in cache test
	failReq = &cns.IPConfigsRequest{
		PodInterfaceID:   testPod2Info.InterfaceID(),
		InfraContainerID: testPod2Info.InfraContainerID(),
	}
	b, _ := testPod2Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	respCode, _ = middleware.ValidateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)

	// Failed to get MTPNC
	b, _ = testPod3Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	respCode, _ = middleware.ValidateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)

	// MTPNC not ready
	b, _ = testPod4Info.OrchestratorContext()
	failReq.OrchestratorContext = b
	respCode, _ = middleware.ValidateIPConfigsRequest(context.TODO(), failReq)
	assert.Equal(t, respCode, types.UnexpectedError)
}

func TestGetSWIFTv2IPConfigSuccess(t *testing.T) {
	setEnvVar()
	defer unsetEnvVar()

	middleware := SWIFTv2Middleware{Cli: mock.NewClient()}

	ipInfo, err := middleware.GetIPConfig(context.TODO(), testPod1Info)
	assert.Equal(t, err, nil)
	assert.Equal(t, ipInfo.NICType, cns.DelegatedVMNIC)
	assert.Equal(t, ipInfo.SkipDefaultRoutes, false)
}

func TestGetSWIFTv2IPConfigFailure(t *testing.T) {
	middleware := SWIFTv2Middleware{Cli: mock.NewClient()}

	// Pod's MTPNC doesn't exist in cache test
	_, err := middleware.GetIPConfig(context.TODO(), testPod2Info)
	assert.ErrorContains(t, err, mock.ErrMTPNCNotFound.Error())

	// Pod's MTPNC is not ready test
	_, err = middleware.GetIPConfig(context.TODO(), testPod4Info)
	assert.Error(t, err, errMTPNCNotReady.Error())
}

func TestSetRoutesSuccess(t *testing.T) {
	middleware := SWIFTv2Middleware{Cli: mock.NewClient()}
	setEnvVar()
	defer unsetEnvVar()
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
		err := middleware.SetRoutes(ipInfo)
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
	middleware := SWIFTv2Middleware{Cli: mock.NewClient()}
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
		err := middleware.SetRoutes(ipInfo)
		if err == nil {
			t.Errorf("SetRoutes should fail due to env var not set")
		}
	}
}
