package restserver

import (
	"fmt"
	"net"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/network/networkutils"
	goiptables "github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
)

const SWIFT = "SWIFT-POSTROUTING"

type IPtablesProvider struct{}

func (c *IPtablesProvider) GetIPTables() (iptablesClient, error) {
	client, err := goiptables.New()
	return client, errors.Wrap(err, "failed to get iptables client")
}

// nolint
func (service *HTTPRestService) programSNATRules(req *cns.CreateNetworkContainerRequest) (types.ResponseCode, string) {
	service.Lock()
	defer service.Unlock()

	// Parse primary ip and ipnet from nnc
	// in podsubnet case, ncPrimaryIP is the pod subnet's primary ip
	// in vnet scale case, ncPrimaryIP is the node's ip
	ncPrimaryIP, _, _ := net.ParseCIDR(req.IPConfiguration.IPSubnet.IPAddress + "/" + fmt.Sprintf("%d", req.IPConfiguration.IPSubnet.PrefixLength))
	ipt, err := service.iptables.GetIPTables()
	if err != nil {
		return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to create iptables interface : %v", err)
	}

	chainExist, err := ipt.ChainExists(iptables.Nat, SWIFT)
	if err != nil {
		return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of SWIFT chain: %v", err)
	}
	if !chainExist { // create and append chain if it doesn't exist
		logger.Printf("[Azure CNS] Creating SWIFT Chain ...")
		err = ipt.NewChain(iptables.Nat, SWIFT)
		if err != nil {
			return types.FailedToRunIPTableCmd, "[Azure CNS] failed to create SWIFT chain : " + err.Error()
		}
		logger.Printf("[Azure CNS] Append SWIFT Chain to POSTROUTING ...")
		err = ipt.Append(iptables.Nat, iptables.Postrouting, "-j", SWIFT)
		if err != nil {
			return types.FailedToRunIPTableCmd, "[Azure CNS] failed to append SWIFT chain : " + err.Error()
		}
	}

	postroutingToSwiftJumpexist, err := ipt.Exists(iptables.Nat, iptables.Postrouting, "-j", SWIFT)
	if err != nil {
		return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of POSTROUTING to SWIFT chain jump: %v", err)
	}
	if !postroutingToSwiftJumpexist {
		logger.Printf("[Azure CNS] Append SWIFT Chain to POSTROUTING ...")
		err = ipt.Append(iptables.Nat, iptables.Postrouting, "-j", SWIFT)
		if err != nil {
			return types.FailedToRunIPTableCmd, "[Azure CNS] failed to append SWIFT chain : " + err.Error()
		}
	}

	// use any secondary ip + the nnc prefix length to get an iptables rule to allow dns and imds traffic from the pods
	for _, v := range req.SecondaryIPConfigs {
		// check if the ip address is IPv4
		if net.ParseIP(v.IPAddress).To4() == nil {
			// skip if the ip address is not IPv4
			continue
		}

		// put the ip address in standard cidr form (where we zero out the parts that are not relevant)
		_, podSubnet, _ := net.ParseCIDR(v.IPAddress + "/" + fmt.Sprintf("%d", req.IPConfiguration.IPSubnet.PrefixLength))

		snatUDPRuleExists, err := ipt.Exists(iptables.Nat, SWIFT, "-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureDNS, "-p", iptables.UDP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", ncPrimaryIP.String())
		if err != nil {
			return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of pod SNAT UDP rule : %v", err)
		}
		if !snatUDPRuleExists {
			logger.Printf("[Azure CNS] Inserting pod SNAT UDP rule ...")
			err = ipt.Insert(iptables.Nat, SWIFT, 1, "-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureDNS, "-p", iptables.UDP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", ncPrimaryIP.String())
			if err != nil {
				return types.FailedToRunIPTableCmd, "[Azure CNS] failed to insert pod SNAT UDP rule : " + err.Error()
			}
		}

		snatPodTCPRuleExists, err := ipt.Exists(iptables.Nat, SWIFT, "-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureDNS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", ncPrimaryIP.String())
		if err != nil {
			return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of pod SNAT TCP rule : %v", err)
		}
		if !snatPodTCPRuleExists {
			logger.Printf("[Azure CNS] Inserting pod SNAT TCP rule ...")
			err = ipt.Insert(iptables.Nat, SWIFT, 1, "-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureDNS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", ncPrimaryIP.String())
			if err != nil {
				return types.FailedToRunIPTableCmd, "[Azure CNS] failed to insert pod SNAT TCP rule : " + err.Error()
			}
		}

		snatIMDSRuleexist, err := ipt.Exists(iptables.Nat, SWIFT, "-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureIMDS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.HTTPPort), "-j", iptables.Snat, "--to", req.HostPrimaryIP)
		if err != nil {
			return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of pod SNAT IMDS rule : %v", err)
		}
		if !snatIMDSRuleexist {
			logger.Printf("[Azure CNS] Inserting pod SNAT IMDS rule ...")
			err = ipt.Insert(iptables.Nat, SWIFT, 1, "-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureIMDS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.HTTPPort), "-j", iptables.Snat, "--to", req.HostPrimaryIP)
			if err != nil {
				return types.FailedToRunIPTableCmd, "[Azure CNS] failed to insert pod SNAT IMDS rule : " + err.Error()
			}
		}

		// we only need to run this code once as the iptable rule applies to all secondary ip configs in the same subnet
		break
	}

	return types.Success, ""
}

// no-op for linux
func (service *HTTPRestService) setVFForAccelnetNICs() error {
	return nil
}
