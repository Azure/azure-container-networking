package translation

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

/*
TODO(jungukcho)
1. namespace is default in label in K8s. Need to check whether I missed something.
- Targeting a Namespace by its name
(https://kubernetes.io/docs/concepts/services-networking/network-policies/#targeting-a-namespace-by-its-name)
*/

var (
	errUnknownPortType = errors.New("unknown port Type")
	// ErrUnsupportedNamedPort is returned when named port translation feature is used in windows.
	ErrUnsupportedNamedPort = errors.New("unsupported namedport translation features used on windows")
	// ErrUnsupportedNegativeMatch is returned when negative match translation feature is used in windows.
	ErrUnsupportedNegativeMatch = errors.New("unsupported NotExist operator translation features used on windows")
)

type netpolPortType string

const (
	numericPortType      netpolPortType = "validport"
	namedPortType        netpolPortType = "namedport"
	included             bool           = true
	ipBlocksetNameFormat                = "%s-in-ns-%s-%d%s"
)

// portType returns type of ports (e.g., numeric port or namedPort) given NetworkPolicyPort object.
func portType(portRule networkingv1.NetworkPolicyPort) (netpolPortType, error) {
	if portRule.Port == nil || portRule.Port.IntValue() != 0 {
		return numericPortType, nil
	} else if portRule.Port.IntValue() == 0 && portRule.Port.String() != "" {
		if util.IsWindowsDP() {
			klog.Warningf("Windows does not support named port. Use numeric port instead.")
			return "", ErrUnsupportedNamedPort
		}
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

	namedPortIPSet = ipsets.NewTranslatedIPSet(portRule.Port.String(), ipsets.NamedPorts)
	return namedPortIPSet, protocol
}

func namedPortRule(portRule *networkingv1.NetworkPolicyPort) (*ipsets.TranslatedIPSet, policies.SetInfo, string) {
	if portRule == nil {
		return nil, policies.SetInfo{}, ""
	}

	namedPortIPSet, protocol := namedPortRuleInfo(portRule)
	setInfo := policies.NewSetInfo(portRule.Port.String(), ipsets.NamedPorts, included, policies.DstDstMatch)
	return namedPortIPSet, setInfo, protocol
}

func portRule(ruleIPSets []*ipsets.TranslatedIPSet, acl *policies.ACLPolicy, portRule *networkingv1.NetworkPolicyPort, portType netpolPortType) []*ipsets.TranslatedIPSet {
	// port rule is always applied to destination side.
	if portType == namedPortType {
		namedPortIPSet, namedPortRuleDstList, protocol := namedPortRule(portRule)
		acl.AddSetInfo([]policies.SetInfo{namedPortRuleDstList})
		acl.Protocol = policies.Protocol(protocol)
		ruleIPSets = append(ruleIPSets, namedPortIPSet)
	} else if portType == numericPortType {
		portInfo, protocol := numericPortRule(portRule)
		acl.DstPorts = portInfo
		acl.Protocol = policies.Protocol(protocol)
	}

	return ruleIPSets
}

// ipBlockSetName returns ipset name of the IPBlock.
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

// exceptCidr returns "cidr + " " (space) + nomatch" format.
// e.g., "10.0.0.0/1 nomatch"
func exceptCidr(exceptCidr string) string {
	return exceptCidr + " " + util.IpsetNomatch
}

// deDuplicateExcept removes redundance elements and return slices which has only unique element.
func deDuplicateExcept(exceptInIPBlock []string) []string {
	deDupExcepts := []string{}
	exceptsSet := make(map[string]struct{})
	for _, except := range exceptInIPBlock {
		if _, exist := exceptsSet[except]; !exist {
			deDupExcepts = append(deDupExcepts, except)
			exceptsSet[except] = struct{}{}
		}
	}
	return deDupExcepts
}

// ipBlockIPSet return translatedIPSet based based on ipBlockRule.
func ipBlockIPSet(policyName, ns string, direction policies.Direction, ipBlockSetIndex int, ipBlockRule *networkingv1.IPBlock) *ipsets.TranslatedIPSet {
	if ipBlockRule == nil || ipBlockRule.CIDR == "" {
		return nil
	}

	// de-duplicated Except if there are redundance elements.
	deDupExcepts := deDuplicateExcept(ipBlockRule.Except)
	lenOfDeDupExcepts := len(deDupExcepts)
	var members []string
	indexOfMembers := 0
	// Ipset doesn't allow 0.0.0.0/0 to be added.
	// A solution is split 0.0.0.0/0 in half which convert to 0.0.0.0/1 and 128.0.0.0/1.
	// splitCIDRSet is used to handle case where IPBlock has "0.0.0.0/0" in CIDR and "0.0.0.0/1" or "128.0.0.0/1"  in Except.
	// splitCIDRSet has two entries ("0.0.0.0/1" and "128.0.0.0/1") as key.
	splitCIDRLen := 2
	splitCIDRSet := make(map[string]int, splitCIDRLen)
	if ipBlockRule.CIDR == "0.0.0.0/0" {
		// two cidrs (0.0.0.0/1 and 128.0.0.0/1) for 0.0.0.0/0 + except.
		members = make([]string, lenOfDeDupExcepts+splitCIDRLen)
		// in case of "0.0.0.0/0", "0.0.0.0/1" or "0.0.0.0/1 nomatch" comes eariler than "128.0.0.0/1" or "128.0.0.0/1 nomatch".
		splitCIDRs := []string{"0.0.0.0/1", "128.0.0.0/1"}
		for _, cidr := range splitCIDRs {
			members[indexOfMembers] = cidr
			splitCIDRSet[cidr] = indexOfMembers
			indexOfMembers++
		}
	} else {
		// one cidr + except
		members = make([]string, lenOfDeDupExcepts+1)
		members[indexOfMembers] = ipBlockRule.CIDR
		indexOfMembers++
	}

	for i := 0; i < lenOfDeDupExcepts; i++ {
		except := deDupExcepts[i]
		if splitCIDRIndex, exist := splitCIDRSet[except]; exist {
			// replace stored splitCIDR with "nomatch" option
			members[splitCIDRIndex] = exceptCidr(except)
			indexOfMembers--
			members = members[:len(members)-1]
		} else {
			members[i+indexOfMembers] = exceptCidr(except)
		}
	}

	ipBlockIPSetName := ipBlockSetName(policyName, ns, direction, ipBlockSetIndex)
	ipBlockIPSet := ipsets.NewTranslatedIPSet(ipBlockIPSetName, ipsets.CIDRBlocks, members...)
	return ipBlockIPSet
}

// ipBlockRule translates IPBlock field in networkpolicy object to translatedIPSet and SetInfo.
// ipBlockSetIndex parameter is used to diffentiate ipBlock fields in one networkpolicy object.
func ipBlockRule(policyName, ns string, direction policies.Direction, matchType policies.MatchType, ipBlockSetIndex int,
	ipBlockRule *networkingv1.IPBlock) (*ipsets.TranslatedIPSet, policies.SetInfo) {

	if ipBlockRule == nil || ipBlockRule.CIDR == "" {
		return nil, policies.SetInfo{}
	}

	ipBlockIPSet := ipBlockIPSet(policyName, ns, direction, ipBlockSetIndex, ipBlockRule)
	setInfo := policies.NewSetInfo(ipBlockIPSet.Metadata.Name, ipsets.CIDRBlocks, included, matchType)
	return ipBlockIPSet, setInfo
}

// PodSelector translates podSelector of NetworkPolicyPeer field in networkpolicy object to translatedIPSet and SetInfo.
// This function is called only when the NetworkPolicyPeer has namespaceSelector field.
func podSelector(matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo, error) {
	podSelectors, err := parsePodSelector(selector)
	if err != nil {
		return nil, nil, err
	}
	lenOfPodSelectors := len(podSelectors)
	podSelectorIPSets := []*ipsets.TranslatedIPSet{}
	podSelectorList := make([]policies.SetInfo, lenOfPodSelectors)

	for i := 0; i < lenOfPodSelectors; i++ {
		ps := podSelectors[i]
		podSelectorIPSets = append(podSelectorIPSets, ipsets.NewTranslatedIPSet(ps.setName, ps.setType, ps.members...))
		// if value is nested value, create translatedIPSet with the nested value
		for j := 0; j < len(ps.members); j++ {
			podSelectorIPSets = append(podSelectorIPSets, ipsets.NewTranslatedIPSet(ps.members[j], ipsets.KeyValueLabelOfPod))
		}

		podSelectorList[i] = policies.NewSetInfo(ps.setName, ps.setType, ps.include, matchType)
	}

	return podSelectorIPSets, podSelectorList, nil
}

// podSelectorWithNS translates podSelector of spec and NetworkPolicyPeer in networkpolicy object to translatedIPSet and SetInfo.
// This function is called only when the NetworkPolicyPeer does not have namespaceSelector field.
func podSelectorWithNS(ns string, matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo, error) {
	podSelectorIPSets, podSelectorList, err := podSelector(matchType, selector)
	if err != nil {
		return nil, nil, err
	}
	// Add translatedIPSet and SetInfo based on namespace
	podSelectorIPSets = append(podSelectorIPSets, ipsets.NewTranslatedIPSet(ns, ipsets.Namespace))
	podSelectorList = append(podSelectorList, policies.NewSetInfo(ns, ipsets.Namespace, included, matchType))
	return podSelectorIPSets, podSelectorList, nil
}

// nameSpaceSelector translates namespaceSelector of NetworkPolicyPeer in networkpolicy object to translatedIPSet and SetInfo.
func nameSpaceSelector(matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	nsSelectors := parseNSSelector(selector)
	lenOfnsSelectors := len(nsSelectors)
	nsSelectorIPSets := make([]*ipsets.TranslatedIPSet, lenOfnsSelectors)
	nsSelectorList := make([]policies.SetInfo, lenOfnsSelectors)

	for i := 0; i < lenOfnsSelectors; i++ {
		nsc := nsSelectors[i]
		nsSelectorIPSets[i] = ipsets.NewTranslatedIPSet(nsc.setName, nsc.setType)
		nsSelectorList[i] = policies.NewSetInfo(nsc.setName, nsc.setType, nsc.include, matchType)
	}

	return nsSelectorIPSets, nsSelectorList
}

// allowAllInternal returns translatedIPSet and SetInfo in case of allowing all internal traffic excluding external.
func allowAllInternal(matchType policies.MatchType) (*ipsets.TranslatedIPSet, policies.SetInfo) {
	allowAllIPSets := ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace)
	setInfo := policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace, included, matchType)
	return allowAllIPSets, setInfo
}

