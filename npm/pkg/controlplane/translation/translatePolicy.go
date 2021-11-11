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
- Return error and validation
3. Need to handle 0.0.0.0/0 in IPBlock field
- Ipset doesn't allow 0.0.0.0/0 to be added. A general solution is split 0.0.0.0/1 in half which convert to
  1.0.0.0/1 and 128.0.0.0/1 in linux
*/

var errUnknownPortType = errors.New("unknown port Type")

type netpolPortType string

const (
	numericPortType      netpolPortType = "validport"
	namedPortType        netpolPortType = "namedport"
	included             bool           = true
	ipBlocksetNameFormat                = "%s-in-ns-%s-%d%s"
	onlyKeyLabel                        = 1
	keyValueLabel                       = 2
)

// portType returns type of ports (e.g., numeric port or namedPort) given NetworkPolicyPort object.
func portType(portRule networkingv1.NetworkPolicyPort) (netpolPortType, error) {
	if portRule.Port == nil || portRule.Port.IntValue() != 0 {
		return numericPortType, nil
	} else if portRule.Port.IntValue() == 0 && portRule.Port.String() != "" {
		return namedPortType, nil
	}
	// TODO (jungukcho): check whether this can be possible or not.
	return "", errUnknownPortType
}

// numericPortRule returns policies.Ports (port, endport) and protocol type
// based on NetworkPolicyPort holding numeric port information.
func numericPortRule(portRule *networkingv1.NetworkPolicyPort) (portRuleInfo policies.Ports, protocol string) {
	portRuleInfo = policies.Ports{}
	protocol = "TCP"
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

// namedPortRuleInfo returns translatedIPSet and protocol type
// based on NetworkPolicyPort holding named port information.
func namedPortRuleInfo(portRule *networkingv1.NetworkPolicyPort) (namedPortIPSet *ipsets.TranslatedIPSet, protocol string) {
	if portRule == nil {
		return nil, ""
	}

	protocol = "TCP"
	if portRule.Protocol != nil {
		protocol = string(*portRule.Protocol)
	}

	if portRule.Port == nil {
		return nil, protocol
	}

	namedPortIPSet = ipsets.NewTranslatedIPSet(util.NamedPortIPSetPrefix+portRule.Port.String(), ipsets.NamedPorts, []string{})
	return namedPortIPSet, protocol
}

func namedPortRule(portRule *networkingv1.NetworkPolicyPort) (*ipsets.TranslatedIPSet, policies.SetInfo, string) {
	if portRule == nil {
		return nil, policies.SetInfo{}, ""
	}

	namedPortIPSet, protocol := namedPortRuleInfo(portRule)
	setInfo := policies.NewSetInfo(util.NamedPortIPSetPrefix+portRule.Port.String(), ipsets.NamedPorts, included, policies.DstDstMatch)
	return namedPortIPSet, setInfo, protocol
}

func portRule(ruleIPSets []*ipsets.TranslatedIPSet, acl *policies.ACLPolicy, portRule *networkingv1.NetworkPolicyPort, portType netpolPortType) []*ipsets.TranslatedIPSet {
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

	return ruleIPSets
}

// ipBlockIPSet returns ipset name of the IPBlock.
// It is our contract to format "<policyname>-in-ns-<namespace>-<ipblock index><direction of ipblock (i.e., ingress: IN, egress: OUT>"
// as ipset name of the IPBlock.
// For example, in case network policy object has
// name: "test"
// namespace: "default"
// ingress rule
// it returns "test-in-ns-default-0IN".
func ipBlockSetName(policyName, ns string, direction policies.Direction, ipBlockSetIndex int) string {
	return fmt.Sprintf(ipBlocksetNameFormat, policyName, ns, ipBlockSetIndex, direction)
}

// ipBlockIPSet
func ipBlockIPSet(policyName, ns string, direction policies.Direction, ipBlockSetIndex int, ipBlockRule *networkingv1.IPBlock) *ipsets.TranslatedIPSet {
	if ipBlockRule == nil || ipBlockRule.CIDR == "" {
		return nil
	}

	members := make([]string, len(ipBlockRule.Except)+1) // except + cidr
	cidrIndex := 0
	members[cidrIndex] = ipBlockRule.CIDR
	for i := 0; i < len(ipBlockRule.Except); i++ {
		members[i+1] = ipBlockRule.Except[i] + util.IpsetNomatch
	}

	ipBlockIPSetName := ipBlockSetName(policyName, ns, direction, ipBlockSetIndex)
	ipBlockIPSet := ipsets.NewTranslatedIPSet(ipBlockIPSetName, ipsets.CIDRBlocks, members)
	return ipBlockIPSet
}

func ipBlockRule(policyName, ns string, direction policies.Direction, ipBlockSetIndex int, ipBlockRule *networkingv1.IPBlock) (*ipsets.TranslatedIPSet, policies.SetInfo) {
	if ipBlockRule == nil || ipBlockRule.CIDR == "" {
		return nil, policies.SetInfo{}
	}

	ipBlockIPSet := ipBlockIPSet(policyName, ns, direction, ipBlockSetIndex, ipBlockRule)
	setInfo := policies.NewSetInfo(ipBlockIPSet.Metadata.Name, ipsets.CIDRBlocks, included, policies.SrcMatch)
	return ipBlockIPSet, setInfo
}

func podLabelType(label string) ipsets.SetType {
	// TODO(jungukcho): this is unnecessary function which has extra computation
	// will be removed after optimizing parseSelector function
	labels := strings.Split(label, ":")
	switch LenOfLabels := len(labels); LenOfLabels {
	case onlyKeyLabel:
		return ipsets.KeyLabelOfPod
	case keyValueLabel:
		return ipsets.KeyValueLabelOfPod
	default: // in case of nested value (i.e., len(labels) >= 3
		return ipsets.NestedLabelOfPod
	}
}

// podSelectorRule returns srcList for ACL by using ops and labelsForSpec
func podSelectorRule(matchType policies.MatchType, ops, ipSetForACL []string) []policies.SetInfo {
	podSelectorList := []policies.SetInfo{}
	for i := 0; i < len(ipSetForACL); i++ {
		noOp := ops[i] == ""
		labelType := podLabelType(ipSetForACL[i])
		setInfo := policies.NewSetInfo(ipSetForACL[i], labelType, noOp, matchType)
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

// targetPodSelectorInfo converts podSelector information to operators and corresponding label information.
// The label information has various types based on type of labels (e.g., single value or multiple value in labels).
func targetPodSelectorInfo(selector *metav1.LabelSelector) (ops, ipSetForACL, ipSetForSingleVal []string, ipSetNameForMultiVal map[string][]string) {
	// TODO(jungukcho) : need to revise parseSelector function to reduce computations and enhance readability
	// 1. use better variables to indicate included instead of "".
	// 2. Classify type of set in parseSelector to avoid multiple computations
	// 3. Resolve makezero lint errors (nozero)
	singleValueLabelsWithOps, multiValuesLabelsWithOps := parseSelector(selector)
	ops, ipSetForSingleVal = GetOperatorsAndLabels(singleValueLabelsWithOps)

	ipSetNameForMultiVal = make(map[string][]string)
	LenOfIPSetForACL := len(ipSetForSingleVal) + len(multiValuesLabelsWithOps)
	ipSetForACL = make([]string, LenOfIPSetForACL)
	IndexOfIPSetForACL := copy(ipSetForACL, ipSetForSingleVal)

	for multiValueLabelKeyWithOps, multiValueLabelList := range multiValuesLabelsWithOps {
		op, multiValueLabelKey := GetOperatorAndLabel(multiValueLabelKeyWithOps)
		ops = append(ops, op) // nozero

		ipSetNameForMultiValueLabel := getSetNameForMultiValueSelector(multiValueLabelKey, multiValueLabelList)
		ipSetForACL[IndexOfIPSetForACL] = ipSetNameForMultiValueLabel
		IndexOfIPSetForACL++

		for _, labelValue := range multiValueLabelList {
			ipsetName := util.GetIpSetFromLabelKV(multiValueLabelKey, labelValue)
			ipSetForSingleVal = append(ipSetForSingleVal, ipsetName) // nozero
			ipSetNameForMultiVal[ipSetNameForMultiValueLabel] = append(ipSetNameForMultiVal[ipSetNameForMultiValueLabel], ipsetName)
		}
	}
	return ops, ipSetForACL, ipSetForSingleVal, ipSetNameForMultiVal
}

// allPodsSelectorInNs returns translatedIPSet and SetInfo
// in case podSelector field has {} which means all pods in the ns namespace.
func allPodsSelectorInNs(ns string, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	// TODO(jungukcho): important this is common component - double-check whether it has duplicated one or not
	ipset := ipsets.NewTranslatedIPSet(ns, ipsets.Namespace, []string{})
	podSelectorIPSets := []*ipsets.TranslatedIPSet{ipset}

	setInfo := policies.NewSetInfo(ns, ipsets.Namespace, included, matchType)
	podSelectorList := []policies.SetInfo{setInfo}
	return podSelectorIPSets, podSelectorList
}

// PodSelector translates podSelector of spec field and NetworkPolicyPeer in networkpolicy object to translatedIPSet and SetInfo.
// TODO(jungukcho): change name of function to podSelector since it uses both podSelector of spec field and NetworkPolicyPeer in networkpolicy object.
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

// PodSelector translates podSelector of NetworkPolicyPeer field in networkpolicy object to translatedIPSet and SetInfo.
// This function is called only when the NetworkPolicyPeer has namespaceSelector field.
func podSelector(matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	podSelectors := parsePodSelector(selector)
	LenOfPodSelectors := len(podSelectors)
	podSelectorIPSets := []*ipsets.TranslatedIPSet{}
	podSelectorList := make([]policies.SetInfo, LenOfPodSelectors)

	for i := 0; i < LenOfPodSelectors; i++ {
		ps := podSelectors[i]
		podSelectorIPSets = append(podSelectorIPSets, ipsets.NewTranslatedIPSet(ps.setName, ps.setType, ps.members))
		// if value is nested value, create translatedIPSet with the nested value
		for j := 0; j < len(ps.members); j++ {
			var nilSlices []string
			podSelectorIPSets = append(podSelectorIPSets, ipsets.NewTranslatedIPSet(ps.members[j], ipsets.KeyValueLabelOfPod, nilSlices))
		}

		podSelectorList[i] = policies.NewSetInfo(ps.setName, ps.setType, ps.include, matchType)
	}

	return podSelectorIPSets, podSelectorList
}

// podSelectorWithNS translates podSelector of spec and NetworkPolicyPeer in networkpolicy object to translatedIPSet and SetInfo.
// This function is called only when the NetworkPolicyPeer does not have namespaceSelector field.
func podSelectorWithNS(ns string, matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	podSelectorIPSets, podSelectorList := podSelector(matchType, selector)

	// Add translatedIPSet and SetInfo based on namespace
	// TODO(jungukcho) this nilSlices will be removed.
	var nilSlices []string
	podSelectorIPSets = append(podSelectorIPSets, ipsets.NewTranslatedIPSet(ns, ipsets.Namespace, nilSlices))
	podSelectorList = append(podSelectorList, policies.NewSetInfo(ns, ipsets.Namespace, included, matchType))
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
		noOp := ops[i] == ""
		labelType := nsLabelType(nsSelectorInfo[i])
		setInfo := policies.NewSetInfo(nsSelectorInfo[i], labelType, noOp, matchType)
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

func nameSpaceSelectorInfo(selector *metav1.LabelSelector) (ops, singleValueLabels []string) {
	// parse namespace label selector.
	// Ignore multiple values from parseSelector since Namespace selector does not have multiple values.
	// TODO(jungukcho): will revise parseSelector for easy understanding between podSelector and namespaceSelector
	singleValueLabelsWithOps, _ := parseSelector(selector)
	ops, singleValueLabels = GetOperatorsAndLabels(singleValueLabelsWithOps)
	return ops, singleValueLabels
}

// allNameSpaceRule returns translatedIPSet and SetInfo
// in case namespaceSelector field has {} which means all namespaces.
func allNameSpaceRule(matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	translatedIPSet := ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.Namespace, []string{})
	nsSelectorIPSets := []*ipsets.TranslatedIPSet{translatedIPSet}

	setInfo := policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.Namespace, included, matchType)
	nsSelectorList := []policies.SetInfo{setInfo}
	return nsSelectorIPSets, nsSelectorList
}

// nameSpaceSelector translates namespaceSelector of NetworkPolicyPeer in networkpolicy object to translatedIPSet and SetInfo.
func nameSpaceSelector(matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	nsSelectors := parseNSSelector(selector)
	LenOfnsSelectors := len(nsSelectors)
	nsSelectorIPSets := make([]*ipsets.TranslatedIPSet, LenOfnsSelectors)
	nsSelectorList := make([]policies.SetInfo, LenOfnsSelectors)

	for i := 0; i < LenOfnsSelectors; i++ {
		nsc := nsSelectors[i]
		nsSelectorIPSets[i] = ipsets.NewTranslatedIPSet(nsc.setName, nsc.setType, []string{})
		nsSelectorList[i] = policies.NewSetInfo(nsc.setName, nsc.setType, nsc.include, matchType)
	}

	return nsSelectorIPSets, nsSelectorList
}

// allowAllTraffic returns translatedIPSet and SetInfo in case of allow all internal traffic.
func allowAllTraffic(matchType policies.MatchType) (*ipsets.TranslatedIPSet, policies.SetInfo) {
	allowAllIPSets := ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.Namespace, []string{})
	setInfo := policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.Namespace, included, matchType)
	return allowAllIPSets, setInfo
}

