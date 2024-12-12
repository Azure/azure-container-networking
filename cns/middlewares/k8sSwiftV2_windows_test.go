package middlewares

import (
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/middlewares/mock"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"gotest.tools/v3/assert"
)

func TestSetRoutesSuccess(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}
	t.Setenv(configuration.EnvPodCIDRs, "10.0.1.10/24")
	t.Setenv(configuration.EnvServiceCIDRs, "10.0.0.0/16")
	t.Setenv(configuration.EnvInfraVNETCIDRs, "10.240.0.10/16")

	podIPInfo := []cns.PodIpInfo{
		{
			PodIPConfig: cns.IPSubnet{
				IPAddress:    "10.0.1.100",
				PrefixLength: 32,
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
				IPAddress:    "10.0.1.100",
				PrefixLength: 32,
			},
			NICType: cns.InfraNIC,
			Routes: []cns.Route{
				{
					IPAddress:        "10.0.1.10/24",
					GatewayIPAddress: "10.0.1.1",
				},
				{
					IPAddress:        "10.0.0.0/16",
					GatewayIPAddress: "10.0.0.1",
				},
				{
					IPAddress:        "10.240.0.10/16",
					GatewayIPAddress: "10.240.0.1",
				},
				{
					IPAddress:        "0.0.0.0/0",
					GatewayIPAddress: "0.0.0.0",
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

	// check if the routes are set as expected
	reflect.DeepEqual(podIPInfo[0].Routes, desiredPodIPInfo[0].Routes)
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
	// assert that the function for windows modifies all the expected fields with prefix-length
	assert.Equal(t, ipInfo.PodIPConfig.PrefixLength, uint8(16))
	assert.Equal(t, ipInfo.HostPrimaryIPInfo.Gateway, intInfo.GatewayIP)
	assert.Equal(t, ipInfo.HostPrimaryIPInfo.Subnet, intInfo.SubnetAddressSpace)
}

func TestAddDefaultRoute(t *testing.T) {
	middleware := K8sSWIFTv2Middleware{Cli: mock.NewClient()}

	podIPInfo := cns.PodIpInfo{
		PodIPConfig: cns.IPSubnet{
			IPAddress:    "20.240.1.242",
			PrefixLength: 32,
		},
		NICType:    cns.DelegatedVMNIC,
		MacAddress: "12:34:56:78:9a:bc",
	}

	gatewayIP := "20.240.1.1"
	intInfo := v1alpha1.InterfaceInfo{
		GatewayIP:          gatewayIP,
		SubnetAddressSpace: "20.240.1.0/16",
	}

	ipInfo := podIPInfo
	middleware.addDefaultRoute(&ipInfo, intInfo.GatewayIP)

	expectedRoutes := []cns.Route{
		{
			IPAddress:        "0.0.0.0/0",
			GatewayIPAddress: gatewayIP,
		},
	}

	if !reflect.DeepEqual(ipInfo.Routes, expectedRoutes) {
		t.Errorf("got '%+v', expected '%+v'", ipInfo.Routes, expectedRoutes)
	}
}