// ruleExists returns type of rules from networkingv1.NetworkPolicyIngressRule or networkingv1.NetworkPolicyEgressRule
func ruleExists(ports []networkingv1.NetworkPolicyPort, peer []networkingv1.NetworkPolicyPeer) (allowExternal, portRuleExists, peerRuleExists bool) {
	// TODO(jungukcho): need to clarify and summarize below flags + more comments
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
// (e.g., IPBlock, podSelector, namespaceSelector, or both podSelector and namespaceSelector).
func peerAndPortRule(npmNetPol *policies.NPMNetworkPolicy, direction policies.Direction, ports []networkingv1.NetworkPolicyPort, setInfo []policies.SetInfo) error {
	if len(ports) == 0 {
		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
		acl.AddSetInfo(setInfo)
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
		return nil
	}

	for i := range ports {
		portKind, err := portType(ports[i])
		if err != nil {
			return err
		}

		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
		acl.AddSetInfo(setInfo)
		npmNetPol.RuleIPSets = portRule(npmNetPol.RuleIPSets, acl, &ports[i], portKind)
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
	}
	return nil
}

// translateRule translates ingress or egress rules and update npmNetPol object.
func translateRule(npmNetPol *policies.NPMNetworkPolicy, direction policies.Direction, matchType policies.MatchType, ruleIndex int,
	ports []networkingv1.NetworkPolicyPort, peers []networkingv1.NetworkPolicyPeer) error {
	// TODO(jungukcho): need to clean up it.
	// Leave allowExternal variable now while the condition is checked before calling this function.
	allowExternal, portRuleExists, peerRuleExists := ruleExists(ports, peers)

	// #0. TODO(jungukcho): cannot come up when this condition is met.
	// The code inside if condition is to handle allowing all internal traffic, but the case is handled in #2.4.
	// So, this code may not execute. After confirming this, need to delete it.
	if !portRuleExists && !peerRuleExists && !allowExternal {
		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
		ruleIPSets, allowAllInternalSetInfo := allowAllInternal(matchType)
		npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, ruleIPSets)
		acl.AddSetInfo([]policies.SetInfo{allowAllInternalSetInfo})
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
		return nil
	}

	// #1. Only Ports fields exist in rule
	if portRuleExists && !peerRuleExists && !allowExternal {
		for i := range ports {
			portKind, err := portType(ports[i])
			if err != nil {
				return err
			}

			portACL := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
			npmNetPol.RuleIPSets = portRule(npmNetPol.RuleIPSets, portACL, &ports[i], portKind)
			npmNetPol.ACLs = append(npmNetPol.ACLs, portACL)
		}
	}

	// #2. From or To fields exist in rule
	for j, peer := range peers {
		// #2.1 Handle IPBlock and port if exist
		if peer.IPBlock != nil {
			if len(peer.IPBlock.CIDR) > 0 {
				ipBlockIPSet, ipBlockSetInfo := ipBlockRule(npmNetPol.Name, npmNetPol.NameSpace, direction, matchType, ruleIndex, peer.IPBlock)
				npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, ipBlockIPSet)
				// all fields (i.e., cidr and except) in peer.IPBlock will be added in one ipset per peer
				if j != 0 {
					continue
				}
				err := peerAndPortRule(npmNetPol, direction, ports, []policies.SetInfo{ipBlockSetInfo})
				if err != nil {
					return err
				}
			}
			// Do not need to run below code to translate PodSelector and NamespaceSelector
			// since IPBlock field is exclusive in NetworkPolicyPeer (i.e., peer in this code).
			continue
		}

		// if there is no PodSelector or NamespaceSelector in peer, no need to run the rest of codes.
		if peer.PodSelector == nil && peer.NamespaceSelector == nil {
			continue
		}

		// #2.2 handle nameSpaceSelector and port if exist
		if peer.PodSelector == nil && peer.NamespaceSelector != nil {
			// Before translating NamespaceSelector, flattenNameSpaceSelector function call should be called
			// to handle multiple values in matchExpressions spec.
			flattenNSSelector := flattenNameSpaceSelector(peer.NamespaceSelector)
			for i := range flattenNSSelector {
				nsSelectorIPSets, nsSelectorList := nameSpaceSelector(matchType, &flattenNSSelector[i])
				npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, nsSelectorIPSets...)
				err := peerAndPortRule(npmNetPol, direction, ports, nsSelectorList)
				if err != nil {
					return err
				}
			}
			continue
		}

		// #2.3 handle podSelector and port if exist
		if peer.PodSelector != nil && peer.NamespaceSelector == nil {
			podSelectorIPSets, podSelectorList, err := podSelectorWithNS(npmNetPol.NameSpace, matchType, peer.PodSelector)
			if err != nil {
				return err
			}
			npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, podSelectorIPSets...)
			err = peerAndPortRule(npmNetPol, direction, ports, podSelectorList)
			if err != nil {
				return err
			}
			continue
		}

		// peer has both namespaceSelector and podSelector set.
		// We should match the selected pods in the selected namespaces.
		// This allows traffic from podSelector intersects namespaceSelector
		// This is only supported in kubernetes version >= 1.11
		if !util.IsNewNwPolicyVerFlag {
			continue
		}

		// #2.4 handle namespaceSelector and podSelector and port if exist
		podSelectorIPSets, podSelectorList, err := podSelector(matchType, peer.PodSelector)
		if err != nil {
			return err
		}
		npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, podSelectorIPSets...)

		// Before translating NamespaceSelector, flattenNameSpaceSelector function call should be called
		// to handle multiple values in matchExpressions spec.
		flattenNSSelector := flattenNameSpaceSelector(peer.NamespaceSelector)
		for i := range flattenNSSelector {
			nsSelectorIPSets, nsSelectorList := nameSpaceSelector(matchType, &flattenNSSelector[i])
			npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, nsSelectorIPSets...)
			nsSelectorList = append(nsSelectorList, podSelectorList...)
			err := peerAndPortRule(npmNetPol, direction, ports, nsSelectorList)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// defaultDropACL returns ACLPolicy to drop traffic which is not allowed.
