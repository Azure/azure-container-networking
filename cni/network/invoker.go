package network

import (
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
)

// IPAMInvoker is used by the azure-vnet CNI plugin to call different sources for IPAM.
// This interface can be used to call into external binaries, like the azure-vnet-ipam binary,
// or simply act as a client to an external ipam, such as azure-cns.
type IPAMInvoker interface {
	// Add returns two results, one IPv4, the other IPv6.
	Add(IPAMAddConfig) (IPAMAddResult, error)

	// Delete calls to the invoker source, and returns error. Returning an error here will fail the CNI Delete call.
	Delete(addresses []*net.IPNet, nwCfg *cni.NetworkConfig, args *cniSkel.CmdArgs, options map[string]interface{}) error
}

type IPAMAddConfig struct {
	nwCfg   *cni.NetworkConfig
	args    *cniSkel.CmdArgs
	options map[string]interface{}
}

type IPAMAddResult struct {
	ipv4Result       *cniTypesCurr.Result
	ipv6Result       *cniTypesCurr.Result
	ncResponse       *cns.GetNetworkContainerResponse
	hostSubnetPrefix net.IPNet
}
