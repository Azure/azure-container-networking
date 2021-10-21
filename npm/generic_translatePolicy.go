// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"

	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

/*
TODO (jungukcho)
1. Make functions to remove redundancy
2. Add 	PodSelectorDstList []SetInfo and PodSelectorSrcList []SetInfo in NPMNetworkPolicy to reduce computation
3. Add NameSapce in NPMNetworkPolicy
*/
type translator struct{}

// Below codes will be moved to policy.go
const (
	policyIDPrefix = "azure-acl-"
)

// TODO(jungukcho) : check input for util.Hash() function.
func createUniquePolicyID(policyID string) string {
	// Anymore information for hash?
	// From Vamsi's suggestion - PolicyID: fmt.Sprintf("azure-acl-%s", hash(npmNetpol.Name+ comment)),
	// What is comment?
	return fmt.Sprintf("%s%s", policyIDPrefix, util.Hash(policyID))
}

func createACLPolicy(policyID string, target policies.Verdict, direction policies.Direction) *policies.ACLPolicy {
	acl := &policies.ACLPolicy{
		PolicyID:  policyID,
		Target:    target,
		Direction: direction,
	}
	return acl
}

func (t *translator) portRule(portRule *networkingv1.NetworkPolicyPort) (policies.Ports, string) {
	portRuleInfo := policies.Ports{}
	protocol := "TCP"
	if portRule.Protocol != nil {
		protocol = string(*portRule.Protocol)
	}

	if portRule.Port == nil {
		return portRuleInfo, protocol
	}

	// TODO(jungukcho): IntValue returns just int. Will check this..
	portRuleInfo.Port = int32(portRule.Port.IntValue())
	if portRule.EndPort != nil {
		portRuleInfo.EndPort = *portRule.EndPort
	}

	return portRuleInfo, protocol
}

func (t *translator) getPortType(portRule networkingv1.NetworkPolicyPort) string {
	if portRule.Port == nil || portRule.Port.IntValue() != 0 {
		return "validport"
	} else if portRule.Port.IntValue() == 0 && portRule.Port.String() != "" {
		return "namedport"
	}
	return "invalid"
}

// TODO(jungukcho): How to check nil value
func (t *translator) namedPortRuleInfo(portRule *networkingv1.NetworkPolicyPort) (*ipsets.TranslatedIPSet, string) {
	if portRule == nil {
		return nil, ""
	}

	protocol := "TCP"
	if portRule.Protocol != nil {
		protocol = string(*portRule.Protocol)
	}

	if portRule.Port == nil {
		return nil, protocol
	}

	namedPortIPSet := ipsets.NewTranslatedIPSet(util.NamedPortIPSetPrefix+portRule.Port.String(), ipsets.NamedPorts, []string{})
	return namedPortIPSet, protocol
}

func (t *translator) namedPortRule(portRule *networkingv1.NetworkPolicyPort) (*ipsets.TranslatedIPSet, policies.SetInfo, string) {
	if portRule == nil {
		return nil, policies.SetInfo{}, ""
	}

	namedPortIPSet, protocol := t.namedPortRuleInfo(portRule)
	setInfo := policies.NewSetInfo(util.NamedPortIPSetPrefix+portRule.Port.String(), ipsets.NamedPorts, false, policies.DstDstMatch)
	return namedPortIPSet, setInfo, protocol
}

func createCidrSetName(policyName, ns, direction string, ipBlockSetIndex int) string {
	setNameFormat := "%s-in-ns-%s-%d%s"
	return fmt.Sprintf(setNameFormat, policyName, ns, ipBlockSetIndex, direction)
}

