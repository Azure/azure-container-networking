package network

import (
	"errors"
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/containernetworking/cni/pkg/skel"
	current "github.com/containernetworking/cni/pkg/types/100"
)

const (
	subnetBits   = 24
	ipv4Bits     = 32
	subnetv6Bits = 64
	ipv6Bits     = 128
)

var (
	errV4         = errors.New("v4 fail")
	errV6         = errors.New("v6 Fail")
	errDeleteIpam = errors.New("delete fail")
)

type MockIpamInvoker struct {
	isIPv6 bool
	v4Fail bool
	v6Fail bool
	ipMap  map[string]bool
}

func NewMockIpamInvoker(ipv6, v4Fail, v6Fail bool) *MockIpamInvoker {
	return &MockIpamInvoker{
		isIPv6: ipv6,
		v4Fail: v4Fail,
		v6Fail: v6Fail,
		ipMap:  make(map[string]bool),
	}
}

func (invoker *MockIpamInvoker) Add(opt IPAMAddConfig) (ipamAddResult IPAMAddResult, err error) {
	if invoker.v4Fail {
		return ipamAddResult, errV4
	}

	ipamAddResult.hostSubnetPrefix = net.IPNet{}

	ipv4Str := "10.240.0.5"
	if _, ok := invoker.ipMap["10.240.0.5/24"]; ok {
		ipv4Str = "10.240.0.6"
	}

	ip := net.ParseIP(ipv4Str)
	ipnet := net.IPNet{IP: ip, Mask: net.CIDRMask(subnetBits, ipv4Bits)}
	gwIP := net.ParseIP("10.240.0.1")
	ipConfig := &current.IPConfig{Address: ipnet, Gateway: gwIP}
	ipamAddResult.ipv4Result = &current.Result{}
	ipamAddResult.ipv4Result.IPs = append(ipamAddResult.ipv4Result.IPs, ipConfig)
	invoker.ipMap[ipnet.String()] = true
	if invoker.v6Fail {
		return ipamAddResult, errV6
	}

	if invoker.isIPv6 {
		ipv6Str := "fc00::2"
		if _, ok := invoker.ipMap["fc00::2/128"]; ok {
			ipv6Str = "fc00::3"
		}

		ip := net.ParseIP(ipv6Str)
		ipnet := net.IPNet{IP: ip, Mask: net.CIDRMask(subnetv6Bits, ipv6Bits)}
		gwIP := net.ParseIP("fc00::1")
		ipConfig := &current.IPConfig{Address: ipnet, Gateway: gwIP}
		ipamAddResult.ipv6Result = &current.Result{}
		ipamAddResult.ipv6Result.IPs = append(ipamAddResult.ipv6Result.IPs, ipConfig)
		invoker.ipMap[ipnet.String()] = true
	}

	return ipamAddResult, nil
}

func (invoker *MockIpamInvoker) Delete(addresses []*net.IPNet, nwCfg *cni.NetworkConfig, _ *skel.CmdArgs, options map[string]interface{}) error {
	if invoker.v4Fail || invoker.v6Fail {
		return errDeleteIpam
	}

	if addresses == nil || invoker.ipMap == nil {
		return nil
	}

	for _, address := range addresses {
		if _, ok := invoker.ipMap[address.String()]; !ok {
			return errDeleteIpam
		}
		delete(invoker.ipMap, address.String())
	}
	return nil
}
