package network

import (
	"fmt"
	"net"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
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

func (invoker *AzureIPAMInvoker) Add(nwCfg *cni.NetworkConfig, subnetPrefix *net.IPNet, options map[string]interface{}) (*cniTypesCurr.Result, *cniTypesCurr.Result, error) {
	var (
		result   *cniTypesCurr.Result
		resultV6 *cniTypesCurr.Result
		err      error
	)

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
				invoker.plugin.ipamInvoker.Delete(&result.IPs[0].Address, nwCfg, options)
			} else {
				err = fmt.Errorf("No IP's to delete on error: %v", err)
			}
		}
	}()

	if nwCfg.IPV6Mode != "" {
		nwCfg6 := nwCfg
		nwCfg6.Ipam.Environment = common.OptEnvironmentIPv6NodeIpam
		nwCfg6.Ipam.Type = ipamV6

		if len(invoker.nwInfo.Subnets) > 1 {
			// ipv6 is the second subnet of the slice
			nwCfg6.Ipam.Subnet = invoker.nwInfo.Subnets[1].Prefix.String()
		}

		resultV6, err = invoker.plugin.DelegateAdd(ipamV6, nwCfg6)
		if err != nil {
			err = invoker.plugin.Errorf("Failed to allocate v6 pool: %v", err)
		}
	}

	sub := &result.IPs[0].Address
	*subnetPrefix = *sub

	return result, resultV6, err
}

func (invoker *AzureIPAMInvoker) Delete(address *net.IPNet, nwCfg *cni.NetworkConfig, options map[string]interface{}) error {
	var err error

	if len(address.IP.To4()) == 4 {

		// cleanup pool
		if options[optReleasePool] == optValPool {
			nwCfg.Ipam.Address = ""
		}

		nwCfg.Ipam.Subnet = invoker.nwInfo.Subnets[0].Prefix.String()
		log.Printf("Releasing ipv4 address :%s pool: %s",
			nwCfg.Ipam.Address, nwCfg.Ipam.Subnet)
		if err := invoker.plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg); err != nil {
			log.Printf("Failed to release ipv4 address: %v", err)
			err = invoker.plugin.Errorf("Failed to release ipv4 address: %v", err)
		}
	} else if len(address.IP.To16()) == 16 {
		nwCfgIpv6 := *nwCfg
		nwCfgIpv6.Ipam.Environment = common.OptEnvironmentIPv6NodeIpam
		nwCfgIpv6.Ipam.Type = ipamV6
		if len(invoker.nwInfo.Subnets) > 1 {
			nwCfgIpv6.Ipam.Subnet = invoker.nwInfo.Subnets[1].Prefix.String()
		}

		log.Printf("Releasing ipv6 address :%s pool: %s",
			nwCfgIpv6.Ipam.Address, nwCfgIpv6.Ipam.Subnet)
		if err = invoker.plugin.DelegateDel(nwCfgIpv6.Ipam.Type, &nwCfgIpv6); err != nil {
			log.Printf("Failed to release ipv6 address: %v", err)
			err = invoker.plugin.Errorf("Failed to release ipv6 address: %v", err)
		}
	} else if address == nil {
		if err = invoker.plugin.DelegateDel(nwCfg.Ipam.Type, nwCfg); err != nil {
			log.Printf("Network not found, attempted to release address with error:  %v", err)
		}
	} else {
		err = fmt.Errorf("Address is incorrect, not valid IPv4 or IPv6")
	}

	return err
}
