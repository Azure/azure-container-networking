package network

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime/debug"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
)

type AzureIPAMInvoker struct {
	plugin delegatePlugin
	nwInfo *network.NetworkInfo
}

type delegatePlugin interface {
	DelegateAdd(pluginName string, nwCfg *cni.NetworkConfig) (*cniTypesCurr.Result, error)
	DelegateDel(pluginName string, nwCfg *cni.NetworkConfig) error
	Errorf(format string, args ...interface{}) *cniTypes.Error
}

// Create an instance of CNI, then call a delegate function for CNI action (ADD/DEL)
// Avoid having IPAM state but no CNI state.
func NewAzureIpamInvoker(plugin *NetPlugin, nwInfo *network.NetworkInfo) *AzureIPAMInvoker {
	return &AzureIPAMInvoker{
		plugin: plugin,
		nwInfo: nwInfo,
	}
}

func (invoker *AzureIPAMInvoker) Add(addConfig IPAMAddConfig) (IPAMAddResult, error) {
	var (
		addResult = IPAMAddResult{}
		err       error
	)

	if addConfig.nwCfg == nil {
		return addResult, invoker.plugin.Errorf("nil nwCfg passed to CNI ADD, stack: %+v", string(debug.Stack()))
	}

	if len(invoker.nwInfo.Subnets) > 0 {
		addConfig.nwCfg.Ipam.Subnet = invoker.nwInfo.Subnets[0].Prefix.String()
	}

	// Call into IPAM plugin to allocate an address pool for the network.
	addResult.ipv4Result, err = invoker.plugin.DelegateAdd(addConfig.nwCfg.Ipam.Type, addConfig.nwCfg)
	if err != nil {

		if errors.Is(err, ipam.ErrNoAvailableAddressPools) {
			cniStateExists, er := platform.CheckIfFileExists(platform.CNIStateFilePath)
			if er != nil {
				log.Printf("[cni] Error checking CNI state exist: %v", er)
			}

			ipamStateExists, er := platform.CheckIfFileExists(platform.CNIIpamStatePath)
			if er != nil {
				log.Printf("[cni] Error checking IPAM state exist: %v", er)
			}

			if !cniStateExists && ipamStateExists {
				er = os.Remove(platform.CNIIpamStatePath)
				if er != nil {
					log.Printf("[cni] error deleting state file %v", er)
				}
			}
		}
		err = invoker.plugin.Errorf("Failed to allocate pool: %v", err)
		return addResult, err
	}

	defer func() {
		if err != nil {
			if len(addResult.ipv4Result.IPs) > 0 {
				if er := invoker.Delete(&addResult.ipv4Result.IPs[0].Address, addConfig.nwCfg, nil, addConfig.options); er != nil {
					err = invoker.plugin.Errorf("Failed to clean up IP's during Delete with error %v, after Add failed with error %w", er, err)
				}
			} else {
				err = fmt.Errorf("No IP's to delete on error: %v", err)
			}
		}
	}()

	if addConfig.nwCfg.IPV6Mode != "" {
		nwCfg6 := *addConfig.nwCfg
		nwCfg6.Ipam.Environment = common.OptEnvironmentIPv6NodeIpam
		nwCfg6.Ipam.Type = ipamV6

		if len(invoker.nwInfo.Subnets) > 1 {
			// ipv6 is the second subnet of the slice
			nwCfg6.Ipam.Subnet = invoker.nwInfo.Subnets[1].Prefix.String()
		}

		addResult.ipv6Result, err = invoker.plugin.DelegateAdd(nwCfg6.Ipam.Type, &nwCfg6)
		if err != nil {
			err = invoker.plugin.Errorf("Failed to allocate v6 pool: %v", err)
		}
	}

	addResult.hostSubnetPrefix = addResult.ipv4Result.IPs[0].Address

	return addResult, err
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
			if errors.Is(err, ipam.ErrNoAvailableAddressPools) {
				cniStateExists, er := platform.CheckIfFileExists(platform.CNIStateFilePath)
				if er != nil {
					log.Printf("[cni] Error checking CNI state exist: %v", er)
				}

				ipamStateExists, er := platform.CheckIfFileExists(platform.CNIIpamStatePath)
				if er != nil {
					log.Printf("[cni] Error checking IPAM state exist: %v", er)
				}

				if !cniStateExists && ipamStateExists {
					er = os.Remove(platform.CNIIpamStatePath)
					if er != nil {
						log.Printf("[cni] error deleting state file %v", er)
					}
				}
			}
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
