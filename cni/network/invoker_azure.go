package network

import (
	"fmt"
	"net"
	"runtime/debug"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

type AzureIPAMInvoker struct {
	plugin *netPlugin
	nwInfo *network.NetworkInfo
}

func NewAzureIpamInvoker(plugin *netPlugin, nwInfo *network.NetworkInfo) *AzureIPAMInvoker {
	return &AzureIPAMInvoker{
		plugin: plugin,
		nwInfo: nwInfo,
	}
}

func (invoker *AzureIPAMInvoker) Add(nwCfg *cni.NetworkConfig, _ *cniSkel.CmdArgs, subnetPrefix *net.IPNet, options map[string]interface{}) (*cniTypesCurr.Result, *cniTypesCurr.Result, error) {
	var (
		result   *cniTypesCurr.Result
		resultV6 *cniTypesCurr.Result
		err      error
	)

	if nwCfg == nil {
		err = invoker.plugin.Errorf("nil nwCfg passed to CNI ADD, stack: %+v", string(debug.Stack()))
		return nil, nil, err
	}

	if len(invoker.nwInfo.Subnets) > 0 {
		nwCfg.Ipam.Subnet = invoker.nwInfo.Subnets[0].Prefix.String()
	}

	// Call into IPAM plugin to allocate an address pool for the network.
	result, err = invoker.plugin.DelegateAdd(nwCfg.Ipam.Type, nwCfg)
	if err != nil {
		err = invoker.plugin.Errorf("Failed to allocate pool: %v", err)
		return nil, nil, err	
	}

	defer func() {
		if err != nil {
			if len(result.IPs) > 0 {
				if er := invoker.plugin.ipamInvoker.Delete(&result.IPs[0].Address, nwCfg, nil, options); er != nil {
					err = invoker.plugin.Errorf("Failed to clean up IP's during Delete with error %v, after Add failed with error %w", er, err)
				}
			} else {
				err = fmt.Errorf("No IP's to delete on error: %v", err)
			}
		}
	}()

	if nwCfg.IPV6Mode != "" {
		nwCfg6 := *nwCfg
		nwCfg6.Ipam.Environment = common.OptEnvironmentIPv6NodeIpam
		nwCfg6.Ipam.Type = ipamV6

		if len(invoker.nwInfo.Subnets) > 1 {
			// ipv6 is the second subnet of the slice
			nwCfg6.Ipam.Subnet = invoker.nwInfo.Subnets[1].Prefix.String()
		}

		resultV6, err = invoker.plugin.DelegateAdd(nwCfg6.Ipam.Type, &nwCfg6)
		if err != nil {
			err = invoker.plugin.Errorf("Failed to allocate v6 pool: %v", err)
		}
	}

	sub := &result.IPs[0].Address
	*subnetPrefix = *sub

	return result, resultV6, err
}

func (invoker *AzureIPAMInvoker) Delete(address *net.IPNet, nwCfg *cni.NetworkConfig, _ *cniSkel.CmdArgs, options map[string]interface{}) error {

	if nwCfg == nil {
		return invoker.plugin.Errorf("nil nwCfg passed to CNI ADD, stack: %+v", string(debug.Stack()))
	}

	if len(invoker.nwInfo.Subnets) > 0 {
		nwCfg.Ipam.Subnet = invoker.nwInfo.Subnets[0].Prefix.String()
	}

	if address == nil {
		if err := invoker.plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg); err != nil {
			return invoker.plugin.Errorf("Attempted to release address with error:  %v", err)
		}
	} else if len(address.IP.To4()) == 4 {
		nwCfg.Ipam.Address = address.IP.String()
		log.Printf("Releasing ipv4 address :%s pool: %s",
			nwCfg.Ipam.Address, nwCfg.Ipam.Subnet)
		if err := invoker.plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg); err != nil {
			log.Printf("Failed to release ipv4 address: %v", err)
			return invoker.plugin.Errorf("Failed to release ipv4 address: %v", err)
		}
	} else if len(address.IP.To16()) == 16 {
		nwCfgIpv6 := *nwCfg
		nwCfgIpv6.Ipam.Environment = common.OptEnvironmentIPv6NodeIpam
		nwCfgIpv6.Ipam.Type = ipamV6
		nwCfgIpv6.Ipam.Address = address.IP.String()
		if len(invoker.nwInfo.Subnets) > 1 {
			nwCfgIpv6.Ipam.Subnet = invoker.nwInfo.Subnets[1].Prefix.String()
		}

		log.Printf("Releasing ipv6 address :%s pool: %s",
			nwCfgIpv6.Ipam.Address, nwCfgIpv6.Ipam.Subnet)
		if err := invoker.plugin.DelegateDel(nwCfgIpv6.Ipam.Type, &nwCfgIpv6); err != nil {
			log.Printf("Failed to release ipv6 address: %v", err)
			return invoker.plugin.Errorf("Failed to release ipv6 address: %v", err)
		}
	} else {
		return invoker.plugin.Errorf("Address is incorrect, not valid IPv4 or IPv6, stack: %+v", string(debug.Stack()))
	}

	return nil
}
