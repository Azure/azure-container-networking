package translation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

/*
TODO
1. namespace is default in label in K8s. Need to check whether I missed something.
- Targeting a Namespace by its name
(https://kubernetes.io/docs/concepts/services-networking/network-policies/#targeting-a-namespace-by-its-name)
2. Check possible error - first check see how K8s guarantees correctness of the submitted network policy
*/

var (
	errIngressTranslation = errors.New("failed to translate ingress rules")
	errUnknownPortType    = errors.New("unknown port Type")
)

type netpolPortType string

const (
	numericPortType      netpolPortType = "validport"
	namedPortType        netpolPortType = "namedport"
	nonInversion         bool           = true
	ipBlocksetNameFormat                = "%s-in-ns-%s-%d%s"
	onlyKeyLabel                        = 1
	keyValueLabel                       = 2
	keyNestedLabel                      = 3
)

func portType(portRule networkingv1.NetworkPolicyPort) (netpolPortType, error) {
	if portRule.Port == nil || portRule.Port.IntValue() != 0 {
		return numericPortType, nil
	} else if portRule.Port.IntValue() == 0 && portRule.Port.String() != "" {
		return namedPortType, nil
	}
	// TODO (jungukcho): check whether this can be possible or not.
	return "", errUnknownPortType
}

func numericPortRule(portRule *networkingv1.NetworkPolicyPort) (policies.Ports, string) {
	portRuleInfo := policies.Ports{}
	protocol := "TCP"
	if portRule.Protocol != nil {
		protocol = string(*portRule.Protocol)
	}

	if portRule.Port == nil {
		return portRuleInfo, protocol
	}

	portRuleInfo.Port = int32(portRule.Port.IntValue())
	if portRule.EndPort != nil {
		portRuleInfo.EndPort = *portRule.EndPort
	}

	return portRuleInfo, protocol
}

