package translation

import (
	"errors"
	"fmt"

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

	namedPortIPSet = ipsets.NewTranslatedIPSet(util.NamedPortIPSetPrefix+portRule.Port.String(), ipsets.NamedPorts)
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
	// port rule is always applied to destination side.
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

// ipBlockIPSet return translatedIPSet based based on ipBlockRule.
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
func podSelector(matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	podSelectors := parsePodSelector(selector)
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

	return podSelectorIPSets, podSelectorList
}

// podSelectorWithNS translates podSelector of spec and NetworkPolicyPeer in networkpolicy object to translatedIPSet and SetInfo.
// This function is called only when the NetworkPolicyPeer does not have namespaceSelector field.
func podSelectorWithNS(ns string, matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	podSelectorIPSets, podSelectorList := podSelector(matchType, selector)

	// Add translatedIPSet and SetInfo based on namespace
	podSelectorIPSets = append(podSelectorIPSets, ipsets.NewTranslatedIPSet(ns, ipsets.Namespace))
	podSelectorList = append(podSelectorList, policies.NewSetInfo(ns, ipsets.Namespace, included, matchType))
	return podSelectorIPSets, podSelectorList
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
func peerAndPortRule(npmNetPol *policies.NPMNetworkPolicy, direction policies.Direction, ports []networkingv1.NetworkPolicyPort, setInfo []policies.SetInfo) {
	if len(ports) == 0 {
		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
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

		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
		acl.SrcList = setInfo
		npmNetPol.RuleIPSets = portRule(npmNetPol.RuleIPSets, acl, &ports[i], portKind)
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
	}
}

// translateRule translates ingress or egress rules and update npmNetPol object.
func translateRule(npmNetPol *policies.NPMNetworkPolicy, direction policies.Direction, matchType policies.MatchType, ruleIndex int,
	ports []networkingv1.NetworkPolicyPort, peers []networkingv1.NetworkPolicyPeer) {
	// TODO(jungukcho): need to clean up it.
	// Leave allowExternal variable now while the condition is checked before calling this function.
	allowExternal, portRuleExists, fromRuleExists := ruleExists(ports, peers)

	// #0. TODO(jungukcho): cannot come up when this condition is met.
	// The code inside if condition is to handleallowing all internal traffic, but the case is handled in #2.4.
	// So, this code may not execute. After confirming this, need to delete it.
	if !portRuleExists && !fromRuleExists && !allowExternal {
		acl := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
		ruleIPSets, setInfo := allowAllInternal(matchType)
		npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, ruleIPSets)
		acl.SrcList = append(acl.SrcList, setInfo)
		npmNetPol.ACLs = append(npmNetPol.ACLs, acl)
		return
	}

	// #1. Only Ports fields exist in rule
	if portRuleExists && !fromRuleExists && !allowExternal {
		for i := range ports {
			portKind, err := portType(ports[i])
			if err != nil {
				klog.Infof("Invalid NetworkPolicyPort %s", err)
				continue
			}

			portACL := policies.NewACLPolicy(npmNetPol.NameSpace, npmNetPol.Name, policies.Allowed, direction)
			npmNetPol.RuleIPSets = portRule(npmNetPol.RuleIPSets, portACL, &ports[i], portKind)
			npmNetPol.ACLs = append(npmNetPol.ACLs, portACL)
		}
	}

	// #2. From fields exist in rule
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
				peerAndPortRule(npmNetPol, direction, ports, []policies.SetInfo{ipBlockSetInfo})
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
			flattenNSSelctor := flattenNameSpaceSelector(peer.NamespaceSelector)
			for i := range flattenNSSelctor {
				nsSelectorIPSets, nsSrcList := nameSpaceSelector(matchType, &flattenNSSelctor[i])
				npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, nsSelectorIPSets...)
				peerAndPortRule(npmNetPol, direction, ports, nsSrcList)
			}
			continue
		}

		// #2.3 handle podSelector and port if exist
		if peer.PodSelector != nil && peer.NamespaceSelector == nil {
			podSelectorIPSets, podSelectorSrcList := podSelectorWithNS(npmNetPol.NameSpace, matchType, peer.PodSelector)
			npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, podSelectorIPSets...)
			peerAndPortRule(npmNetPol, direction, ports, podSelectorSrcList)
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
		podSelectorIPSets, podSelectorSrcList := podSelector(matchType, peer.PodSelector)
		npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, podSelectorIPSets...)

		// Before translating NamespaceSelector, flattenNameSpaceSelector function call should be called
		// to handle multiple values in matchExpressions spec.
		flattenNSSelctor := flattenNameSpaceSelector(peer.NamespaceSelector)
		for i := range flattenNSSelctor {
			nsSelectorIPSets, nsSrcList := nameSpaceSelector(matchType, &flattenNSSelctor[i])
			npmNetPol.RuleIPSets = append(npmNetPol.RuleIPSets, nsSelectorIPSets...)
			nsSrcList = append(nsSrcList, podSelectorSrcList...)
			peerAndPortRule(npmNetPol, direction, ports, nsSrcList)
		}
	}
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
	return (ingress != nil &&
		len(ingress) == 1 &&
		len(ingress[0].Ports) == 0 &&
		len(ingress[0].From) == 0)
}

