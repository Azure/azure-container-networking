package policies

import (
	"fmt"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
)

const (
	maxRetryCount           = 1
	unknownLineErrorPattern = "line (\\d+) failed" // TODO this could happen if syntax is off or AZURE-NPM-INGRESS doesn't exist for -A AZURE-NPM-INGRESS -j hash(NP1) ...
	knownLineErrorPattern   = "Error occurred at line: (\\d+)"

	chainSectionPrefix        = "chain" // TODO are sections necessary for error handling?
	maxLengthForMatchSetSpecs = 6       // 5-6 elements depending on Included boolean
)

// shouldn't call this if the np has no ACLs (check in generic)
func (pMgr *PolicyManager) addPolicy(networkPolicy *NPMNetworkPolicy, _ map[string]string) error {
	// TODO check for newPolicy errors
	creator := pMgr.getCreatorForNewNetworkPolicies(networkPolicy)
	err := restore(creator)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to restore iptables with updated policies", err)
	}
	return nil
}

func (pMgr *PolicyManager) removePolicy(networkPolicy *NPMNetworkPolicy, _ map[string]string) error {
	deleteErr := pMgr.deleteOldJumpRulesOnRemove(networkPolicy)
	if deleteErr != nil {
		return npmerrors.SimpleErrorWrapper("failed to delete jumps to policy chains", deleteErr)
	}
	creator := pMgr.getCreatorForRemovingPolicies(networkPolicy)
	restoreErr := restore(creator)
	if restoreErr != nil {
		return npmerrors.SimpleErrorWrapper("failed to flush policies", restoreErr)
	}
	return nil
}

func restore(creator *ioutil.FileCreator) error {
	err := creator.RunCommandWithFile(util.IptablesRestore, util.IptablesRestoreTableFlag, util.IptablesFilterTable, util.IptablesRestoreNoFlushFlag)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to restore iptables file", err)
	}
	return nil
}

func (pMgr *PolicyManager) getCreatorForRemovingPolicies(networkPolicies ...*NPMNetworkPolicy) *ioutil.FileCreator {
	allChainNames := getAllChainNames(networkPolicies)
	creator := pMgr.getNewCreatorWithChains(allChainNames)
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return creator
}

