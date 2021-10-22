package policies

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
)

const (
	maxRetryCount           = 1
	unknownLineErrorPattern = "line (\\d+) failed" // TODO this could happen if syntax is off or AZURE-NPM-INGRESS doesn't exist for -A AZURE-NPM-INGRESS -j hash(NP1) ...
	knownLineErrorPattern   = "Error occurred at line: (\\d+)"

	chainSectionPrefix = "chain" // TODO is this necessary?
)

// shouldn't call this if the np has no ACLs (check in generic)
func (pMgr *PolicyManager) addPolicy(networkPolicy *NPMNetworkPolicy, _ []string) error {
	// TODO check for newPolicy errors
	creator := pMgr.getCreatorForNewNetworkPolicies([]*NPMNetworkPolicy{networkPolicy})
	err := restore(creator)
	if err != nil {
		return npmerrors.Errorf("AddPolicy", false, fmt.Sprintf("failed to restore iptables with updated policies: %v", err))
	}
	return nil
}

func (pMgr *PolicyManager) removePolicy(name string, _ []string) error {
	networkPolicy := pMgr.policyMap.cache[name]
	pMgr.deleteOldJumpRulesOnRemove(networkPolicy) // TODO get error
	creator := pMgr.getCreatorForRemovingPolicies([]*NPMNetworkPolicy{networkPolicy})
	err := restore(creator)
	if err != nil {
		return npmerrors.Errorf("RemovePolicy", false, fmt.Sprintf("failed to flush policies: %v", err))
	}
	return nil
}

func restore(creator *ioutil.FileCreator) error {
	return creator.RunCommandWithFile(util.IptablesRestore, util.IptablesRestoreTableFlag, util.IptablesFilterTable, util.IptablesRestoreNoFlushFlag)
}

func (pMgr *PolicyManager) getCreatorForRemovingPolicies(networkPolicies []*NPMNetworkPolicy) *ioutil.FileCreator {
	allChainNames := getAllChainNames(networkPolicies)
	creator := pMgr.getNewCreatorWithChains(allChainNames)
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return creator
}

// returns all chain names (ingress and egress policy chain names)
func getAllChainNames(networkPolicies []*NPMNetworkPolicy) []string {
	chainNames := make([]string, 0, 2*len(networkPolicies)) // 1-2 elements per np
	for _, networkPolicy := range networkPolicies {
		hasIngress, hasEgress := networkPolicy.hasIngressAndEgress()

		if hasIngress {
			chainNames = append(chainNames, networkPolicy.getIngressChainName())
		}
		if hasEgress {
			chainNames = append(chainNames, networkPolicy.getEgressChainName())
		}
	}
	return chainNames
}

// returns two booleans indicating whether the network policy has ingress and egress respectively
func (networkPolicy *NPMNetworkPolicy) hasIngressAndEgress() (bool, bool) {
	hasIngress := false
	hasEgress := false
	for _, aclPolicy := range networkPolicy.ACLs {
		hasIngress = hasIngress || aclPolicy.hasIngress()
		hasEgress = hasEgress || aclPolicy.hasEgress()
	}
	return hasIngress, hasEgress
}

func (networkPolicy *NPMNetworkPolicy) getEgressChainName() string {
	return networkPolicy.getChainName(util.IptablesAzureEgressPolicyChainPrefix)
}

func (networkPolicy *NPMNetworkPolicy) getIngressChainName() string {
	return networkPolicy.getChainName(util.IptablesAzureIngressPolicyChainPrefix)
}

func (networkPolicy *NPMNetworkPolicy) getChainName(prefix string) string {
	policyHash := util.Hash(networkPolicy.Name) // assuming the name is unique
	return joinWithDash(prefix, policyHash)
}

func (pMgr *PolicyManager) getNewCreatorWithChains(chainNames []string) *ioutil.FileCreator {
	creator := ioutil.NewFileCreator(pMgr.ioShim, maxRetryCount, knownLineErrorPattern, unknownLineErrorPattern) // TODO pass an array instead of this ... thing

	creator.AddLine("", nil, "*"+util.IptablesFilterTable) // specify the table
	for _, chainName := range chainNames {
		// add chain headers
		sectionID := joinWithDash(chainSectionPrefix, chainName)
		counters := "-" // TODO specify counters eventually? would need iptables-save file
		creator.AddLine(sectionID, nil, ":"+chainName, "-", counters)
		// TODO remove sections??
	}
	return creator
}

// will make a similar func for on update eventually
func (pMgr *PolicyManager) deleteOldJumpRulesOnRemove(policy *NPMNetworkPolicy) {
	shouldDeleteIngress, shouldDeleteEgress := policy.hasIngressAndEgress()
	if shouldDeleteIngress {
		pMgr.deleteIngressJumpRule(policy)
	}
	if shouldDeleteEgress {
		pMgr.deleteEgressJumpRule(policy)
	}
}

