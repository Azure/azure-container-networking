package network

import (
	"errors"
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
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
	errV4             = errors.New("v4 fail")
	errV6             = errors.New("v6 Fail")
	errDelegatedVMNIC = errors.New("delegatedVMNIC fail")
	errDeleteIpam     = errors.New("delete fail")
)

type MockIpamInvoker struct {
	isIPv6             bool
	v4Fail             bool
	v6Fail             bool
	delegatedVMNIC     bool
	delegatedVMNICFail bool
	ipMap              map[string]bool
}

func NewMockIpamInvoker(ipv6, v4Fail, v6Fail, delegatedVMNIC, delegatedVMNICFail bool) *MockIpamInvoker {
	return &MockIpamInvoker{
		isIPv6:             ipv6,
		v4Fail:             v4Fail,
		v6Fail:             v6Fail,
		delegatedVMNIC:     delegatedVMNIC,
		delegatedVMNICFail: delegatedVMNICFail,

		ipMap: make(map[string]bool),
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
	ipamAddResult.defaultInterfaceInfo = InterfaceInfo{
		ipResult: &current.Result{
			IPs: []*current.IPConfig{{Address: ipnet, Gateway: gwIP}},
		},
		nicType: cns.InfraNIC,
	}
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
		ipamAddResult.defaultInterfaceInfo.ipResult.IPs = append(ipamAddResult.defaultInterfaceInfo.ipResult.IPs, &current.IPConfig{Address: ipnet, Gateway: gwIP})
		invoker.ipMap[ipnet.String()] = true
	}

	if invoker.delegatedVMNIC {
		if invoker.delegatedVMNICFail {
			return IPAMAddResult{}, errDelegatedVMNIC
		}

		ipStr := "20.20.20.20/32"
		_, ipnet, _ := net.ParseCIDR(ipStr)
		ipamAddResult.secondaryInterfacesInfo = append(ipamAddResult.secondaryInterfacesInfo, InterfaceInfo{
			ipResult: &current.Result{
				IPs: []*current.IPConfig{{Address: *ipnet}},
			},
			nicType: cns.DelegatedVMNIC,
		})
	}

	return ipamAddResult, nil
}

func (invoker *MockIpamInvoker) Delete(address *net.IPNet, nwCfg *cni.NetworkConfig, _ *skel.CmdArgs, options map[string]interface{}) error {
	if invoker.v4Fail || invoker.v6Fail {
		return errDeleteIpam
	}

	if address == nil || invoker.ipMap == nil {
		return nil
	}

	if _, ok := invoker.ipMap[address.String()]; !ok {
		return errDeleteIpam
	}
	delete(invoker.ipMap, address.String())
	return nil
}