func namedPortRuleInfo(portRule *networkingv1.NetworkPolicyPort) (*ipsets.TranslatedIPSet, string) {
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

func namedPortRule(portRule *networkingv1.NetworkPolicyPort) (*ipsets.TranslatedIPSet, policies.SetInfo, string) {
	if portRule == nil {
		return nil, policies.SetInfo{}, ""
	}

	namedPortIPSet, protocol := namedPortRuleInfo(portRule)
	setInfo := policies.NewSetInfo(util.NamedPortIPSetPrefix+portRule.Port.String(), ipsets.NamedPorts, nonInversion, policies.DstDstMatch)
	return namedPortIPSet, setInfo, protocol
}

func ipBlockSetName(policyName, ns string, direction policies.Direction, ipBlockSetIndex int) string {
	return fmt.Sprintf(ipBlocksetNameFormat, policyName, ns, ipBlockSetIndex, direction)
}

func ipBlockIPSet(policyName, ns string, direction policies.Direction, ipBlockSetIndex int, ipBlockRule *networkingv1.IPBlock) *ipsets.TranslatedIPSet {
	if ipBlockRule == nil || ipBlockRule.CIDR == "" {
		return nil
	}

	ipBlockIPSetName := ipBlockSetName(policyName, ns, direction, ipBlockSetIndex)
	members := make([]string, len(ipBlockRule.Except)+1) // except + cidr
	cidrIndex := 0
	members[cidrIndex] = ipBlockRule.CIDR
	for i := 0; i < len(ipBlockRule.Except); i++ {
		members[i+1] = ipBlockRule.Except[i] + util.IpsetNomatch
	}

	ipBlockIPSet := ipsets.NewTranslatedIPSet(ipBlockIPSetName, ipsets.CIDRBlocks, members)
	return ipBlockIPSet
}

func ipBlockRule(policyName, ns string, direction policies.Direction, ipBlockSetIndex int, ipBlockRule *networkingv1.IPBlock) (*ipsets.TranslatedIPSet, policies.SetInfo) {
	if ipBlockRule == nil || ipBlockRule.CIDR == "" {
		return nil, policies.SetInfo{}
	}

	ipBlockIPSet := ipBlockIPSet(policyName, ns, direction, ipBlockSetIndex, ipBlockRule)
	setInfo := policies.NewSetInfo(ipBlockIPSet.Metadata.Name, ipsets.CIDRBlocks, nonInversion, policies.SrcMatch)
	return ipBlockIPSet, setInfo
}

func podLabelType(label string) ipsets.SetType {
	// TODO(jungukcho): this is unnecessary function which has extra computation
	// will be removed after optimizing parseSelector function
	labels := strings.Split(label, ":")
	if len(labels) == onlyKeyLabel {
		return ipsets.KeyLabelOfPod
	} else if len(labels) == keyValueLabel {
		return ipsets.KeyValueLabelOfPod
	} else if len(labels) >= keyNestedLabel {
		return ipsets.NestedLabelOfPod
	}

	// (TODO): check whether this is possible
	return ipsets.UnknownType
}

// podSelectorRule return srcList for ACL by using ops and labelsForSpec
func podSelectorRule(matchType policies.MatchType, ops, ipSetForACL []string) []policies.SetInfo {
	podSelectorList := []policies.SetInfo{}
	for i := 0; i < len(ipSetForACL); i++ {
		included := ops[i] == ""
		labelType := podLabelType(ipSetForACL[i])
		setInfo := policies.NewSetInfo(ipSetForACL[i], labelType, included, matchType)
		podSelectorList = append(podSelectorList, setInfo)
	}
	return podSelectorList
}

func podSelectorIPSets(ipSetForSingleVal []string, ipSetNameForMultiVal map[string][]string) []*ipsets.TranslatedIPSet {
	podSelectorIPSets := []*ipsets.TranslatedIPSet{}
	for _, hashSetName := range ipSetForSingleVal {
		labelType := podLabelType(hashSetName)
		ipset := ipsets.NewTranslatedIPSet(hashSetName, labelType, []string{})
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	for listSetName, hashIPSetList := range ipSetNameForMultiVal {
		ipset := ipsets.NewTranslatedIPSet(listSetName, ipsets.NestedLabelOfPod, hashIPSetList)
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	return podSelectorIPSets
}

func targetPodSelectorInfo(selector *metav1.LabelSelector) ([]string, []string, []string, map[string][]string) {
	// TODO(jungukcho) : need to revise parseSelector function to reduce computations and enhance readability
	// 1. use better variables to indicate included instead of "".
	// 2. Classify type of set in parseSelector to avoid multiple computations
	singleValueLabelsWithOps, multiValuesLabelsWithOps := parseSelector(selector)
	ops, ipSetForSingleVal := GetOperatorsAndLabels(singleValueLabelsWithOps)

	ipSetForACL := make([]string, len(ipSetForSingleVal))
	copy(ipSetForACL, ipSetForSingleVal)

	listSetMembers := []string{}
	ipSetNameForMultiVal := make(map[string][]string)

	for multiValueLabelKeyWithOps, multiValueLabelList := range multiValuesLabelsWithOps {
		op, multiValueLabelKey := GetOperatorAndLabel(multiValueLabelKeyWithOps)
		ops = append(ops, op)

		ipSetNameForMultiValueLabel := getSetNameForMultiValueSelector(multiValueLabelKey, multiValueLabelList)
		ipSetForACL = append(ipSetForACL, ipSetNameForMultiValueLabel)

		for _, labelValue := range multiValueLabelList {
			ipsetName := util.GetIpSetFromLabelKV(multiValueLabelKey, labelValue)
			listSetMembers = append(listSetMembers, ipsetName)
			ipSetNameForMultiVal[ipSetNameForMultiValueLabel] = append(ipSetNameForMultiVal[ipSetNameForMultiValueLabel], ipsetName)
		}
	}
	ipSetForSingleVal = append(ipSetForSingleVal, listSetMembers...)
	return ops, ipSetForACL, ipSetForSingleVal, ipSetNameForMultiVal
}

func allPodsSelectorInNs(ns string, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	// TODO(jungukcho): important this is common component - double-check whether it has duplicated one or not
	ipset := ipsets.NewTranslatedIPSet(ns, ipsets.Namespace, []string{})
	podSelectorIPSets := []*ipsets.TranslatedIPSet{ipset}

	setInfo := policies.NewSetInfo(ns, ipsets.Namespace, nonInversion, matchType)
	podSelectorList := []policies.SetInfo{setInfo}
	return podSelectorIPSets, podSelectorList
}

func targetPodSelector(ns string, matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	// (TODO): some data in singleValueLabels and multiValuesLabels are duplicated
	ops, ipSetForACL, ipSetForSingleVal, ipSetNameForMultiVal := targetPodSelectorInfo(selector)
	// select all pods in a namespace
	if len(ops) == 1 && len(ipSetForSingleVal) == 1 && ops[0] == "" && ipSetForSingleVal[0] == "" {
		podSelectorIPSets, podSelectorList := allPodsSelectorInNs(ns, matchType)
		return podSelectorIPSets, podSelectorList
	}

	// TODO(jungukcho): may need to check ordering hashset and listset if ipSetNameForMultiVal exists.
	// refer to last test set in TestPodSelectorIPSets
	podSelectorIPSets := podSelectorIPSets(ipSetForSingleVal, ipSetNameForMultiVal)
	podSelectorList := podSelectorRule(matchType, ops, ipSetForACL)
	return podSelectorIPSets, podSelectorList
}

func nsLabelType(label string) ipsets.SetType {
	// TODO(jungukcho): this is unnecessary function which has extra computation
	// will be removed after optimizing parseSelector function
	labels := strings.Split(label, ":")
	if len(labels) == onlyKeyLabel {
		return ipsets.KeyLabelOfNamespace
	} else if len(labels) == keyValueLabel {
		return ipsets.KeyValueLabelOfNamespace
	}

	// (TODO): check whether this is possible
	return ipsets.UnknownType
}

func nameSpaceSelectorRule(matchType policies.MatchType, ops, nsSelectorInfo []string) []policies.SetInfo {
	nsSelectorList := []policies.SetInfo{}
	for i := 0; i < len(nsSelectorInfo); i++ {
		included := ops[i] == ""
		labelType := nsLabelType(nsSelectorInfo[i])
		setInfo := policies.NewSetInfo(nsSelectorInfo[i], labelType, included, matchType)
		nsSelectorList = append(nsSelectorList, setInfo)
	}
	return nsSelectorList
}

func nameSpaceSelectorIPSets(singleValueLabels []string) []*ipsets.TranslatedIPSet {
	nsSelectorIPSets := []*ipsets.TranslatedIPSet{}
	for _, listSet := range singleValueLabels {
		labelType := nsLabelType(listSet)
		translatedIPSet := ipsets.NewTranslatedIPSet(listSet, labelType, []string{})
		nsSelectorIPSets = append(nsSelectorIPSets, translatedIPSet)
	}
	return nsSelectorIPSets
}

func nameSpaceSelectorInfo(selector *metav1.LabelSelector) ([]string, []string) {
	// parse namespace label selector.
	// Ignore multiple values from parseSelector since Namespace selector does not have multiple values.
	// TODO(jungukcho): will revise parseSelector for easy understanding between podSelector and namespaceSelector
	singleValueLabelsWithOps, _ := parseSelector(selector)
	ops, singleValueLabels := GetOperatorsAndLabels(singleValueLabelsWithOps)
	return ops, singleValueLabels
}

func allNameSpaceRule(matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	translatedIPSet := ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.Namespace, []string{})
	nsSelectorIPSets := []*ipsets.TranslatedIPSet{translatedIPSet}

	setInfo := policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.Namespace, nonInversion, matchType)
	nsSelectorList := []policies.SetInfo{setInfo}
	return nsSelectorIPSets, nsSelectorList
}

func nameSpaceSelector(matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	ops, singleValueLabels := nameSpaceSelectorInfo(selector)

	if len(ops) == 1 && len(singleValueLabels) == 1 && ops[0] == "" && singleValueLabels[0] == "" {
		nsSelectorIPSets, nsSelectorList := allNameSpaceRule(matchType)
		return nsSelectorIPSets, nsSelectorList
	}

	nsSelectorIPSets := nameSpaceSelectorIPSets(singleValueLabels)
	nsSelectorList := nameSpaceSelectorRule(matchType, ops, singleValueLabels)
	return nsSelectorIPSets, nsSelectorList
}

func allowAllTraffic(matchType policies.MatchType) (*ipsets.TranslatedIPSet, policies.SetInfo) {
	allowAllIPSets := ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.Namespace, []string{})
	setInfo := policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.Namespace, nonInversion, matchType)
	return allowAllIPSets, setInfo
}