// ingressPolicy traslates NetworkPolicyIngressRule in NetworkPolicy object
// to NPMNetworkPolicy object.
func ingressPolicy(npmNetPol *policies.NPMNetworkPolicy, ingress []networkingv1.NetworkPolicyIngressRule) {
	// #1. Allow all traffic from both internal and external.
	// In yaml file, it is specified with '{}'.
	if isAllowAllToIngress(ingress) {
		allowAllPolicy(npmNetPol, policies.Ingress)
		return
	}

	// #2. If ingress is nil (in yaml file, it is specified with '[]'), it means it does not allow receiving any traffic from others.
	// In the case, skip translateRule function call and only need default drop ACL..
	// If ingress is not nil, Ingress rule is not AllowAll (including internal and external) and DenyAll policy.
	// So, start translating ingress policy.
	if ingress != nil {
		for i, rule := range ingress {
			translateRule(npmNetPol, policies.Ingress, policies.SrcMatch, i, rule.Ports, rule.From)
		}
	}

	// #3. Except for allow all traffic case in #1, the rest of them should not have default drop rules.
	// Add drop ACL to drop the rest of traffic which is not specified in Egress Spec.
	dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Ingress)
	npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
}

// isAllowAllToEgress returns true if this network policy allows all traffic to internal (i.e,. K8s cluster) and external (i.e., internet)
// Otherwise, it returns false.
func isAllowAllToEgress(egress []networkingv1.NetworkPolicyEgressRule) bool {
	return (egress != nil &&
		len(egress) == 1 &&
		len(egress[0].Ports) == 0 &&
		len(egress[0].To) == 0)
}

// egressPolicy traslates NetworkPolicyEgressRule in networkpolicy object
// to NPMNetworkPolicy object.
func egressPolicy(npmNetPol *policies.NPMNetworkPolicy, egress []networkingv1.NetworkPolicyEgressRule) {
	// #1. Allow all traffic to both internal and external.
	// In yaml file, it is specified with '{}'.
	if isAllowAllToEgress(egress) {
		allowAllPolicy(npmNetPol, policies.Egress)
		return
	}

	// #2. If egress is nil (in yaml file, it is specified with '[]'), it means it does not allow sending traffic to others.
	// In the case, skip translateRule function call and only need default drop ACL.
	// If egress is not nil, Egress rule is not AllowAll (including internal and external) and DenyAll.
	// So, start translating egress policy.
	if egress != nil {
		for i, rule := range egress {
			translateRule(npmNetPol, policies.Egress, policies.DstMatch, i, rule.Ports, rule.To)
		}
	}

	// #3. Except for allow all traffic case in #1, the rest of them should not have default drop rules.
	// Add drop ACL to drop the rest of traffic which is not specified in Egress Spec.
	dropACL := defaultDropACL(npmNetPol.NameSpace, npmNetPol.Name, policies.Egress)
	npmNetPol.ACLs = append(npmNetPol.ACLs, dropACL)
}

// TranslatePolicy traslates networkpolicy object to NPMNetworkPolicy object
// and return the NPMNetworkPolicy object.
func TranslatePolicy(npObj *networkingv1.NetworkPolicy) *policies.NPMNetworkPolicy {
	npmNetPol := &policies.NPMNetworkPolicy{
		Name:      npObj.ObjectMeta.Name,
		NameSpace: npObj.ObjectMeta.Namespace,
	}

	// podSelector in spec.PodSelector is common for ingress and egress.
	// Process this podSelector first.
	npmNetPol.PodSelectorIPSets, npmNetPol.PodSelectorList = podSelectorWithNS(npmNetPol.NameSpace, policies.EitherMatch, &npObj.Spec.PodSelector)

	// Each NetworkPolicy includes a policyTypes list which may include either Ingress, Egress, or both.
	// If no policyTypes are specified on a NetworkPolicy then by default Ingress will always be set
	// and Egress will be set if the NetworkPolicy has any egress rules.
	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			ingressPolicy(npmNetPol, npObj.Spec.Ingress)
		} else {
			egressPolicy(npmNetPol, npObj.Spec.Egress)
		}
	}

	return npmNetPol
}