func (t *translator) IPBlockIPSet(policyName, ns, direction string, ipBlockSetIndex int, IPBlockRule *networkingv1.IPBlock) *ipsets.TranslatedIPSet {
	if IPBlockRule == nil || len(IPBlockRule.CIDR) <= 0 {
		return nil
	}

	// TODO: use it in networkPolicyController as well and use const for "out" and int
	ipBlockIPSetName := createCidrSetName(policyName, ns, direction, ipBlockSetIndex)
	ipBlockIPSet := ipsets.NewTranslatedIPSet(ipBlockIPSetName, ipsets.CIDRBlocks, []string{})

	// TODO(junguk): "0.0.0.0/0"
	//ipBlockInfo := []string{IPBlockRule.CIDR}
	// TODO(jungukcho) Just use slices
	// TODO(junguk): Do I need to create return slices or just *ipsets.TranslatedIPSet enough?
	ipBlockIPSet.Members = append(ipBlockIPSet.Members, IPBlockRule.CIDR)
	if len(IPBlockRule.Except) > 0 {
		for _, except := range IPBlockRule.Except {
			ipBlockIPSet.Members = append(ipBlockIPSet.Members, except+util.IpsetNomatch)
		}
	}
	return ipBlockIPSet
}

func (t *translator) IPBlockRule(policyName, ns, direction string, ipBlockSetIndex int, IPBlockRule *networkingv1.IPBlock) (*ipsets.TranslatedIPSet, policies.SetInfo) {
	if IPBlockRule == nil || len(IPBlockRule.CIDR) <= 0 {
		return nil, policies.SetInfo{}
	}

	ipBlockIPSet := t.IPBlockIPSet(policyName, ns, direction, ipBlockSetIndex, IPBlockRule)
	setInfo := policies.NewSetInfo(ipBlockIPSet.Metadata.Name, ipsets.CIDRBlocks, false, policies.SrcMatch)
	return ipBlockIPSet, setInfo
}

// createPodSelectorRule return srcList for ACL by using ops and labelsForSpec
// TODO(jungukcho) change variable names and make struct for ops, nsSelectorInfo and matchType
func (t *translator) createPodSelectorRule(ops []string, ipSetForAcl []string) []policies.SetInfo {
	setInfos := []policies.SetInfo{}
	for i := 0; i < len(ipSetForAcl); i++ {
		included := ops[i] == ""
		// (TODO): need to clarify all types for Pods - ipsets.KeyValueLabelOfPod
		setInfo := policies.NewSetInfo(ipSetForAcl[i], ipsets.KeyValueLabelOfPod, included, policies.SrcMatch)
		setInfos = append(setInfos, setInfo)
	}
	return setInfos
}

