package network

import (
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cns"
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
	// Splitting defaultInterfaceInfo from secondaryInterfacesInfo so we don't need to loop for default CNI result every time
	defaultInterfaceInfo    network.InterfaceInfo
	secondaryInterfacesInfo []network.InterfaceInfo
	// ncResponse is used for Swift 1.0 multitenancy
	ncResponse       *cns.GetNetworkContainerResponse
	hostSubnetPrefix net.IPNet
	ipv6Enabled      bool
}
