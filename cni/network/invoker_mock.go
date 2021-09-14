package network

import (
	"github.com/Azure/azure-container-networking/cni"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
	"net"
)

type MockIpamInvoker struct {
	isIPv6 bool
}

func NewMockIpamInvoker(ipv6 bool) *MockIpamInvoker {
	return &MockIpamInvoker{
		isIPv6: ipv6,
	}
}

func (invoker *MockIpamInvoker) Add(nwCfg *cni.NetworkConfig, _ *cniSkel.CmdArgs, subnetPrefix *net.IPNet, options map[string]interface{}) (*cniTypesCurr.Result, *cniTypesCurr.Result, error) {
	result := &cniTypesCurr.Result{}
	_, ipnet, _ := net.ParseCIDR("10.240.0.5/24")
	gwIp := net.ParseIP("10.240.0.1")
	ipConfig := &cniTypesCurr.IPConfig{Address: *ipnet, Gateway: gwIp}
	result.IPs = append(result.IPs, ipConfig)
	return result, nil, nil
}

func (invoker *MockIpamInvoker) Delete(address *net.IPNet, nwCfg *cni.NetworkConfig, _ *cniSkel.CmdArgs, options map[string]interface{}) error {
	return nil
}