func (t *translator) createPodSelectorIPSets(ipSetForSingleVal []string, ipSetNameForMultiVal map[string][]string) []*ipsets.TranslatedIPSet {
	podSelectorIPSets := []*ipsets.TranslatedIPSet{}
	for _, hashSetName := range ipSetForSingleVal {
		ipset := ipsets.NewTranslatedIPSet(hashSetName, ipsets.KeyLabelOfPod, []string{})
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	for listSetName, hashIPSetList := range ipSetNameForMultiVal {
		ipset := ipsets.NewTranslatedIPSet(listSetName, ipsets.NestedLabelOfPod, hashIPSetList)
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	return podSelectorIPSets
}

func (t *translator) targetPodSelectorInfo(ns string, selector *metav1.LabelSelector) ([]string, []string, []string, map[string][]string) {
	singleValueLabelsWithOps, multiValuesLabelsWithOps := parseSelector(selector)
	ops, ipSetForSingleVal := GetOperatorsAndLabels(singleValueLabelsWithOps)

	ipSetForAcl := make([]string, len(ipSetForSingleVal))
	copy(ipSetForAcl, ipSetForSingleVal)

	listSetMembers := []string{}
	ipSetNameForMultiVal := make(map[string][]string)

	for multiValueLabelKeyWithOps, multiValueLabelList := range multiValuesLabelsWithOps {
		op, multiValueLabelKey := GetOperatorAndLabel(multiValueLabelKeyWithOps)
		ipSetNameForMultiValueLabel := getSetNameForMultiValueSelector(multiValueLabelKey, multiValueLabelList)
		ops = append(ops, op)
		ipSetForAcl = append(ipSetForAcl, ipSetNameForMultiValueLabel)

		for _, labelValue := range multiValueLabelList {
			ipsetName := util.GetIpSetFromLabelKV(multiValueLabelKey, labelValue)
			listSetMembers = append(listSetMembers, ipsetName)
			ipSetNameForMultiVal[ipSetNameForMultiValueLabel] = append(ipSetNameForMultiVal[ipSetNameForMultiValueLabel], ipsetName)
		}
	}
	ipSetForSingleVal = append(ipSetForSingleVal, listSetMembers...)
	return ops, ipSetForAcl, ipSetForSingleVal, ipSetNameForMultiVal
}

// be consistent to use "namespace" or "ns"
func (t *translator) allPodsSelectorInNs(ns string, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	// TODO(jungukcho): important this is common component - double-check whether it has duplicated one or not
	ipset := ipsets.NewTranslatedIPSet(ns, ipsets.KeyLabelOfNamespace, []string{})
	podSelectorIPSets := []*ipsets.TranslatedIPSet{ipset}

	setInfo := policies.NewSetInfo(ns, ipsets.KeyLabelOfNamespace, false, matchType)
	dstList := []policies.SetInfo{setInfo}
	return podSelectorIPSets, dstList

}
func (t *translator) targetPodSelector(ns string, selector *metav1.LabelSelector, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	/* ex)
	ipSetForAcl :      [k0:v0 k2 k1:v10 k1:v11 k1:v10:v11]
	         op :      [                      ]
	singleValueLabels : [k0:v0 k2 k1:v10 k1:v11]
	multiValuesLabels : map[k1:v10:v11:[k1:v10 k1:v11]]
	// (TODO): some data in singleValueLabels and multiValuesLabels are duplicated
	*/

	ops, ipSetForAcl, ipSetForSingleVal, ipSetNameForMultiVal := t.targetPodSelectorInfo(ns, selector)
	// select all pods in a namespace
	if len(ops) == 1 && len(ipSetForSingleVal) == 1 && ops[0] == "" && ipSetForSingleVal[0] == "" {
		podSelectorIPSets, dstList := t.allPodsSelectorInNs(ns, matchType)
		return podSelectorIPSets, dstList
	}

	podSelectorIPSets := t.createPodSelectorIPSets(ipSetForSingleVal, ipSetNameForMultiVal)
	setInfos := t.createPodSelectorRule(ops, ipSetForAcl)

	return podSelectorIPSets, setInfos
}

// TODO(jungukcho) change variable names and make struct for ops, nsSelectorInfo and matchType
// TODO (NOW) - DIFFERENT from createPodSelectorRule?
func (t *translator) nameSpaceSelectorRule(ops []string, nsSelectorInfo []string, matchType policies.MatchType) []policies.SetInfo {
	srcList := []policies.SetInfo{}
	for i := 0; i < len(nsSelectorInfo); i++ {
		included := ops[i] == ""
		setInfo := policies.NewSetInfo(nsSelectorInfo[i], ipsets.KeyValueLabelOfNamespace, included, matchType)
		srcList = append(srcList, setInfo)
	}
	return srcList
}

// // TODO check this func references and change the label and op logic
// // craftPartialIptablesCommentFromSelector :- ns must be "" for namespace selectors
func (t *translator) nameSpaceSelectorIPSets(singleValueLabels []string) []*ipsets.TranslatedIPSet {
	nsSelectorIPSets := []*ipsets.TranslatedIPSet{}

	for _, hashSet := range singleValueLabels {
		translatedIPSet := ipsets.NewTranslatedIPSet(hashSet, ipsets.KeyValueLabelOfNamespace, []string{})
		nsSelectorIPSets = append(nsSelectorIPSets, translatedIPSet)
	}

	return nsSelectorIPSets
}

func (t *translator) nameSpaceSelectorInfo(selector *metav1.LabelSelector) ([]string, []string) {
	// parse the sector into labels and maps of multiVal match Exprs
	labelsWithOps, _ := parseSelector(selector)
	ops, singleValueLabels := GetOperatorsAndLabels(labelsWithOps)
	return ops, singleValueLabels
}

func (t *translator) allNameSpaceRule(matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	translatedIPSet := ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.KeyValueLabelOfNamespace, []string{})
	nsSelectorIPSets := []*ipsets.TranslatedIPSet{translatedIPSet}

	setInfo := policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.KeyValueLabelOfNamespace, false, matchType)
	srcList := []policies.SetInfo{setInfo}
	return nsSelectorIPSets, srcList
}

