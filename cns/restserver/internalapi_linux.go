package restserver

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/network/networkutils"
	goiptables "github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
)

const SWIFTPOSTROUTING = "SWIFT-POSTROUTING"

type IPtablesProvider struct{}

func (c *IPtablesProvider) GetIPTables() (iptablesClient, error) {
	client, err := goiptables.New()
	return client, errors.Wrap(err, "failed to get iptables client")
}
func (c *IPtablesProvider) GetIPTablesLegacy() (iptablesLegacyClient, error) {
	return &iptablesLegacy{}, nil
}

type iptablesLegacy struct{}

func (c *iptablesLegacy) Delete(table, chain string, rulespec ...string) error {
	cmd := append([]string{"-t", table, "-D", chain}, rulespec...)
	return errors.Wrap(exec.Command("iptables-legacy", cmd...).Run(), "iptables legacy failed delete")
}

// nolint
func (service *HTTPRestService) programSNATRules(req *cns.CreateNetworkContainerRequest) (types.ResponseCode, string) {
	service.Lock()
	defer service.Unlock()

	iptl, err := service.iptables.GetIPTablesLegacy()
	if err == nil {
		err = iptl.Delete(iptables.Nat, iptables.Postrouting, "-j", SWIFTPOSTROUTING)
		// ignore if command fails
		if err == nil {
			logger.Printf("[Azure CNS] Deleted legacy jump to SWIFT-POSTROUTING Chain")
		}
	} else {
		logger.Printf("[Azure CNS] Could not create iptables legacy interface, continuing : %v", err)
	}

	ipt, err := service.iptables.GetIPTables()
	if err != nil {
		return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to create iptables interface : %v", err)
	}

	chainExist, err := ipt.ChainExists(iptables.Nat, SWIFTPOSTROUTING)
	if err != nil {
		return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of SWIFT-POSTROUTING chain: %v", err)
	}
	if !chainExist { // create and append chain if it doesn't exist
		logger.Printf("[Azure CNS] Creating SWIFT-POSTROUTING Chain ...")
		err = ipt.NewChain(iptables.Nat, SWIFTPOSTROUTING)
		if err != nil {
			return types.FailedToRunIPTableCmd, "[Azure CNS] failed to create SWIFT-POSTROUTING chain : " + err.Error()
		}
	}

	// reconcile jump to SWIFT-POSTROUTING chain
	rules, err := ipt.List(iptables.Nat, iptables.Postrouting)
	if err != nil {
		return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check rules in postrouting chain of nat table: %v", err)
	}
	swiftRuleIndex := len(rules) // append if neither jump rule from POSTROUTING is found
	// one time migration from old SWIFT chain
	// previously, CNI may have a jump to the SWIFT chain-- our jump to SWIFT-POSTROUTING needs to happen first
	for index, rule := range rules {
		if rule == "-A POSTROUTING -j SWIFT" {
			// jump to SWIFT comes before jump to SWIFT-POSTROUTING, so potential reordering required
			swiftRuleIndex = index
			break
		}
		if rule == "-A POSTROUTING -j SWIFT-POSTROUTING" {
			// jump to SWIFT-POSTROUTING comes before jump to SWIFT, which requires no further action
			swiftRuleIndex = -1
			break
		}
	}
	if swiftRuleIndex != -1 {
		// jump SWIFT rule exists, insert SWIFT-POSTROUTING rule at the same position so it ends up running first
		// first, remove any existing SWIFT-POSTROUTING rules to avoid duplicates
		// note: inserting at len(rules) and deleting a jump to SWIFT-POSTROUTING is mutually exclusive
		swiftPostroutingExists, err := ipt.Exists(iptables.Nat, iptables.Postrouting, "-j", SWIFTPOSTROUTING)
		if err != nil {
			return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of SWIFT-POSTROUTING rule: %v", err)
		}
		if swiftPostroutingExists {
			err = ipt.Delete(iptables.Nat, iptables.Postrouting, "-j", SWIFTPOSTROUTING)
			if err != nil {
				return types.FailedToRunIPTableCmd, "[Azure CNS] failed to delete existing SWIFT-POSTROUTING rule : " + err.Error()
			}
		}

		// slice index is 0-based, iptables insert is 1-based, but list also gives us the -P POSTROUTING ACCEPT
		// as the first rule so swiftRuleIndex gives us the correct 1-indexed iptables position.
		// Example:
		// -P POSTROUTING ACCEPT is at swiftRuleIndex 0
		// -A POSTROUTING -j SWIFT is at swiftRuleIndex 1, and iptables index 1
		logger.Printf("[Azure CNS] Inserting SWIFT-POSTROUTING Chain at iptables position %d", swiftRuleIndex)
		err = ipt.Insert(iptables.Nat, iptables.Postrouting, swiftRuleIndex, "-j", SWIFTPOSTROUTING)
		if err != nil {
			return types.FailedToRunIPTableCmd, "[Azure CNS] failed to insert SWIFT-POSTROUTING chain : " + err.Error()
		}
	}

	// use any secondary ip + the nnc prefix length to get an iptables rule to allow dns and imds traffic from the pods
	for _, v := range req.SecondaryIPConfigs {
		// check if the ip address is IPv4. A check is required because DNS and IMDS do not have IPv6 addresses. Since support currently exists only for IPv4, other ip families are skipped.
		if net.ParseIP(v.IPAddress).To4() == nil {
			// skip if the ip address is not IPv4
			continue
		}

		// put the ip address in standard cidr form (where we zero out the parts that are not relevant)
		_, podSubnet, _ := net.ParseCIDR(v.IPAddress + "/" + fmt.Sprintf("%d", req.IPConfiguration.IPSubnet.PrefixLength))

		// define all rules we want in the chain
		rules := [][]string{
			{"-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureDNS, "-p", iptables.UDP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", req.HostPrimaryIP},
			{"-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureDNS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.DNSPort), "-j", iptables.Snat, "--to", req.HostPrimaryIP},
			{"-m", "addrtype", "!", "--dst-type", "local", "-s", podSubnet.String(), "-d", networkutils.AzureIMDS, "-p", iptables.TCP, "--dport", strconv.Itoa(iptables.HTTPPort), "-j", iptables.Snat, "--to", req.HostPrimaryIP},
		}

		// check if all rules exist
		allRulesExist := true
		for _, rule := range rules {
			exists, err := ipt.Exists(iptables.Nat, SWIFTPOSTROUTING, rule...)
			if err != nil {
				return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to check for existence of rule: %v", err)
			}
			if !exists {
				allRulesExist = false
				break
			}
		}

		// get current rule count in SWIFT-POSTROUTING chain
		currentRules, err := ipt.List(iptables.Nat, SWIFTPOSTROUTING)
		if err != nil {
			return types.UnexpectedError, fmt.Sprintf("[Azure CNS] Error. Failed to list rules in SWIFT-POSTROUTING chain: %v", err)
		}

		// if rule count doesn't match or not all rules exist, reconcile
		// add one because there is always a singular starting rule in the chain, in addition to the ones we add
		if len(currentRules) != len(rules)+1 || !allRulesExist {
			logger.Printf("[Azure CNS] Reconciling SWIFT-POSTROUTING chain rules to SNAT Azure DNS and IMDS to Host IP")

			err = ipt.ClearChain(iptables.Nat, SWIFTPOSTROUTING)
			if err != nil {
				return types.FailedToRunIPTableCmd, "[Azure CNS] failed to flush SWIFT-POSTROUTING chain : " + err.Error()
			}

			for _, rule := range rules {
				err = ipt.Append(iptables.Nat, SWIFTPOSTROUTING, rule...)
				if err != nil {
					return types.FailedToRunIPTableCmd, "[Azure CNS] failed to append rule to SWIFT-POSTROUTING chain : " + err.Error()
				}
			}
			logger.Printf("[Azure CNS] Finished reconciling SWIFT-POSTROUTING chain")
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