// returns all chain names (ingress and egress policy chain names)
func getAllChainNames(networkPolicies []*NPMNetworkPolicy) []string {
	chainNames := make([]string, 0)
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
func (networkPolicy *NPMNetworkPolicy) hasIngressAndEgress() (hasIngress, hasEgress bool) {
	hasIngress = false
	hasEgress = false
	for _, aclPolicy := range networkPolicy.ACLs {
		hasIngress = hasIngress || aclPolicy.hasIngress()
		hasEgress = hasEgress || aclPolicy.hasEgress()
	}
	return
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
func (pMgr *PolicyManager) deleteOldJumpRulesOnRemove(policy *NPMNetworkPolicy) error {
	shouldDeleteIngress, shouldDeleteEgress := policy.hasIngressAndEgress()
	if shouldDeleteIngress {
		if err := pMgr.deleteJumpRule(policy, true); err != nil {
			return err
		}
	}
	if shouldDeleteEgress {
		if err := pMgr.deleteJumpRule(policy, false); err != nil {
			return err
		}
	}
	return nil
}

func (pMgr *PolicyManager) deleteJumpRule(policy *NPMNetworkPolicy, isIngress bool) error {
	var specs []string
	var baseChainName string
	var chainName string
	if isIngress {
		specs = getIngressJumpSpecs(policy)
		baseChainName = util.IptablesAzureIngressChain
		chainName = policy.getIngressChainName()
	} else {
		specs = getEgressJumpSpecs(policy)
		baseChainName = util.IptablesAzureEgressChain
		chainName = policy.getEgressChainName()
	}

	specs = append([]string{baseChainName}, specs...)
	errCode, err := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, specs...)
	if err != nil && errCode != couldntLoadTargetErrorCode {
		// TODO check rule doesn't exist error code instead because the chain should exist
		errorString := fmt.Sprintf("failed to delete jump from %s chain to %s chain for policy %s with exit code %d", baseChainName, chainName, policy.Name, errCode)
		log.Errorf(errorString+": %w", err)
		return npmerrors.SimpleErrorWrapper(errorString, err)
	}
	return nil
}

func getIngressJumpSpecs(networkPolicy *NPMNetworkPolicy) []string {
	chainName := networkPolicy.getIngressChainName()
	specs := []string{util.IptablesJumpFlag, chainName}
	return append(specs, getMatchSetSpecsForNetworkPolicy(networkPolicy, DstMatch)...)
}

func getEgressJumpSpecs(networkPolicy *NPMNetworkPolicy) []string {
	chainName := networkPolicy.getEgressChainName()
	specs := []string{util.IptablesJumpFlag, chainName}
	return append(specs, getMatchSetSpecsForNetworkPolicy(networkPolicy, SrcMatch)...)
}

// noflush add to chains impacted
func (pMgr *PolicyManager) getCreatorForNewNetworkPolicies(networkPolicies ...*NPMNetworkPolicy) *ioutil.FileCreator {
	allChainNames := getAllChainNames(networkPolicies)
	creator := pMgr.getNewCreatorWithChains(allChainNames)

	ingressJumpLineNumber := 1
	egressJumpLineNumber := 1
	for _, networkPolicy := range networkPolicies {
		writeNetworkPolicyRules(creator, networkPolicy)

		// add jump rule(s) to policy chain(s)
		hasIngress, hasEgress := networkPolicy.hasIngressAndEgress()
		if hasIngress {
			ingressJumpSpecs := getInsertSpecs(util.IptablesAzureIngressChain, ingressJumpLineNumber, getIngressJumpSpecs(networkPolicy))
			creator.AddLine("", nil, ingressJumpSpecs...) // TODO error handler
			ingressJumpLineNumber++
		}
		if hasEgress {
			egressJumpSpecs := getInsertSpecs(util.IptablesAzureEgressChain, egressJumpLineNumber, getEgressJumpSpecs(networkPolicy))
			creator.AddLine("", nil, egressJumpSpecs...) // TODO error handler
			egressJumpLineNumber++
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
		if aclPolicy.hasIngress() {
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
	specs = append(specs, getPortSpecs([]Ports{aclPolicy.DstPorts})...)
	specs = append(specs, getMatchSetSpecsFromSetInfo(aclPolicy.SrcList)...)
	specs = append(specs, getMatchSetSpecsFromSetInfo(aclPolicy.DstList)...)
	if aclPolicy.Comment != "" {
		specs = append(specs, getCommentSpecs(aclPolicy.Comment)...)
	}
	return specs
}

func getPortSpecs(portRanges []Ports) []string {
	// TODO(jungukcho): do not need to take slices since it can only have one dst port
	if len(portRanges) != 1 {
		return []string{}
	}

	// TODO(jungukcho): temporary solution and need to fix it.
	if portRanges[0].Port == 0 && portRanges[0].EndPort == 0 {
		return []string{}
	}

	return []string{util.IptablesDstPortFlag, portRanges[0].toIPTablesString()}
}

func getMatchSetSpecsForNetworkPolicy(networkPolicy *NPMNetworkPolicy, matchType MatchType) []string {
	// TODO update to use included boolean/new data structure from Junguk's PR
	specs := make([]string, 0, maxLengthForMatchSetSpecs*len(networkPolicy.PodSelectorIPSets))
	for _, translatedIPSet := range networkPolicy.PodSelectorIPSets {
		matchString := matchType.toIPTablesString()
		hashedSetName := util.GetHashedName(translatedIPSet.Metadata.GetPrefixName())
		specs = append(specs, util.IptablesModuleFlag, util.IptablesSetModuleFlag, util.IptablesMatchSetFlag, hashedSetName, matchString)
	}
	return specs
}

func getMatchSetSpecsFromSetInfo(setInfoList []SetInfo) []string {
	specs := make([]string, 0, maxLengthForMatchSetSpecs*len(setInfoList))
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

func getInsertSpecs(chainName string, index int, specs []string) []string {
	indexString := fmt.Sprint(index)
	insertSpecs := []string{util.IptablesInsertionFlag, chainName, indexString}
	return append(insertSpecs, specs...)
}

func joinWithDash(prefix, item string) string {
	return fmt.Sprintf("%s-%s", prefix, item)
}