func (t *translator) nameSpaceSelector(selector *metav1.LabelSelector, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	ops, singleValueLabels := t.nameSpaceSelectorInfo(selector)

	if len(ops) == 1 && len(singleValueLabels) == 1 && ops[0] == "" && singleValueLabels[0] == "" {
		nsSelectorIPSets, srcList := t.allNameSpaceRule(matchType)
		return nsSelectorIPSets, srcList
	}

	nsSelectorIPSets := t.nameSpaceSelectorIPSets(singleValueLabels)
	srcList := t.nameSpaceSelectorRule(ops, singleValueLabels, matchType)
	return nsSelectorIPSets, srcList

}

// TODO(jungukcho): get parameter for MatchType - direction?
func (t *translator) allowAllTraffic() (*ipsets.TranslatedIPSet, policies.SetInfo) {
	allowAllIPSets := ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace, []string{})
	setInfo := policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace, false, policies.SrcMatch)
	return allowAllIPSets, setInfo
}

func (t *translator) defaultDropACL(npmNetpol *policies.NPMNetworkPolicy, direction policies.Direction) {
	dropACL := createACLPolicy(npmNetpol.Name, policies.Dropped, direction)
	if direction == policies.Ingress {
		dropACL.DstList = npmNetpol.PodSelectorList
	} else if direction == policies.Egress {
		dropACL.SrcList = npmNetpol.PodSelectorList
	}

	npmNetpol.ACLs = append(npmNetpol.ACLs, dropACL)
}

// ruleTypes return type of rules from networkingv1.NetworkPolicyIngressRule or networkingv1.NetworkPolicyEgressRule
func (t *translator) ruleExists(ports []networkingv1.NetworkPolicyPort, peer []networkingv1.NetworkPolicyPeer) (bool, bool, bool) {
	// TODO(jungukcho): need to clarify and summarize below flags
	allowExternal := false
	portRuleExists := ports != nil && len(ports) > 0
	peerRuleExists := false
	if peer != nil {
		if len(peer) == 0 {
			peerRuleExists = true
			allowExternal = true
		}

		for _, peerRule := range peer {
			if peerRule.PodSelector != nil ||
				peerRule.NamespaceSelector != nil ||
				peerRule.IPBlock != nil {
				peerRuleExists = true
				break
			}
		}
	} else if !portRuleExists {
		allowExternal = true
	}

	return allowExternal, portRuleExists, peerRuleExists
}

