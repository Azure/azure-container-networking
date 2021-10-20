// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package npm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/iptm"
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

// getPortRange returns a specific port (only portRule.Port exist) or port ranges (when EndPort exists as well) based on portRule
func (t *translator) getPortRange(portRule networkingv1.NetworkPolicyPort) (string, bool) {
	if portRule.Port == nil {
		return "", false
	}

	portRange := portRule.Port.String()
	if portRule.EndPort != nil {
		portRange = fmt.Sprintf("%s:%d", portRange, *portRule.EndPort)
	}
	return portRange, true
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

	namedPortIPSet := &ipsets.TranslatedIPSet{
		Metadata: &ipsets.IPSetMetadata{
			Name: util.NamedPortIPSetPrefix + portRule.Port.String(),
			Type: ipsets.NamedPorts,
		},
	}
	return namedPortIPSet, protocol
}

func (t *translator) namedPortRule(portRule *networkingv1.NetworkPolicyPort) (*ipsets.TranslatedIPSet, policies.SetInfo, string) {
	if portRule == nil {
		return nil, policies.SetInfo{}, ""
	}

	namedPortIPSet, protocol := t.namedPortRuleInfo(portRule)
	setInfo := policies.SetInfo{
		IPSet: &ipsets.IPSetMetadata{
			Name: util.NamedPortIPSetPrefix + portRule.Port.String(),
			Type: ipsets.NamedPorts,
		},
		MatchType: policies.DstDstMatch,
	}
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
	ipBlockIPSet := &ipsets.TranslatedIPSet{
		Metadata: &ipsets.IPSetMetadata{
			Name: ipBlockIPSetName,
			Type: ipsets.CIDRBlocks,
		},
		Members: []string{},
	}

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
	setInfo := policies.SetInfo{
		IPSet: &ipsets.IPSetMetadata{
			Name: ipBlockIPSet.Metadata.Name,
			Type: ipsets.CIDRBlocks, // need to change name
		},
		Included:  false,
		MatchType: policies.SrcMatch,
	}

	return ipBlockIPSet, setInfo
}

// createPodSelectorRule return srcList for ACL by using ops and labelsForSpec
func (t *translator) createPodSelectorRule(ops []string, labelsForSpec []string, matchType policies.MatchType) []policies.SetInfo {
	srcList := []policies.SetInfo{}
	for i := 0; i < len(labelsForSpec); i++ {
		lableForSpec := labelsForSpec[i]
		podIPSet := ipsets.TranslatedIPSet{
			Metadata: &ipsets.IPSetMetadata{
				Name: lableForSpec,
				Type: ipsets.KeyValueLabelOfPod,
			},
			Members: []string{},
		}

		setInfo := policies.SetInfo{
			// TODO(jungukcho): is using podIPSet.Metadata safe?
			IPSet:     podIPSet.Metadata,
			Included:  false,
			MatchType: matchType,
		}

		if ops[i] == "" {
			setInfo.Included = true
		}
		srcList = append(srcList, setInfo)
	}
	return srcList
}