func defaultDropACL(policyNS, policyName string, direction policies.Direction) *policies.ACLPolicy {
	dropACL := policies.NewACLPolicy(policyNS, policyName, policies.Dropped, direction)
	return dropACL
}

// ruleExists returns type of rules from networkingv1.NetworkPolicyIngressRule or networkingv1.NetworkPolicyEgressRule
func ruleExists(ports []networkingv1.NetworkPolicyPort, peer []networkingv1.NetworkPolicyPeer) (bool, bool, bool) {
	// TODO(jungukcho): need to clarify and summarize below flags
	allowExternal := false
	portRuleExists := len(ports) > 0
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

func portRule(ruleIPSets []*ipsets.TranslatedIPSet, acl *policies.ACLPolicy, portRule *networkingv1.NetworkPolicyPort, portType netpolPortType) ([]*ipsets.TranslatedIPSet, *policies.ACLPolicy) {
	if portType == namedPortType {
		namedPortIPSet, namedPortRuleDstList, protocol := namedPortRule(portRule)
		acl.DstList = append(acl.DstList, namedPortRuleDstList)
		acl.Protocol = policies.Protocol(protocol)
		ruleIPSets = append(ruleIPSets, namedPortIPSet)
	} else if portType == numericPortType {
		portInfo, protocol := numericPortRule(portRule)
		acl.DstPorts = portInfo
		acl.Protocol = policies.Protocol(protocol)
	}

	return ruleIPSets, acl
}

func peerAndPortRule(npmNetPol *policies.NPMNetworkPolicy, ports []networkingv1.NetworkPolicyPort, setInfo []policies.SetInfo) {
	if len(ports) == 0 {
		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
		acl.SrcList = setInfo
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
		return
	}

	for _, port := range ports {
		portT, err := portType(port)
		if err != nil {
			// TODO(jungukcho): handle error
			klog.Infof("Invalid NetworkPolicyPort %s", err)
			continue
		}

		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
		acl.SrcList = setInfo
		npmNetPol.RuleIPSets, acl = portRule(npmNetPol.RuleIPSets, acl, &port, portT)
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
	}
}

func translateIngress(npmNetPol *policies.NPMNetworkPolicy, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyIngressRule) error {
	// TODO(jungukcho) : Double-check addedCidrEntry.
	var addedCidrEntry bool // all cidr entry will be added in one set per from/to rule
	npmNetPol.PodSelectorIPSets, npmNetPol.PodSelectorList = targetPodSelector(npmNetPol.NameSpace, policies.DstMatch, &targetSelector)

	for i, rule := range rules {
		allowExternal, portRuleExists, fromRuleExists := ruleExists(rule.Ports, rule.From)

		// #0. TODO(jungukcho): cannot come up when this condition is met.
		if !portRuleExists && !fromRuleExists && !allowExternal {
			acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
			ruleIPSets, setInfo := allowAllTraffic(policies.SrcMatch)
			npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, ruleIPSets)
			acl.SrcList = append(acl.SrcList, setInfo)
			npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
			continue
		}

		// #1. Only Ports fields exist in rule
		if portRuleExists && !fromRuleExists && !allowExternal {
			for _, port := range rule.Ports {
				portT, err := portType(port)
				if err != nil {
					klog.Infof("Invalid NetworkPolicyPort %s", err)
					continue
				}

				portACL := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
				npmNetPol.RuleIPSets, portACL = portRule(npmNetPol.RuleIPSets, portACL, &port, portT)
				npmNetPol.ACLs = append(npmNetPol.ACLs, portACL)
			}
			continue
		}

		// #2. From fields exist in rule
		for j, fromRule := range rule.From {
			// #2.1 Handle IPBlock and port if exist
			if fromRule.IPBlock != nil {
				if len(fromRule.IPBlock.CIDR) > 0 {
					// TODO(jungukcho): check this - need UTs
					// TODO(jungukcho): need a const for "in"
					ipBlockIPSet, ipBlockSetInfo := ipBlockRule(npmNetPol.Name, npmNetPol.NameSpace, policies.Ingress, i, fromRule.IPBlock)
					npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, ipBlockIPSet)
					if j != 0 && addedCidrEntry {
						continue
					}
					peerAndPortRule(npmNetPol, rule.Ports, []policies.SetInfo{ipBlockSetInfo})
					addedCidrEntry = true
				}
				// Do not check further since IPBlock filed is exclusive field.
				continue
			}

			// if there is no podSelector or namespaceSelector in fromRule, no need to check below code.
			if fromRule.PodSelector == nil && fromRule.NamespaceSelector == nil {
				continue
			}

			// #2.2 handle nameSpaceSelector and port if exist
			if fromRule.PodSelector == nil && fromRule.NamespaceSelector != nil {
				for _, nsSelector := range FlattenNameSpaceSelector(fromRule.NamespaceSelector) {
					nsSelectorIPSets, nsSrcList := nameSpaceSelector(policies.SrcMatch, &nsSelector)
					npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, nsSelectorIPSets...)
					peerAndPortRule(npmNetPol, rule.Ports, nsSrcList)
				}
				continue
			}

			// #2.3 handle podSelector and port if exist
			if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
				// TODO check old code if we need any ns- prefix for pod selectors
				podSelectorIPSets, podSelectorSrcList := targetPodSelector(npmNetPol.NameSpace, policies.SrcMatch, fromRule.PodSelector)
				npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, podSelectorIPSets...)
				peerAndPortRule(npmNetPol, rule.Ports, podSelectorSrcList)
				continue
			}

			// fromRule has both namespaceSelector and podSelector set.
			// We should match the selected pods in the selected namespaces.
			// This allows traffic from podSelector intersects namespaceSelector
			// This is only supported in kubernetes version >= 1.11
			if !util.IsNewNwPolicyVerFlag {
				continue
			}

			// #2.4 handle namespaceSelector and podSelector and port if exist
			podSelectorIPSets, podSelectorSrcList := targetPodSelector(npmNetPol.NameSpace, policies.SrcMatch, fromRule.PodSelector)
			npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, podSelectorIPSets...)

			for _, nsSelector := range FlattenNameSpaceSelector(fromRule.NamespaceSelector) {
				nsSelectorIPSets, nsSrcList := nameSpaceSelector(policies.SrcMatch, &nsSelector)
				npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, nsSelectorIPSets...)
				nsSrcList = append(nsSrcList, podSelectorSrcList...)
				peerAndPortRule(npmNetPol, rule.Ports, nsSrcList)
			}
		}

		// TODO(jungukcho): move this code in entry point of this function?
		if allowExternal {
			allowExternalACL := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
			npmNetPol.ACLs = append(npmNetPol.ACLs, allowExternalACL)
		}
	}

	klog.Info("finished parsing ingress rule")
	return nil
}