func (t *translator) translateIngress(npmNetpol *policies.NPMNetworkPolicy, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyIngressRule) error {
	// TODO(jungukcho) : Double-check addedCidrEntry.
	var addedCidrEntry bool // all cidr entry will be added in one set per from/to rule
	npmNetpol.PodSelectorIPSets, npmNetpol.PodSelectorList = t.targetPodSelector(npmNetpol.NameSpace, &targetSelector, policies.DstMatch)

	for i, rule := range rules {
		allowExternal, portRuleExists, fromRuleExists := t.ruleExists(rule.Ports, rule.From)

		// TODO(jungukcho): cannot come up when this condition is met.
		if !portRuleExists && !fromRuleExists && !allowExternal {
			acl := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
			acl.DstList = npmNetpol.PodSelectorList
			ruleIPSets, setInfo := t.allowAllTraffic()
			npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, ruleIPSets)
			acl.SrcList = append(acl.SrcList, setInfo)

			npmNetpol.ACLs = append(npmNetpol.ACLs, acl)
			continue
		}

		// Only Ports rules exist
		if portRuleExists && !fromRuleExists && !allowExternal {
			for _, portRule := range rule.Ports {
				portType := getPortType(portRule)
				if portType == "invalid" {
					// TODO(jungukcho): deal with error
					//return fmt.Errorf("Invalid Port type %s", portType)
					klog.Info("Invalid NetworkPolicyPort")
					continue
				}
				portACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
				// TODO(jungukcho): repeat this. Think how to remove this repeat
				portACL.DstList = npmNetpol.PodSelectorList
				if portType == "namedport" {
					namedPortIPSet, namedPortRuleDstList, protocol := t.namedPortRule(&portRule)
					portACL.DstList = append(portACL.DstList, namedPortRuleDstList)
					portACL.Protocol = policies.Protocol(protocol)
					npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, namedPortIPSet)
				} else if portType == "validport" { // TODO (jungukcho): change validport -> numberPort
					portInfo, protocol := t.portRule(&portRule)
					portACL.DstPorts = portInfo
					portACL.Protocol = policies.Protocol(protocol)
				}
				npmNetpol.ACLs = append(npmNetpol.ACLs, portACL)
			}
			continue
		}

		// fromRuleExists
		for j, fromRule := range rule.From {
			// Handle IPBlock field of NetworkPolicyPeer
			if fromRule.IPBlock != nil {
				if len(fromRule.IPBlock.CIDR) > 0 {
					// TODO(jungukcho): check this - need UTs
					// TODO(jungukcho): need a const for "in"
					ipBlockIPSet, ipBlockSetInfo := t.IPBlockRule(npmNetpol.Name, npmNetpol.NameSpace, "in", i, fromRule.IPBlock)
					npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, ipBlockIPSet)
					if j != 0 && addedCidrEntry {
						continue
					}

					if !portRuleExists {
						fromRuleACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
						fromRuleACL.DstList = npmNetpol.PodSelectorList
						fromRuleACL.SrcList = append(fromRuleACL.SrcList, ipBlockSetInfo)
						npmNetpol.ACLs = append(npmNetpol.ACLs, fromRuleACL)
					} else {
						for _, portRule := range rule.Ports {
							ipBlockAndPortRuleACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
							ipBlockAndPortRuleACL.DstList = npmNetpol.PodSelectorList
							ipBlockAndPortRuleACL.SrcList = append(ipBlockAndPortRuleACL.SrcList, ipBlockSetInfo)

							portType := getPortType(portRule)
							if portType == "invalid" {
								// TODO(jungukcho): deal with error
								klog.Info("Invalid NetworkPolicyPort")
								continue
							}

							if portType == "namedport" {
								// (TODO): check whether we need to check nil
								namedPortIPSet, namedPortRuleDstList, protocol := t.namedPortRule(&portRule)
								ipBlockAndPortRuleACL.DstList = append(ipBlockAndPortRuleACL.DstList, namedPortRuleDstList)
								ipBlockAndPortRuleACL.Protocol = policies.Protocol(protocol)
								npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, namedPortIPSet)
							} else if portType == "validport" { // TODO (jungukcho): change validport -> numberPort
								portInfo, protocol := t.portRule(&portRule)
								ipBlockAndPortRuleACL.DstPorts = portInfo
								ipBlockAndPortRuleACL.Protocol = policies.Protocol(protocol)
							}
							npmNetpol.ACLs = append(npmNetpol.ACLs, ipBlockAndPortRuleACL)
						}
					}
					addedCidrEntry = true
				}
				continue
			}

			// TODO (necessary)?
			if fromRule.PodSelector == nil && fromRule.NamespaceSelector == nil {
				continue
			}

			// Only NameSpaceSelector case..
			if fromRule.PodSelector == nil && fromRule.NamespaceSelector != nil {
				for _, nsSelector := range FlattenNameSpaceSelector(fromRule.NamespaceSelector) {
					nsSelectorIPSets, nsSrcList := t.nameSpaceSelector(&nsSelector, policies.SrcMatch)
					npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, nsSelectorIPSets...)

					// no port rule exists
					if !portRuleExists {
						nsACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
						nsACL.DstList = npmNetpol.PodSelectorList
						nsACL.SrcList = nsSrcList
						npmNetpol.ACLs = append(npmNetpol.ACLs, nsACL)
						continue
					}

					for _, portRule := range rule.Ports {
						portType := getPortType(portRule)
						if portType == "invalid" {
							// TODO(jungukcho): deal with error
							//return fmt.Errorf("Invalid Port type %s", portType)
							klog.Info("Invalid NetworkPolicyPort")
							continue
						}
						nsAndPortACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
						nsAndPortACL.DstList = npmNetpol.PodSelectorList
						nsAndPortACL.SrcList = nsSrcList

						if portType == "namedport" {
							namedPortIPSet, namedPortRuleDstList, protocol := t.namedPortRule(&portRule)
							nsAndPortACL.DstList = append(nsAndPortACL.DstList, namedPortRuleDstList)
							nsAndPortACL.Protocol = policies.Protocol(protocol)
							npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, namedPortIPSet)
						} else if portType == "validport" { // TODO (jungukcho): change validport -> numberPort
							portInfo, protocol := t.portRule(&portRule)
							nsAndPortACL.DstPorts = portInfo
							nsAndPortACL.Protocol = policies.Protocol(protocol)
						}
						npmNetpol.ACLs = append(npmNetpol.ACLs, nsAndPortACL)
					}
				}
				continue
			}

			// only PodSelector
			if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
				// TODO check old code if we need any ns- prefix for pod selectors
				podSelectorIPSets, podSelectorSrcList := t.targetPodSelector(npmNetpol.NameSpace, fromRule.PodSelector, policies.SrcMatch)
				npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, podSelectorIPSets...)
				if !portRuleExists {
					nsACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
					nsACL.DstList = npmNetpol.PodSelectorList
					nsACL.SrcList = podSelectorSrcList
					npmNetpol.ACLs = append(npmNetpol.ACLs, nsACL)
					continue
				}

				for _, portRule := range rule.Ports {
					portType := getPortType(portRule)
					if portType == "invalid" {
						klog.Info("Invalid NetworkPolicyPort")
						continue
					}
					podAndPortACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
					podAndPortACL.DstList = npmNetpol.PodSelectorList
					podAndPortACL.SrcList = podSelectorSrcList

					if portType == "namedport" {
						namedPortIPSet, namedPortRuleDstList, protocol := t.namedPortRule(&portRule)
						podAndPortACL.DstList = append(podAndPortACL.DstList, namedPortRuleDstList)
						podAndPortACL.Protocol = policies.Protocol(protocol)
						npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, namedPortIPSet)
					} else if portType == "validport" { // TODO (jungukcho): change validport -> numberPort
						portInfo, protocol := t.portRule(&portRule)
						podAndPortACL.DstPorts = portInfo
						podAndPortACL.Protocol = policies.Protocol(protocol)
					}
					npmNetpol.ACLs = append(npmNetpol.ACLs, podAndPortACL)
				}
				continue
			}

			// TODO(jungukcho): What?
			// fromRule has both namespaceSelector and podSelector set.
			// We should match the selected pods in the selected namespaces.
			// This allows traffic from podSelector intersects namespaceSelector
			// This is only supported in kubernetes version >= 1.11
			if !util.IsNewNwPolicyVerFlag {
				continue
			}

			// fromRule has both namespaceSelector and podSelector

			podSelectorIPSets, podSelectorSrcList := t.targetPodSelector(npmNetpol.NameSpace, fromRule.PodSelector, policies.SrcMatch)
			npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, podSelectorIPSets...)

			for _, nsSelector := range FlattenNameSpaceSelector(fromRule.NamespaceSelector) {
				nsSelectorIPSets, nsSrcList := t.nameSpaceSelector(&nsSelector, policies.SrcMatch)
				npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, nsSelectorIPSets...)
				nsSrcList = append(nsSrcList, podSelectorSrcList...)

				if !portRuleExists {
					nsACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
					nsACL.DstList = npmNetpol.PodSelectorList
					nsACL.SrcList = nsSrcList
					npmNetpol.ACLs = append(npmNetpol.ACLs, nsACL)
					continue
				}

				for _, portRule := range rule.Ports {
					portType := getPortType(portRule)
					if portType == "invalid" {
						klog.Info("Invalid NetworkPolicyPort")
						continue
					}

					aclWithAllFields := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
					aclWithAllFields.DstList = npmNetpol.PodSelectorList
					aclWithAllFields.SrcList = nsSrcList

					if portType == "namedport" {
						namedPortIPSet, namedPortRuleDstList, protocol := t.namedPortRule(&portRule)
						aclWithAllFields.DstList = append(aclWithAllFields.DstList, namedPortRuleDstList)
						aclWithAllFields.Protocol = policies.Protocol(protocol)
						npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, namedPortIPSet)
					} else if portType == "validport" { // TODO (jungukcho): change validport -> numberPort
						portInfo, protocol := t.portRule(&portRule)
						aclWithAllFields.DstPorts = portInfo
						aclWithAllFields.Protocol = policies.Protocol(protocol)
					}
					npmNetpol.ACLs = append(npmNetpol.ACLs, aclWithAllFields)
				}
			}
		}

		// TODO(jungukcho): move this code in entry point of this function?
		if allowExternal {
			allowExternalACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
			allowExternalACL.DstList = npmNetpol.PodSelectorList
			npmNetpol.ACLs = append(npmNetpol.ACLs, allowExternalACL)
		}
	}

	klog.Info("finished parsing ingress rule")
	return nil
}