func (t *translator) createPodSelectorIPSets(singleValueLabels []string, multiValuesLabels map[string][]string) []*ipsets.TranslatedIPSet {
	podSelectorIPSets := []*ipsets.TranslatedIPSet{}

	for _, hashSet := range singleValueLabels {
		ipset := &ipsets.TranslatedIPSet{
			// TODO(jungukcho): create function and do not use capitals
			Metadata: &ipsets.IPSetMetadata{
				Name: hashSet,
				Type: ipsets.KeyLabelOfPod,
			},
		}
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	for listSetName, hashSet := range multiValuesLabels {
		ipset := &ipsets.TranslatedIPSet{
			Metadata: &ipsets.IPSetMetadata{
				Name: listSetName,
				Type: ipsets.NestedLabelOfPod,
			},
			Members: hashSet,
		}
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	return podSelectorIPSets
}

// TODO(jungukcho): have better naming
func (t *translator) targetPodSelectorInfo(ns string, selector *metav1.LabelSelector) ([]string, []string, []string, map[string][]string) {
	singleValueLabelsWithOps, multiValuesLabelsWithOps := parseSelector(selector)
	ops, singleValueLabels := GetOperatorsAndLabels(singleValueLabelsWithOps)
	labelsForSpec := make([]string, len(singleValueLabels))
	copy(labelsForSpec, singleValueLabels)

	listSetMembers := []string{}
	// (TODO) []struct : key : string, value : []string - deterministic
	multiValuesLabels := make(map[string][]string)

	for multiValueLabelKeyWithOps, multiValueLabelList := range multiValuesLabelsWithOps {
		op, multiValueLabelKey := GetOperatorAndLabel(multiValueLabelKeyWithOps)
		ipSetNameForMultiValueLabel := getSetNameForMultiValueSelector(multiValueLabelKey, multiValueLabelList)
		if !util.StrExistsInSlice(singleValueLabels, ipSetNameForMultiValueLabel) {
			ops = append(ops, op)
			labelsForSpec = append(labelsForSpec, ipSetNameForMultiValueLabel)
		}
		for _, labelValue := range multiValueLabelList {
			ipsetName := util.GetIpSetFromLabelKV(multiValueLabelKey, labelValue)
			listSetMembers = append(listSetMembers, ipsetName)
			multiValuesLabels[ipSetNameForMultiValueLabel] = append(multiValuesLabels[ipSetNameForMultiValueLabel], ipsetName)
		}
	}
	singleValueLabels = append(singleValueLabels, listSetMembers...)
	return ops, labelsForSpec, singleValueLabels, multiValuesLabels
}

// be consistent to use "namespace" or "ns"
func (t *translator) allPodsSelectorInNs(ns string, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	podSelectorIPSets := []*ipsets.TranslatedIPSet{}
	ipset := &ipsets.TranslatedIPSet{
		// TODO(jungukcho): important this is common component - double-check whether it has duplicated one or not
		Metadata: &ipsets.IPSetMetadata{
			Name: ns,
			Type: ipsets.KeyLabelOfNamespace,
		},
	}
	podSelectorIPSets = append(podSelectorIPSets, ipset)

	dstList := []policies.SetInfo{}
	nsIPSet := ipsets.TranslatedIPSet{
		Metadata: &ipsets.IPSetMetadata{
			Name: ns,
			Type: ipsets.KeyLabelOfNamespace,
		},
	}
	setInfo := policies.SetInfo{
		IPSet:     nsIPSet.Metadata,
		Included:  true,
		MatchType: matchType,
	}

	dstList = append(dstList, setInfo)
	return podSelectorIPSets, dstList

}
func (t *translator) targetPodSelector(ns string, selector *metav1.LabelSelector, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	ops, labelsForSpec, singleValueLabels, multiValuesLabels := t.targetPodSelectorInfo(ns, selector)
	// select all pods in a namespace
	if len(ops) == 1 && len(singleValueLabels) == 1 && ops[0] == "" && singleValueLabels[0] == "" {
		podSelectorIPSets, dstList := t.allPodsSelectorInNs(ns, matchType)
		return podSelectorIPSets, dstList
	}

	podSelectorIPSets := t.createPodSelectorIPSets(singleValueLabels, multiValuesLabels)
	dstList := t.createPodSelectorRule(ops, labelsForSpec, matchType)

	return podSelectorIPSets, dstList
}

// TODO(jungukcho): can reuse translatedIPSet
func (t *translator) nameSpaceSelectorRule(ops []string, nsSelectorInfo []string, matchType policies.MatchType) []policies.SetInfo {
	srcList := []policies.SetInfo{}
	for i := 0; i < len(nsSelectorInfo); i++ {
		lableForSpec := nsSelectorInfo[i]
		nsIPSet := ipsets.TranslatedIPSet{
			Metadata: &ipsets.IPSetMetadata{
				Name: lableForSpec,
				Type: ipsets.KeyValueLabelOfNamespace,
			},
		}

		setInfo := policies.SetInfo{
			// TODO(jungukcho): is using podIPSet.Metadata safe?
			IPSet:     nsIPSet.Metadata,
			Included:  false,
			MatchType: matchType,
		}

		if ops[i] == "" {
			setInfo.Included = true
		}
		srcList = append(srcList, setInfo)
	}
	return srcList
}

// // TODO check this func references and change the label and op logic
// // craftPartialIptablesCommentFromSelector :- ns must be "" for namespace selectors
func (t *translator) nameSpaceSelectorIPSets(singleValueLabels []string) []*ipsets.TranslatedIPSet {
	nsSelectorIPSets := []*ipsets.TranslatedIPSet{}

	for _, hashSet := range singleValueLabels {
		ipset := &ipsets.TranslatedIPSet{
			// TODO(jungukcho): create function and do not use capitals
			Metadata: &ipsets.IPSetMetadata{
				Name: hashSet,
				Type: ipsets.KeyValueLabelOfNamespace,
			},
		}
		nsSelectorIPSets = append(nsSelectorIPSets, ipset)
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
	nsSelectorIPSets := []*ipsets.TranslatedIPSet{}
	ipset := &ipsets.TranslatedIPSet{
		// TODO(jungukcho): create function and do not use capitals
		Metadata: &ipsets.IPSetMetadata{
			Name: util.KubeAllNamespacesFlag,
			Type: ipsets.KeyValueLabelOfNamespace,
		},
	}
	nsSelectorIPSets = append(nsSelectorIPSets, ipset)

	srcList := []policies.SetInfo{}
	nsIPSet := ipsets.TranslatedIPSet{
		Metadata: &ipsets.IPSetMetadata{
			Name: util.KubeAllNamespacesFlag,
			Type: ipsets.KeyValueLabelOfNamespace,
		},
	}
	setInfo := policies.SetInfo{
		IPSet:     nsIPSet.Metadata,
		Included:  true,
		MatchType: matchType,
	}

	srcList = append(srcList, setInfo)
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
	allowAllIPSets := &ipsets.TranslatedIPSet{
		Metadata: &ipsets.IPSetMetadata{
			Name: util.KubeAllNamespacesFlag,
			Type: ipsets.KeyLabelOfNamespace,
		},
	}

	setInfo := policies.SetInfo{
		//TODO(jungukcho): this is confusion IPSet -> Metadata
		IPSet: &ipsets.IPSetMetadata{
			Name: util.KubeAllNamespacesFlag,
			Type: ipsets.KeyLabelOfNamespace,
		},
		MatchType: policies.SrcMatch,
	}
	return allowAllIPSets, setInfo
}

func (t *translator) defaultDropACL(npmNetpol *policies.NPMNetworkPolicy, hasIngress, hasEgress bool) {
	if hasIngress {
		ingressDropACL := createACLPolicy(npmNetpol.Name, policies.Dropped, policies.Ingress)
		ingressDropACL.DstList = npmNetpol.PodSelectorDstList
		npmNetpol.ACLs = append(npmNetpol.ACLs, ingressDropACL)
	}

	if hasEgress {
		egressDropACL := createACLPolicy(npmNetpol.Name, policies.Dropped, policies.Egress)
		egressDropACL.DstList = npmNetpol.PodSelectorSrcList
		npmNetpol.ACLs = append(npmNetpol.ACLs, egressDropACL)
	}
}

func (t *translator) translateIngress(npmNetpol *policies.NPMNetworkPolicy, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyIngressRule) error {
	var addedCidrEntry bool // all cidr entry will be added in one set per from/to rule
	// TODO(jungukcho): addedPortEntry is not used.. Why?
	var addedPortEntry bool // add drop entries at the end of the chain when there are non ALLOW-ALL* rules

	npmNetpol.PodSelectorIPSets, npmNetpol.PodSelectorDstList = t.targetPodSelector(npmNetpol.NameSpace, &targetSelector, policies.DstMatch)

	for i, rule := range rules {
		fmt.Printf("i: %d rule: %d\n", i, len(rule.Ports))
		// TODO(jungukcho): need to clarify and summarize below flags
		allowExternal := false
		portRuleExists := rule.Ports != nil && len(rule.Ports) > 0
		fromRuleExists := false
		addedPortEntry = addedPortEntry || portRuleExists
		if rule.From != nil {
			if len(rule.From) == 0 {
				fromRuleExists = true
				allowExternal = true
			}

			for _, fromRule := range rule.From {
				if fromRule.PodSelector != nil ||
					fromRule.NamespaceSelector != nil ||
					fromRule.IPBlock != nil {
					fromRuleExists = true
					break
				}
			}
		} else if !portRuleExists {
			allowExternal = true
		}

		// TODO(jungukcho): cannot come up when this condition is met.
		if !portRuleExists && !fromRuleExists && !allowExternal {
			acl := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
			acl.DstList = npmNetpol.PodSelectorDstList
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
				portACL.DstList = npmNetpol.PodSelectorDstList
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
						fromRuleACL.DstList = npmNetpol.PodSelectorDstList
						fromRuleACL.SrcList = append(fromRuleACL.SrcList, ipBlockSetInfo)
						npmNetpol.ACLs = append(npmNetpol.ACLs, fromRuleACL)
					} else {
						for _, portRule := range rule.Ports {
							ipBlockAndPortRuleACL := createACLPolicy(npmNetpol.Name, policies.Allowed, policies.Ingress)
							ipBlockAndPortRuleACL.DstList = npmNetpol.PodSelectorDstList
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
						nsACL.DstList = npmNetpol.PodSelectorDstList
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
						nsAndPortACL.DstList = npmNetpol.PodSelectorDstList
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
					nsACL.DstList = npmNetpol.PodSelectorDstList
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
					podAndPortACL.DstList = npmNetpol.PodSelectorDstList
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
					nsACL.DstList = npmNetpol.PodSelectorDstList
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
					aclWithAllFields.DstList = npmNetpol.PodSelectorDstList
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
			allowExternalACL.DstList = npmNetpol.PodSelectorDstList
			npmNetpol.ACLs = append(npmNetpol.ACLs, allowExternalACL)
		}
	}

	klog.Info("finished parsing ingress rule")
	return nil
}

// TODO(jungukcho): need to complete this function
func (t *translator) translatePolicy(npObj *networkingv1.NetworkPolicy) (*policies.NPMNetworkPolicy, error) {
	npNs := npObj.ObjectMeta.Namespace
	policyName := npObj.ObjectMeta.Name

	// TODO(jungukcho): craete this before calling translateIngress function
	npmNetpol := &policies.NPMNetworkPolicy{
		Name:      policyName,
		NameSpace: npNs,
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		if err := t.translateIngress(npmNetpol, npObj.Spec.PodSelector, npObj.Spec.Ingress); err != nil {
			klog.Info("Cannot translate ingress rules")
			return nil, fmt.Errorf("Cannot translate ingress rules")
		}

		// egressSets, egressNamedPorts, egressLists, egressIPCidrs, egressEntries := translateEgress(npNs, policyName, npObj.Spec.PodSelector, npObj.Spec.Egress)
		// resultSets = append(resultSets, egressSets...)
		// resultNamedPorts = append(resultNamedPorts, egressNamedPorts...)
		// for resultListKey, resultLists := range egressLists {
		// 	resultListMap[resultListKey] = append(resultListMap[resultListKey], resultLists...)
		// }
		// entries = append(entries, egressEntries...)

		var hasIngress, hasEgress bool
		if npObj.Spec.Ingress != nil &&
			len(npObj.Spec.Ingress) == 1 &&
			len(npObj.Spec.Ingress[0].Ports) == 0 &&
			len(npObj.Spec.Ingress[0].From) == 0 {
			hasIngress = false
		} else {
			hasIngress = true
		}

		t.defaultDropACL(npmNetpol, hasIngress, hasEgress)

		return npmNetpol, nil
	}

	// for _, ptype := range npObj.Spec.PolicyTypes {
	// 	if ptype == networkingv1.PolicyTypeIngress {
	// 		ingressSets, ingressNamedPorts, ingressLists, ingressIPCidrs, ingressEntries := t.translateIngress(npmNetpol, npObj.Spec.PodSelector, npObj.Spec.Ingress)
	// 		resultSets = append(resultSets, ingressSets...)
	// 		resultNamedPorts = append(resultNamedPorts, ingressNamedPorts...)
	// 		for resultListKey, resultLists := range ingressLists {
	// 			resultListMap[resultListKey] = append(resultListMap[resultListKey], resultLists...)
	// 		}
	// 		resultIngressIPCidrs = ingressIPCidrs
	// 		entries = append(entries, ingressEntries...)

	// 		if npObj.Spec.Ingress != nil &&
	// 			len(npObj.Spec.Ingress) == 1 &&
	// 			len(npObj.Spec.Ingress[0].Ports) == 0 &&
	// 			len(npObj.Spec.Ingress[0].From) == 0 {
	// 			hasIngress = false
	// 		} else {
	// 			hasIngress = true
	// 		}
	// 	}

	// 	if ptype == networkingv1.PolicyTypeEgress {
	// 		egressSets, egressNamedPorts, egressLists, egressIPCidrs, egressEntries := translateEgress(npNs, policyName, npObj.Spec.PodSelector, npObj.Spec.Egress)
	// 		resultSets = append(resultSets, egressSets...)
	// 		resultNamedPorts = append(resultNamedPorts, egressNamedPorts...)
	// 		for resultListKey, resultLists := range egressLists {
	// 			resultListMap[resultListKey] = append(resultListMap[resultListKey], resultLists...)
	// 		}
	// 		resultEgressIPCidrs = egressIPCidrs
	// 		entries = append(entries, egressEntries...)

	// 		if npObj.Spec.Egress != nil &&
	// 			len(npObj.Spec.Egress) == 1 &&
	// 			len(npObj.Spec.Egress[0].Ports) == 0 &&
	// 			len(npObj.Spec.Egress[0].To) == 0 {
	// 			hasEgress = false
	// 		} else {
	// 			hasEgress = true
	// 		}
	// 	}
	// }

	// //entries = append(entries, t.getDefaultDropEntries(npNs, npObj.Spec.PodSelector, hasIngress, hasEgress)...)
	// t.defaultDropACL(npmNetpol, hasIngress, hasEgress)
	// for resultListKey, resultLists := range resultListMap {
	// 	resultListMap[resultListKey] = util.UniqueStrSlice(resultLists)
	// }

	// return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultNamedPorts), resultListMap, resultIngressIPCidrs, resultEgressIPCidrs, entries
	// TODO(jungukcho): temporarily return value
	return nil, fmt.Errorf("Still working on this function and it is just to avoid errors ")

}

/*
	Just for testing now
*/
func (t *translator) appendSelectorLabelsToLists(lists, listLabelsWithMembers map[string][]string, isNamespaceSelector bool) {
	for parsedListName, parsedListMembers := range listLabelsWithMembers {
		if isNamespaceSelector {
			parsedListName = util.GetNSNameWithPrefix(parsedListName)
		}
		for _, member := range parsedListMembers {
			if isNamespaceSelector {
				member = util.GetNSNameWithPrefix(member)
			}
			lists[parsedListName] = append(lists[parsedListName], member)
		}
	}
}

func (t *translator) craftPartialIptEntrySpecFromPort(portRule networkingv1.NetworkPolicyPort) []string {
	partialSpec := []string{}
	if portRule.Protocol != nil {
		partialSpec = append(
			partialSpec,
			util.IptablesProtFlag,
			string(*portRule.Protocol),
		)
	}

	if portRange, exist := t.getPortRange(portRule); exist {
		partialSpec = append(
			partialSpec,
			util.IptablesDstPortFlag,
			portRange,
		)
	}
	return partialSpec
}

func (t *translator) craftPartialIptEntrySpecFromOpAndLabel(op, label, srcOrDstFlag string, isNamespaceSelector bool) []string {
	if isNamespaceSelector {
		label = "ns-" + label
	}
	partialSpec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		op,
		util.IptablesMatchSetFlag,
		util.GetHashedName(label),
		srcOrDstFlag,
	}

	return util.DropEmptyFields(partialSpec)
}

// TODO check this func references and change the label and op logic
// craftPartialIptablesCommentFromSelector :- ns must be "" for namespace selectors
func (t *translator) craftPartialIptEntrySpecFromOpsAndLabels(ns string, ops, labels []string, srcOrDstFlag string, isNamespaceSelector bool) []string {
	var spec []string

	fmt.Println("ns : ", ns)
	if ns != "" {
		spec = []string{
			util.IptablesModuleFlag,
			util.IptablesSetModuleFlag,
			util.IptablesMatchSetFlag,
			util.GetHashedName("ns-" + ns),
			srcOrDstFlag,
		}
	}

	if len(ops) == 1 && len(labels) == 1 {
		if ops[0] == "" && labels[0] == "" {
			if isNamespaceSelector {
				// This is an empty namespaceSelector,
				// selecting all namespaces.
				spec = []string{
					util.IptablesModuleFlag,
					util.IptablesSetModuleFlag,
					util.IptablesMatchSetFlag,
					util.GetHashedName(util.KubeAllNamespacesFlag),
					srcOrDstFlag,
				}
			}

			return spec
		}
	}
	for i := range ops {
		// TODO need to change this logic, create a list of lsts here and have a single match against it
		spec = append(spec, t.craftPartialIptEntrySpecFromOpAndLabel(ops[i], labels[i], srcOrDstFlag, isNamespaceSelector)...)
	}
	return spec
}

// craftPartialIptEntrySpecFromSelector :- ns must be "" for namespace selectors
// func helps in taking a labelSelector and converts it into corresponding matchSets
// to be a used in full iptable rules
//  selector *metav1.LabelSelector: is used to create matchSets
//  ns string: helps with adding a namespace name in case of empty (or all) selector
//  srcOrDstFlag string: helps with determining if the src flag is to used in matchsets or dst flag,
// depending on ingress or egress translate policy
//  isNamespaceSelector bool: helps in adding prefix for nameSpace ipsets
func (t *translator) craftPartialIptEntrySpecFromSelector(ns string, selector *metav1.LabelSelector, srcOrDstFlag string, isNamespaceSelector bool) ([]string, []string, map[string][]string) {
	// parse the sector into labels and maps of multiVal match Exprs
	labelsWithOps, nsLabelListKVs := parseSelector(selector)
	ops, labels := GetOperatorsAndLabels(labelsWithOps)

	valueLabels := []string{}
	listLabelsWithMembers := make(map[string][]string)
	labelsForSpec := labels
	// parseSelector returns a slice of processed label and a map of lists with members.
	// now we need to compute the 2nd-level ipset names from lists and its members
	// add use those 2nd level ipsets to be used to create the partial match set
	for labelKeyWithOps, labelValueList := range nsLabelListKVs {
		// look at each list and its members
		op, labelKey := GetOperatorAndLabel(labelKeyWithOps)
		// get the new 2nd level IpSet name
		labelKVIpsetName := getSetNameForMultiValueSelector(labelKey, labelValueList)
		if !util.StrExistsInSlice(labels, labelKVIpsetName) {
			// Important: make sure length andordering of ops and labelsForSpec are same
			// because craftPartialEntry loops over both of them at once and assumes
			// a given position ops is to be applied on the same position label in labelsForSpec
			ops = append(ops, op)
			// TODO doubt check if this 2nd level needs to be added to the labels when labels are added to lists
			// check if the 2nd level is already part of labels
			labelsForSpec = append(labelsForSpec, labelKVIpsetName)
		}
		for _, labelValue := range labelValueList {
			ipsetName := util.GetIpSetFromLabelKV(labelKey, labelValue)
			valueLabels = append(valueLabels, ipsetName)
			listLabelsWithMembers[labelKVIpsetName] = append(listLabelsWithMembers[labelKVIpsetName], ipsetName)
		}
	}
	iptEntrySpecs := t.craftPartialIptEntrySpecFromOpsAndLabels(ns, ops, labelsForSpec, srcOrDstFlag, isNamespaceSelector)
	// only append valueLabels to labels after creating the Ipt Spec with valueLabels
	// as 1D valueLabels are included in 2nd level labelKVIpsetName
	labels = append(labels, valueLabels...)
	return iptEntrySpecs, labels, listLabelsWithMembers
}

func (t *translator) craftPartialIptablesCommentFromPort(portRule networkingv1.NetworkPolicyPort) string {
	partialComment := ""
	if portRule.Protocol != nil {
		partialComment += string(*portRule.Protocol)
		if portRule.Port != nil {
			partialComment += "-"
		}
	}

	if portRule.Port != nil {
		partialComment += "PORT-"
		partialComment += portRule.Port.String()
	}

	return partialComment
}

// craftPartialIptablesCommentFromSelector :- ns must be "" for namespace selectors
func (t *translator) craftPartialIptablesCommentFromSelector(ns string, selector *metav1.LabelSelector, isNamespaceSelector bool) string {
	if selector == nil {
		return "none"
	}

	if len(selector.MatchExpressions) == 0 && len(selector.MatchLabels) == 0 {
		if isNamespaceSelector {
			return util.KubeAllNamespacesFlag
		}

		return "ns-" + ns
	}

	// TODO check if we are missing any crucial comment
	labelsWithOps, labelKVs := parseSelector(selector)
	ops, labelsWithoutOps := GetOperatorsAndLabels(labelsWithOps)
	for labelKeyWithOps, labelValueList := range labelKVs {
		op, labelKey := GetOperatorAndLabel(labelKeyWithOps)
		labelKVIpsetName := getSetNameForMultiValueSelector(labelKey, labelValueList)
		labelsWithoutOps = append(labelsWithoutOps, labelKVIpsetName)
		ops = append(ops, op)
	}

	var prefix, postfix string
	if isNamespaceSelector {
		prefix = "ns-"
	} else {
		if ns != "" {
			postfix = "-IN-ns-" + ns
		}
	}

	comments := []string{}
	for i := range labelsWithoutOps {
		comments = append(comments, prefix+ops[i]+labelsWithoutOps[i])
	}

	sort.Strings(comments)
	return strings.Join(comments, "-AND-") + postfix
}

func (t *translator) translatePolicyV1(npObj *networkingv1.NetworkPolicy) ([]string, []string, map[string][]string, [][]string, [][]string, []*iptm.IptEntry) {
	var (
		resultSets            []string
		resultNamedPorts      []string
		resultListMap         map[string][]string
		resultIngressIPCidrs  [][]string
		resultEgressIPCidrs   [][]string
		entries               []*iptm.IptEntry
		hasIngress, hasEgress bool
	)

	defer func() {
		log.Logf("Finished translatePolicy")
		log.Logf("sets: %v", resultSets)
		log.Logf("lists: %v", resultListMap)
		log.Logf("entries: ")
		for _, entry := range entries {
			log.Logf("entry: %+v", entry)
		}
	}()

	npNs := npObj.ObjectMeta.Namespace
	policyName := npObj.ObjectMeta.Name
	resultListMap = make(map[string][]string)

	//targetSelectorIptEntrySpec, targetSelectorLabels, listLabelsWithMembers := t.craftPartialIptEntrySpecFromSelector(npNs, &npObj.Spec.PodSelector, util.IptablesDstFlag, false)

	// Since nested ipset list:sets are not allowed. We cannot use 2nd level Ipsets
	// for NameSpaceSelectors with multiple values
	// NPM will need to duplicate rules for each value in NSSelector
	if len(npObj.Spec.PolicyTypes) == 0 {
		ingressSets, ingressNamedPorts, ingressLists, ingressIPCidrs, ingressEntries := translateIngress(npNs, policyName, npObj.Spec.PodSelector, npObj.Spec.Ingress)
		resultSets = append(resultSets, ingressSets...)
		resultNamedPorts = append(resultNamedPorts, ingressNamedPorts...)
		for resultListKey, resultLists := range ingressLists {
			resultListMap[resultListKey] = append(resultListMap[resultListKey], resultLists...)
		}
		entries = append(entries, ingressEntries...)

		egressSets, egressNamedPorts, egressLists, egressIPCidrs, egressEntries := translateEgress(npNs, policyName, npObj.Spec.PodSelector, npObj.Spec.Egress)
		resultSets = append(resultSets, egressSets...)
		resultNamedPorts = append(resultNamedPorts, egressNamedPorts...)
		for resultListKey, resultLists := range egressLists {
			resultListMap[resultListKey] = append(resultListMap[resultListKey], resultLists...)
		}
		entries = append(entries, egressEntries...)

		hasIngress = len(ingressSets) > 0
		hasEgress = len(egressSets) > 0
		entries = append(entries, getDefaultDropEntries(npNs, npObj.Spec.PodSelector, hasIngress, hasEgress)...)
		for resultListKey, resultLists := range resultListMap {
			resultListMap[resultListKey] = util.UniqueStrSlice(resultLists)
		}

		return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultNamedPorts), resultListMap, ingressIPCidrs, egressIPCidrs, entries
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			ingressSets, ingressNamedPorts, ingressLists, ingressIPCidrs, ingressEntries := translateIngress(npNs, policyName, npObj.Spec.PodSelector, npObj.Spec.Ingress)
			resultSets = append(resultSets, ingressSets...)
			resultNamedPorts = append(resultNamedPorts, ingressNamedPorts...)
			for resultListKey, resultLists := range ingressLists {
				resultListMap[resultListKey] = append(resultListMap[resultListKey], resultLists...)
			}
			resultIngressIPCidrs = ingressIPCidrs
			entries = append(entries, ingressEntries...)

			if npObj.Spec.Ingress != nil &&
				len(npObj.Spec.Ingress) == 1 &&
				len(npObj.Spec.Ingress[0].Ports) == 0 &&
				len(npObj.Spec.Ingress[0].From) == 0 {
				hasIngress = false
			} else {
				hasIngress = true
			}
		}

		if ptype == networkingv1.PolicyTypeEgress {
			egressSets, egressNamedPorts, egressLists, egressIPCidrs, egressEntries := translateEgress(npNs, policyName, npObj.Spec.PodSelector, npObj.Spec.Egress)
			resultSets = append(resultSets, egressSets...)
			resultNamedPorts = append(resultNamedPorts, egressNamedPorts...)
			for resultListKey, resultLists := range egressLists {
				resultListMap[resultListKey] = append(resultListMap[resultListKey], resultLists...)
			}
			resultEgressIPCidrs = egressIPCidrs
			entries = append(entries, egressEntries...)

			if npObj.Spec.Egress != nil &&
				len(npObj.Spec.Egress) == 1 &&
				len(npObj.Spec.Egress[0].Ports) == 0 &&
				len(npObj.Spec.Egress[0].To) == 0 {
				hasEgress = false
			} else {
				hasEgress = true
			}
		}
	}

	entries = append(entries, getDefaultDropEntries(npNs, npObj.Spec.PodSelector, hasIngress, hasEgress)...)
	for resultListKey, resultLists := range resultListMap {
		resultListMap[resultListKey] = util.UniqueStrSlice(resultLists)
	}

	return util.UniqueStrSlice(resultSets), util.UniqueStrSlice(resultNamedPorts), resultListMap, resultIngressIPCidrs, resultEgressIPCidrs, entries
}
