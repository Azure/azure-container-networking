package network

import (
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
)

// IPAMInvoker is used by the azure-vnet CNI plugin to call different sources for IPAM.
// This interface can be used to call into external binaries, like the azure-vnet-ipam binary,
// or simply act as a client to an external ipam, such as azure-cns.
type IPAMInvoker interface {
	// Add returns two results, one IPv4, the other IPv6.
	Add(IPAMAddConfig) (IPAMAddResult, error)

	// Delete calls to the invoker source, and returns error. Returning an error here will fail the CNI Delete call.
	Delete(address *net.IPNet, nwCfg *cni.NetworkConfig, args *cniSkel.CmdArgs, options map[string]interface{}) error
}

type IPAMAddConfig struct {
	nwCfg   *cni.NetworkConfig
	args    *cniSkel.CmdArgs
	options map[string]interface{}
}

type IPAMAddResult struct {
	interfaceInfo map[string]network.InterfaceInfo
	// ncResponse and host subnet prefix were moved into interface info
	ipv6Enabled bool
}

func (ipamAddResult IPAMAddResult) PrettyString() string {
	pStr := "InterfaceInfo: "
	for key := range ipamAddResult.interfaceInfo {
		val := ipamAddResult.interfaceInfo[key]
		pStr += val.PrettyString()
	}
	return pStr
}

// shallow copy options from one map to a new options map
func (ipamAddConfig IPAMAddConfig) shallowCopyIpamAddConfigOptions() map[string]interface{} {
	res := map[string]interface{}{}
	for k, v := range ipamAddConfig.options {
		// only support primitive type
		res[k] = v
	}
	return res
}