func defaultDropACL(policyNS, policyName string, direction policies.Direction) *policies.ACLPolicy {
	dropACL := policies.NewACLPolicy(policyNS, policyName, policies.Dropped, direction)
	return dropACL
}

// allowAllPolicy adds acl to allow all traffic including internal (i.e,. K8s cluster) and external (i.e., internet)
func allowAllPolicy(npmNetPol *policies.NPMNetworkPolicy, direction policies.Direction) {
	allowAllACL := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
	npmNetPol.ACLs = append(npmNetPol.ACLs, allowAllACL)
}

// isAllowAllToIngress returns true if this network policy allows all traffic from internal (i.e,. K8s cluster) and external (i.e., internet)
// Otherwise, it returns false.
func isAllowAllToIngress(ingress []networkingv1.NetworkPolicyIngressRule) bool {
	return len(ingress) == 1 &&
		len(ingress[0].Ports) == 0 &&
		len(ingress[0].From) == 0
}

// ingressPolicy traslates NetworkPolicyIngressRule in NetworkPolicy object
// to NPMNetworkPolicy object.
func ingressPolicy(npmNetPol *policies.NPMNetworkPolicy, ingress []networkingv1.NetworkPolicyIngressRule) error {
	// #1. Allow all traffic from both internal and external.
	// In yaml file, it is specified with '{}'.
	if isAllowAllToIngress(ingress) {
		allowAllPolicy(npmNetPol, policies.Ingress)
		return nil
	}

	// #2. If ingress is nil (in yaml file, it is specified with '[]'), it means "Deny all" - it does not allow receiving any traffic from others.
	if ingress == nil {
		// Except for allow all traffic case in #1, the rest of them should have default drop rules.
		dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Ingress)
		npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
		return nil
	}

	// #3. Ingress rule is not AllowAll (including internal and external) and DenyAll policy.
	// So, start translating ingress policy.
	for i, rule := range ingress {
		if err := translateRule(npmNetPol, policies.Ingress, policies.SrcMatch, i, rule.Ports, rule.From); err != nil {
			return err
		}
	}
	// Except for allow all traffic case in #1, the rest of them should have default drop rules.
	dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Ingress)
	npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
	return nil
}