// defaultDropACL returns ACLPolicy to drop traffic which is not allowed.
func defaultDropACL(policyNS, policyName string, direction policies.Direction) *policies.ACLPolicy {
	dropACL := policies.NewACLPolicy(policyNS, policyName, policies.Dropped, direction)
	return dropACL
}

// ruleExists returns type of rules from networkingv1.NetworkPolicyIngressRule or networkingv1.NetworkPolicyEgressRule
func ruleExists(ports []networkingv1.NetworkPolicyPort, peer []networkingv1.NetworkPolicyPeer) (allowExternal, portRuleExists, peerRuleExists bool) {
	// TODO(jungukcho): need to clarify and summarize below flags
	portRuleExists = len(ports) > 0
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

// peerAndPortRule deals with composite rules including ports and peers
// (e.g., IPBlock, podSelector, namespaceSelector, or both podSelector and namespaceSelector)
func peerAndPortRule(npmNetPol *policies.NPMNetworkPolicy, ports []networkingv1.NetworkPolicyPort, setInfo []policies.SetInfo) {
	if len(ports) == 0 {
		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
		acl.SrcList = setInfo
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
		return
	}

	for i := range ports {
		portKind, err := portType(ports[i])
		if err != nil {
			// TODO(jungukcho): handle error
			klog.Infof("Invalid NetworkPolicyPort %s", err)
			continue
		}

		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
		acl.SrcList = setInfo
		npmNetPol.RuleIPSets = portRule(npmNetPol.RuleIPSets, acl, &ports[i], portKind)
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
	}
}

// translateIngress traslates podSelector of spec field and NetworkPolicyIngressRule in networkpolicy object
// to NPMNetworkPolicy object.
func translateIngress(npmNetPol *policies.NPMNetworkPolicy, targetSelector *metav1.LabelSelector, rules []networkingv1.NetworkPolicyIngressRule) {
	npmNetPol.PodSelectorIPSets, npmNetPol.PodSelectorList = podSelectorWithNS(npmNetPol.NameSpace, policies.DstMatch, targetSelector)

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
			for i := range rule.Ports {
				portKind, err := portType(rule.Ports[i])
				if err != nil {
					klog.Infof("Invalid NetworkPolicyPort %s", err)
					continue
				}

				portACL := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, policies.Ingress)
				npmNetPol.RuleIPSets = portRule(npmNetPol.RuleIPSets, portACL, &rule.Ports[i], portKind)
				npmNetPol.ACLs = append(npmNetPol.ACLs, portACL)
			}
			continue
		}

		// #2. From fields exist in rule
		for j, fromRule := range rule.From {
			// #2.1 Handle IPBlock and port if exist
			if fromRule.IPBlock != nil {
				if len(fromRule.IPBlock.CIDR) > 0 {
					ipBlockIPSet, ipBlockSetInfo := ipBlockRule(npmNetPol.Name, npmNetPol.NameSpace, policies.Ingress, i, fromRule.IPBlock)
					npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, ipBlockIPSet)
					// all IPBlock entries (i.e., cidr and except) will be added in one ipset per from rule
					if j != 0 {
						continue
					}
					peerAndPortRule(npmNetPol, rule.Ports, []policies.SetInfo{ipBlockSetInfo})
				}
				// Do not check further since IPBlock field is exclusive in NetworkPolicyPeer (i.e., fromRule in this code).
				continue
			}

			// if there is no podSelector or namespaceSelector in fromRule, no need to check below code.
			if fromRule.PodSelector == nil && fromRule.NamespaceSelector == nil {
				continue
			}

			// #2.2 handle nameSpaceSelector and port if exist
			if fromRule.PodSelector == nil && fromRule.NamespaceSelector != nil {
				flattenNSSelctor := FlattenNameSpaceSelector(fromRule.NamespaceSelector)
				for i := range flattenNSSelctor {
					nsSelectorIPSets, nsSrcList := nameSpaceSelector(policies.SrcMatch, &flattenNSSelctor[i])
					npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, nsSelectorIPSets...)
					peerAndPortRule(npmNetPol, rule.Ports, nsSrcList)
				}
				continue
			}

			// #2.3 handle podSelector and port if exist
			if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
				// TODO check old code if we need any ns- prefix for pod selectors
				podSelectorIPSets, podSelectorSrcList := podSelectorWithNS(npmNetPol.NameSpace, policies.SrcMatch, fromRule.PodSelector)
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
			podSelectorIPSets, podSelectorSrcList := podSelector(policies.SrcMatch, fromRule.PodSelector)
			npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, podSelectorIPSets...)

			flattenNSSelctor := FlattenNameSpaceSelector(fromRule.NamespaceSelector)
			for i := range flattenNSSelctor {
				nsSelectorIPSets, nsSrcList := nameSpaceSelector(policies.SrcMatch, &flattenNSSelctor[i])
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
}

func existIngress(npObj *networkingv1.NetworkPolicy) bool {
	return !(npObj.Spec.Ingress != nil &&
		len(npObj.Spec.Ingress) == 1 &&
		len(npObj.Spec.Ingress[0].Ports) == 0 &&
		len(npObj.Spec.Ingress[0].From) == 0)
}

// TranslatePolicy traslates networkpolicy object to NPMNetworkPolicy object
// and return the NPMNetworkPolicy object.
func TranslatePolicy(npObj *networkingv1.NetworkPolicy) *policies.NPMNetworkPolicy {
	npmNetPol := &policies.NPMNetworkPolicy{
		Name:      npObj.ObjectMeta.Name,
		NameSpace: npObj.ObjectMeta.Namespace,
	}

	if len(npObj.Spec.PolicyTypes) == 0 {
		translateIngress(npmNetPol, &npObj.Spec.PodSelector, npObj.Spec.Ingress)
		return npmNetPol
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			translateIngress(npmNetPol, &npObj.Spec.PodSelector, npObj.Spec.Ingress)
		}
	}

	if hasIngress := existIngress(npObj); hasIngress {
		dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Ingress)
		npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
	}
	return npmNetPol
}