func (t *translator) existIngress(npObj *networkingv1.NetworkPolicy) bool {
	hasIngress := true
	if npObj.Spec.Ingress != nil &&
		len(npObj.Spec.Ingress) == 1 &&
		len(npObj.Spec.Ingress[0].Ports) == 0 &&
		len(npObj.Spec.Ingress[0].From) == 0 {
		hasIngress = false
	}

	return hasIngress

}

func (t *translator) translatePolicy(npObj *networkingv1.NetworkPolicy) (*policies.NPMNetworkPolicy, error) {
	npmNetpol := &policies.NPMNetworkPolicy{
		Name:      npObj.ObjectMeta.Name,
		NameSpace: npObj.ObjectMeta.Namespace,
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		if err := t.translateIngress(npmNetpol, npObj.Spec.PodSelector, npObj.Spec.Ingress); err != nil {
			klog.Info("Cannot translate ingress rules")
			return nil, fmt.Errorf("Cannot translate ingress rules")
		}
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			if err := t.translateIngress(npmNetpol, npObj.Spec.PodSelector, npObj.Spec.Ingress); err != nil {
				klog.Info("Cannot translate ingress rules")
				return nil, fmt.Errorf("Cannot translate ingress rules")
			}
		}
	}

	if hasIngress := t.existIngress(npObj); hasIngress {
		t.defaultDropACL(npmNetpol, policies.Ingress)
	}

	return npmNetpol, nil
}