// isAllowAllToEgress returns true if this network policy allows all traffic to internal (i.e,. K8s cluster) and external (i.e., internet)
// Otherwise, it returns false.
func isAllowAllToEgress(egress []networkingv1.NetworkPolicyEgressRule) bool {
	return len(egress) == 1 &&
		len(egress[0].Ports) == 0 &&
		len(egress[0].To) == 0
}

// egressPolicy traslates NetworkPolicyEgressRule in networkpolicy object
// to NPMNetworkPolicy object.
func egressPolicy(npmNetPol *policies.NPMNetworkPolicy, egress []networkingv1.NetworkPolicyEgressRule) error {
	// #1. Allow all traffic to both internal and external.
	// In yaml file, it is specified with '{}'.
	if isAllowAllToEgress(egress) {
		allowAllPolicy(npmNetPol, policies.Egress)
		return nil
	}

	// #2. If egress is nil (in yaml file, it is specified with '[]'), it means "Deny all" - it does not allow sending traffic to others.
	if egress == nil {
		// Except for allow all traffic case in #1, the rest of them should have default drop rules.
		dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Egress)
		npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
		return nil
	}

	// #3. Egress rule is not AllowAll (including internal and external) and DenyAll.
	// So, start translating egress policy.
	for i, rule := range egress {
		err := translateRule(npmNetPol, policies.Egress, policies.DstMatch, i, rule.Ports, rule.To)
		if err != nil {
			return err
		}
	}

	// #3. Except for allow all traffic case in #1, the rest of them should have default drop rules.
	// Add drop ACL to drop the rest of traffic which is not specified in Egress Spec.
	dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Egress)
	npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
	return nil
}

