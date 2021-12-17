package policies

import (
	"fmt"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
)

const (
	maxTryCount             = 1
	unknownLineErrorPattern = "line (\\d+) failed" // TODO this could happen if syntax is off or AZURE-NPM-INGRESS doesn't exist for -A AZURE-NPM-INGRESS -j hash(NP1) ...
	knownLineErrorPattern   = "Error occurred at line: (\\d+)"

	chainSectionPrefix        = "chain" // TODO are sections necessary for error handling?
	maxLengthForMatchSetSpecs = 6       // 5-6 elements depending on Included boolean
)

func (pMgr *PolicyManager) addPolicy(networkPolicy *NPMNetworkPolicy, _ map[string]string) error {
	// 1. Add rules for the network policies and activate NPM (if necessary).
	chainsToCreate := allChainNames([]*NPMNetworkPolicy{networkPolicy})
	creator := pMgr.creatorForNewNetworkPolicies(chainsToCreate, []*NPMNetworkPolicy{networkPolicy})
	err := restore(creator)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to restore iptables with updated policies", err)
	}

	// 2. Make sure the new chains don't get deleted in the background
	for _, chain := range chainsToCreate {
		pMgr.staleChains.remove(chain)
	}
	return nil
}

func (pMgr *PolicyManager) removePolicy(networkPolicy *NPMNetworkPolicy, _ map[string]string) error {
	// 1. Delete jump rules from ingress/egress chains to ingress/egress policy chains.
	// We ought to delete these jump rules here in the foreground since if we add an NP back after deleting, iptables-restore --noflush can add duplicate jump rules.
	deleteErr := pMgr.deleteOldJumpRulesOnRemove(networkPolicy)
	if deleteErr != nil {
		return npmerrors.SimpleErrorWrapper("failed to delete jumps to policy chains", deleteErr)
	}

	// 2. Flush the policy chains and deactivate NPM (if necessary).
	chainsToDelete := allChainNames([]*NPMNetworkPolicy{networkPolicy})
	creator := pMgr.creatorForRemovingPolicies(chainsToDelete)
	restoreErr := restore(creator)
	if restoreErr != nil {
		return npmerrors.SimpleErrorWrapper("failed to flush policies", restoreErr)
	}

	// 3. Delete policy chains in the background.
	for _, chain := range chainsToDelete {
		pMgr.staleChains.add(chain)
	}
	return nil
}

func restore(creator *ioutil.FileCreator) error {
	err := creator.RunCommandWithFile(util.IptablesRestore, util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesRestoreTableFlag, util.IptablesFilterTable, util.IptablesRestoreNoFlushFlag)
	if err != nil {
		return npmerrors.SimpleErrorWrapper("failed to restore iptables file", err)
	}
	return nil
}

func (pMgr *PolicyManager) creatorForRemovingPolicies(allChainNames []string) *ioutil.FileCreator {
	creator := pMgr.newCreatorWithChains(nil)
	// 1. Deactivate NPM (if necessary).
	if pMgr.isLastPolicy() {
		creator.AddLine("", nil, util.IptablesFlushFlag, util.IptablesAzureChain)
	}

	// 2. Flush the policy chains.
	for _, chainName := range allChainNames {
		creator.AddLine("", nil, util.IptablesFlushFlag, chainName)
	}
	creator.AddLine("", nil, util.IptablesRestoreCommit)
	return creator
}

// returns all chain names (ingress and egress policy chain names)
func allChainNames(networkPolicies []*NPMNetworkPolicy) []string {
	chainNames := make([]string, 0)
	for _, networkPolicy := range networkPolicies {
		hasIngress, hasEgress := networkPolicy.hasIngressAndEgress()

		if hasIngress {
			chainNames = append(chainNames, networkPolicy.ingressChainName())
		}
		if hasEgress {
			chainNames = append(chainNames, networkPolicy.egressChainName())
		}
	}
	return chainNames
}