func (pMgr *PolicyManager) deleteIngressJumpRule(policy *NPMNetworkPolicy) {
	errCode, err := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, getIngressJumpSpecs(policy)...)
	if err != nil {
		log.Errorf("failed to delete jump to ingress rule for policy %s with error [%w] and exit code %d", policy.Name, err, errCode)
	}
}

func (pMgr *PolicyManager) deleteEgressJumpRule(policy *NPMNetworkPolicy) {
	errCode, err := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, getEgressJumpSpecs(policy)...)
	if err != nil {
		log.Errorf("failed to delete jump to egress rule for policy %s with error [%w] and exit code %d", policy.Name, err, errCode)
	}
}

func getIngressJumpSpecs(networkPolicy *NPMNetworkPolicy) []string {
	chainName := networkPolicy.getIngressChainName()
	specs := []string{util.IptablesAzureIngressChain, util.IptablesJumpFlag, chainName}
	return append(specs, getMatchSetSpecsForNetworkPolicy(networkPolicy, DstMatch)...)
}

func getEgressJumpSpecs(networkPolicy *NPMNetworkPolicy) []string {
	chainName := networkPolicy.getEgressChainName()
	specs := []string{util.IptablesAzureEgressChain, util.IptablesJumpFlag, chainName}
	return append(specs, getMatchSetSpecsForNetworkPolicy(networkPolicy, SrcMatch)...)
}

// noflush add to chains impacted
func (pMgr *PolicyManager) getCreatorForNewNetworkPolicies(networkPolicies []*NPMNetworkPolicy) *ioutil.FileCreator {
	allChainNames := getAllChainNames(networkPolicies)
	creator := pMgr.getNewCreatorWithChains(allChainNames)

	for _, networkPolicy := range networkPolicies {
		writeNetworkPolicyRules(creator, networkPolicy)

		// add jump rule(s) to policy chain(s)
		hasIngress, hasEgress := networkPolicy.hasIngressAndEgress()
		if hasIngress {
			ingressJumpSpecs := append([]string{"-A"}, getIngressJumpSpecs(networkPolicy)...)
			creator.AddLine("", nil, ingressJumpSpecs...) // TODO error handler
		}
		if hasEgress {
			egressJumpSpecs := append([]string{"-A"}, getEgressJumpSpecs(networkPolicy)...)
			creator.AddLine("", nil, egressJumpSpecs...) // TODO error networkPolicy
		}
	}
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return creator
}

// write rules for the policy chain(s)
func writeNetworkPolicyRules(creator *ioutil.FileCreator, networkPolicy *NPMNetworkPolicy) {
	for _, aclPolicy := range networkPolicy.ACLs {
		var chainName string
		var actionSpecs []string
		if aclPolicy.Direction == Ingress {
			chainName = networkPolicy.getIngressChainName()
			if aclPolicy.Target == Allowed {
				actionSpecs = []string{util.IptablesJumpFlag, util.IptablesAzureEgressChain}
			} else {
				actionSpecs = getSetMarkSpecs(util.IptablesAzureIngressDropMarkHex)
			}
		} else {
			chainName = networkPolicy.getEgressChainName()
			if aclPolicy.Target == Allowed {
				actionSpecs = []string{util.IptablesJumpFlag, util.IptablesAzureAcceptChain}
			} else {
				actionSpecs = getSetMarkSpecs(util.IptablesAzureEgressDropMarkHex)
			}
		}
		line := []string{"-A", chainName}
		line = append(line, actionSpecs...)
		line = append(line, getIPTablesRuleSpecs(aclPolicy)...)
		creator.AddLine("", nil, line...) // TODO add error handler
	}
}

func getIPTablesRuleSpecs(aclPolicy *ACLPolicy) []string {
	specs := make([]string, 0)
	specs = append(specs, util.IptablesProtFlag, string(aclPolicy.Protocol)) // NOTE: protocol must be ALL instead of nil
	specs = append(specs, getPortSpecs(aclPolicy.SrcPorts, false)...)
	specs = append(specs, getPortSpecs(aclPolicy.DstPorts, true)...)
	specs = append(specs, getMatchSetSpecsFromSetInfo(aclPolicy.SrcList)...)
	specs = append(specs, getMatchSetSpecsFromSetInfo(aclPolicy.DstList)...)
	if aclPolicy.Comment != "" {
		specs = append(specs, getCommentSpecs(aclPolicy.Comment)...)
	}
	return specs
}