// TranslatePolicy traslates networkpolicy object to NPMNetworkPolicy object
// and return the NPMNetworkPolicy object.
func TranslatePolicy(npObj *networkingv1.NetworkPolicy) (*policies.NPMNetworkPolicy, error) {
	npmNetPol := policies.NewNPMNetworkPolicy(npObj.Name, npObj.Namespace)

	// podSelector in spec.PodSelector is common for ingress and egress.
	// Process this podSelector first.
	var err error
	npmNetPol.PodSelectorIPSets, npmNetPol.PodSelectorList, err = podSelectorWithNS(npmNetPol.NameSpace, policies.EitherMatch, &npObj.Spec.PodSelector)
	if err != nil {
		return nil, err
	}

	// Each NetworkPolicy includes a policyTypes list which may include either Ingress, Egress, or both.
	// If no policyTypes are specified on a NetworkPolicy then by default Ingress will always be set
	// and Egress will be set if the NetworkPolicy has any egress rules.
	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			err := ingressPolicy(npmNetPol, npObj.Spec.Ingress)
			if err != nil {
				return nil, err
			}
		} else {
			err := egressPolicy(npmNetPol, npObj.Spec.Egress)
			if err != nil {
				return nil, err
			}
		}
	}
	return npmNetPol, nil
}