func (pMgr *PolicyManager) newCreatorWithChains(chainNames []string) *ioutil.FileCreator {
	creator := ioutil.NewFileCreator(pMgr.ioShim, maxTryCount, knownLineErrorPattern, unknownLineErrorPattern) // TODO pass an array instead of this ... thing

	creator.AddLine("", nil, "*"+util.IptablesFilterTable) // specify the table
	for _, chainName := range chainNames {
		// add chain headers
		sectionID := joinWithDash(chainSectionPrefix, chainName)
		counters := "-"
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

func (pMgr *PolicyManager) deleteJumpRule(policy *NPMNetworkPolicy, direction UniqueDirection) error {
	var specs []string
	var baseChainName string
	var chainName string
	if direction == forIngress {
		specs = ingressJumpSpecs(policy)
		baseChainName = util.IptablesAzureIngressChain
		chainName = policy.ingressChainName()
	} else {
		specs = egressJumpSpecs(policy)
		baseChainName = util.IptablesAzureEgressChain
		chainName = policy.egressChainName()
	}

	specs = append([]string{baseChainName}, specs...)
	errCode, err := pMgr.runIPTablesCommand(util.IptablesDeletionFlag, specs...)
	if err != nil && errCode != couldntLoadTargetErrorCode {
		// TODO check rule doesn't exist error code instead because the chain should exist
		errorString := fmt.Sprintf("failed to delete jump from %s chain to %s chain for policy %s with exit code %d", baseChainName, chainName, policy.PolicyKey, errCode)
		log.Errorf(errorString+": %w", err)
		return npmerrors.SimpleErrorWrapper(errorString, err)
	}
	return nil
}

func ingressJumpSpecs(networkPolicy *NPMNetworkPolicy) []string {
	chainName := networkPolicy.ingressChainName()
	specs := []string{util.IptablesJumpFlag, chainName}
	specs = append(specs, matchSetSpecsForNetworkPolicy(networkPolicy, DstMatch)...)
	specs = append(specs, commentSpecs(networkPolicy.commentForJumpToIngress())...)
	return specs
}

func egressJumpSpecs(networkPolicy *NPMNetworkPolicy) []string {
	chainName := networkPolicy.egressChainName()
	specs := []string{util.IptablesJumpFlag, chainName}
	specs = append(specs, matchSetSpecsForNetworkPolicy(networkPolicy, SrcMatch)...)
	specs = append(specs, commentSpecs(networkPolicy.commentForJumpToEgress())...)
	return specs
}

func (pMgr *PolicyManager) creatorForNewNetworkPolicies(policyChains []string, networkPolicies []*NPMNetworkPolicy) *ioutil.FileCreator {
	creator := pMgr.newCreatorWithChains(policyChains)

	// 1. Activate NPM if necessary
	if pMgr.isFirstPolicy() {
		creator.AddLine("", nil, util.IptablesFlushFlag, util.IptablesAzureChain) // flush just in case there are old rules
		creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureIngressChain)
		creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureEgressChain)
		creator.AddLine("", nil, util.IptablesAppendFlag, util.IptablesAzureChain, util.IptablesJumpFlag, util.IptablesAzureAcceptChain)
	}

	// 2. Add all rules for the network policies
	ingressJumpLineNumber := 1
	egressJumpLineNumber := 1
	for _, networkPolicy := range networkPolicies {
		// 2.1 add all rules for the policy chain(s)
		writeNetworkPolicyRules(creator, networkPolicy)

		// 2.2 add jump rule(s) to the policy chain(s)
		hasIngress, hasEgress := networkPolicy.hasIngressAndEgress()
		if hasIngress {
			ingressJumpSpecs := insertSpecs(util.IptablesAzureIngressChain, ingressJumpLineNumber, ingressJumpSpecs(networkPolicy))
			creator.AddLine("", nil, ingressJumpSpecs...) // TODO error handler
			ingressJumpLineNumber++
		}
		if hasEgress {
			egressJumpSpecs := insertSpecs(util.IptablesAzureEgressChain, egressJumpLineNumber, egressJumpSpecs(networkPolicy))
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
			chainName = networkPolicy.ingressChainName()
			if aclPolicy.Target == Allowed {
				actionSpecs = []string{util.IptablesJumpFlag, util.IptablesAzureIngressAllowMarkChain}
			} else {
				actionSpecs = setMarkSpecs(util.IptablesAzureIngressDropMarkHex)
			}
		} else {
			chainName = networkPolicy.egressChainName()
			if aclPolicy.Target == Allowed {
				actionSpecs = []string{util.IptablesJumpFlag, util.IptablesAzureAcceptChain}
			} else {
				actionSpecs = setMarkSpecs(util.IptablesAzureEgressDropMarkHex)
			}
		}
		line := []string{"-A", chainName}
		line = append(line, actionSpecs...)
		line = append(line, iptablesRuleSpecs(aclPolicy)...)
		creator.AddLine("", nil, line...) // TODO add error handler
	}
}