func existIngress(npObj *networkingv1.NetworkPolicy) bool {
	hasIngress := true
	if npObj.Spec.Ingress != nil &&
		len(npObj.Spec.Ingress) == 1 &&
		len(npObj.Spec.Ingress[0].Ports) == 0 &&
		len(npObj.Spec.Ingress[0].From) == 0 {
		hasIngress = false
	}
	return hasIngress
}

func translatePolicy(npObj *networkingv1.NetworkPolicy) (*policies.NPMNetworkPolicy, error) {
	npmNetPol := &policies.NPMNetworkPolicy{
		Name:      npObj.ObjectMeta.Name,
		NameSpace: npObj.ObjectMeta.Namespace,
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		if err := translateIngress(npmNetPol, npObj.Spec.PodSelector, npObj.Spec.Ingress); err != nil {
			klog.Infof("Cannot translate ingress rules due to %s", err)
			return nil, errIngressTranslation
		}
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			if err := translateIngress(npmNetPol, npObj.Spec.PodSelector, npObj.Spec.Ingress); err != nil {
				klog.Infof("Cannot translate ingress rules due to %s", err)
				return nil, errIngressTranslation
			}
		}
	}

	if hasIngress := existIngress(npObj); hasIngress {
		dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Ingress)
		npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
	}

	return npmNetPol, nil
}