func getPortSpecs(portRanges []Ports, isDst bool) []string {
	if len(portRanges) == 0 {
		return []string{}
	}
	if len(portRanges) == 1 {
		portFlag := util.IptablesSrcPortFlag
		if isDst {
			portFlag = util.IptablesDstPortFlag
		}
		return []string{portFlag, portRanges[0].toIPTablesString()}
	}

	portRangeStrings := make([]string, 0)
	for _, portRange := range portRanges {
		portRangeStrings = append(portRangeStrings, portRange.toIPTablesString())
	}
	portFlag := util.IptablesMultiSrcPortFlag
	if isDst {
		portFlag = util.IptablesMultiDstPortFlag
	}
	specs := []string{util.IptablesModuleFlag, util.IptablesMultiportFlag, portFlag}
	return append(specs, strings.Join(portRangeStrings, ","))
}

func getMatchSetSpecsForNetworkPolicy(networkPolicy *NPMNetworkPolicy, matchType MatchType) []string {
	specs := make([]string, 0, 5*len(networkPolicy.PodSelectorIPSets)) // 5 elements per ipset
	for _, translatedIPSet := range networkPolicy.PodSelectorIPSets {
		matchString := matchType.toIPTablesString()
		hashedSetName := util.GetHashedName(translatedIPSet.Metadata.GetPrefixName())
		specs = append(specs, util.IptablesModuleFlag, util.IptablesSetModuleFlag, util.IptablesMatchSetFlag, hashedSetName, matchString)
	}
	return specs
}

func getMatchSetSpecsFromSetInfo(setInfoList []SetInfo) []string {
	specs := make([]string, 0, 6*len(setInfoList)) // 5-6 elements per setInfo
	for _, setInfo := range setInfoList {
		matchString := setInfo.MatchType.toIPTablesString()
		specs = append(specs, util.IptablesModuleFlag, util.IptablesSetModuleFlag)
		if !setInfo.Included {
			specs = append(specs, util.IptablesNotFlag)
		}
		hashedSetName := util.GetHashedName(setInfo.IPSet.GetPrefixName())
		specs = append(specs, util.IptablesMatchSetFlag, hashedSetName, matchString)
	}
	return specs
}

func getSetMarkSpecs(mark string) []string {
	return []string{
		util.IptablesJumpFlag,
		util.IptablesMark,
		util.IptablesSetMarkFlag,
		mark,
	}
}

func getCommentSpecs(comment string) []string {
	return []string{
		util.IptablesModuleFlag,
		util.IptablesCommentModuleFlag,
		util.IptablesCommentFlag,
		comment,
	}
}

func joinWithDash(prefix, item string) string {
	return fmt.Sprintf("%s-%s", prefix, item)
}

func checkForErrors(networkPolicies []*NPMNetworkPolicy) error {
	// TODO make sure comment doesn't have any whitespace??
	for _, networkPolicy := range networkPolicies {
		for _, aclPolicy := range networkPolicy.ACLs {
			if !aclPolicy.hasKnownTarget() {
				return fmt.Errorf("ACL policy %s has unknown target", aclPolicy.PolicyID)
			}
			if !aclPolicy.hasKnownDirection() {
				return fmt.Errorf("ACL policy %s has unknown direction", aclPolicy.PolicyID)
			}
			if !aclPolicy.hasKnownProtocol() {
				return fmt.Errorf("ACL policy %s has unknown protocol (set to All if desired)", aclPolicy.PolicyID)
			}
			if !aclPolicy.satisifiesPortAndProtocolConstraints() {
				return fmt.Errorf("ACL policy %s has multiple src or dst ports, so must have protocol tcp, udp, udplite, sctp, or dccp but has protocol %s", aclPolicy.PolicyID, string(aclPolicy.Protocol))
			}
			for _, portRange := range aclPolicy.DstPorts {
				if !portRange.isValidRange() {
					return fmt.Errorf("ACL policy %s has invalid port range in DstPorts (start: %d, end: %d)", aclPolicy.PolicyID, portRange.Port, portRange.EndPort)
				}
			}
			for _, portRange := range aclPolicy.DstPorts {
				if !portRange.isValidRange() {
					return fmt.Errorf("ACL policy %s has invalid port range in SrcPorts (start: %d, end: %d)", aclPolicy.PolicyID, portRange.Port, portRange.EndPort)
				}
			}
			for _, setInfo := range aclPolicy.SrcList {
				if !setInfo.hasKnownMatchType() {
					return fmt.Errorf("ACL policy %s has set %s in SrcList with unknown Match Type", aclPolicy.PolicyID, setInfo.IPSet.Name)
				}
			}
			for _, setInfo := range aclPolicy.DstList {
				if !setInfo.hasKnownMatchType() {
					return fmt.Errorf("ACL policy %s has set %s in DstList with unknown Match Type", aclPolicy.PolicyID, setInfo.IPSet.Name)
				}
			}
		}
	}
	return nil
}