func iptablesRuleSpecs(aclPolicy *ACLPolicy) []string {
	specs := make([]string, 0)
	if aclPolicy.Protocol != UnspecifiedProtocol {
		specs = append(specs, util.IptablesProtFlag, string(aclPolicy.Protocol))
	}
	specs = append(specs, dstPortSpecs(aclPolicy.DstPorts)...)
	specs = append(specs, matchSetSpecsFromSetInfo(aclPolicy.SrcList)...)
	specs = append(specs, matchSetSpecsFromSetInfo(aclPolicy.DstList)...)
	specs = append(specs, commentSpecs(aclPolicy.comment())...)
	return specs
}

func dstPortSpecs(portRange Ports) []string {
	if portRange.Port == 0 && portRange.EndPort == 0 {
		return []string{}
	}
	return []string{util.IptablesDstPortFlag, portRange.toIPTablesString()}
}

func matchSetSpecsForNetworkPolicy(networkPolicy *NPMNetworkPolicy, matchType MatchType) []string {
	specs := make([]string, 0, maxLengthForMatchSetSpecs*len(networkPolicy.PodSelectorList))
	matchString := matchType.toIPTablesString()
	for _, setInfo := range networkPolicy.PodSelectorList {
		// TODO consolidate this code with that in matchSetSpecsFromSetInfo
		specs = append(specs, util.IptablesModuleFlag, util.IptablesSetModuleFlag)
		if !setInfo.Included {
			specs = append(specs, util.IptablesNotFlag)
		}
		hashedSetName := setInfo.IPSet.GetHashedName()
		specs = append(specs, util.IptablesMatchSetFlag, hashedSetName, matchString)
	}
	return specs
}

func matchSetSpecsFromSetInfo(setInfoList []SetInfo) []string {
	specs := make([]string, 0, maxLengthForMatchSetSpecs*len(setInfoList))
	for _, setInfo := range setInfoList {
		matchString := setInfo.MatchType.toIPTablesString()
		specs = append(specs, util.IptablesModuleFlag, util.IptablesSetModuleFlag)
		if !setInfo.Included {
			specs = append(specs, util.IptablesNotFlag)
		}
		hashedSetName := setInfo.IPSet.GetHashedName()
		specs = append(specs, util.IptablesMatchSetFlag, hashedSetName, matchString)
	}
	return specs
}

func setMarkSpecs(mark string) []string {
	return []string{
		util.IptablesJumpFlag,
		util.IptablesMark,
		util.IptablesSetMarkFlag,
		mark,
	}
}

func commentSpecs(comment string) []string {
	return []string{
		util.IptablesModuleFlag,
		util.IptablesCommentModuleFlag,
		util.IptablesCommentFlag,
		comment,
	}
}

func insertSpecs(chainName string, index int, specs []string) []string {
	indexString := fmt.Sprint(index)
	insertSpecs := []string{util.IptablesInsertionFlag, chainName, indexString}
	return append(insertSpecs, specs...)
}

func joinWithDash(prefix, item string) string {
	return fmt.Sprintf("%s-%s", prefix, item)
}
