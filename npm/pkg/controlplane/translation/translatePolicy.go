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

type translator struct{}

type netpolPortType string

var (
	errIngressTranslation = errors.New("failed to translate ingress rules")
	errUnknownPortType    = errors.New("unknown port Type")
)

const (
	numericPort          netpolPortType = "validport"
	namedPort            netpolPortType = "namedport"
	ipBlocksetNameFormat                = "%s-in-ns-%s-%d%s"
)

func (t *translator) portType(portRule networkingv1.NetworkPolicyPort) (netpolPortType, error) {
	if portRule.Port == nil || portRule.Port.IntValue() != 0 {
		return numericPort, nil
	} else if portRule.Port.IntValue() == 0 && portRule.Port.String() != "" {
		return namedPort, nil
	}
	// TODO (jungukcho): check whether this can be possible or not.
	return "", errUnknownPortType
}

func (t *translator) numericPortRule(portRule *networkingv1.NetworkPolicyPort) (policies.Ports, string) {
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

func ipBlockSetName(policyName, ns string, direction policies.Direction, ipBlockSetIndex int) string {
	return fmt.Sprintf(ipBlocksetNameFormat, policyName, ns, ipBlockSetIndex, direction)
}

func (t *translator) ipBlockIPSet(policyName, ns string, direction policies.Direction, ipBlockSetIndex int, ipBlockRule *networkingv1.IPBlock) *ipsets.TranslatedIPSet {
	if ipBlockRule == nil || len(ipBlockRule.CIDR) == 0 {
		return nil
	}

	/*
		TODO(jungukcho): need to deal with "0.0.0.0/0" here, networkpolicy controller, or ipsetManager?
		Ipset doesn't allow 0.0.0.0/0 to be added. A general solution is split 0.0.0.0/1 in half which convert to
		1.0.0.0/1 and 128.0.0.0/1
	*/

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

func (t *translator) ipBlockRule(policyName, ns string, direction policies.Direction, ipBlockSetIndex int, ipBlockRule *networkingv1.IPBlock) (*ipsets.TranslatedIPSet, policies.SetInfo) {
	if ipBlockRule == nil || len(ipBlockRule.CIDR) == 0 {
		return nil, policies.SetInfo{}
	}

	ipBlockIPSet := t.ipBlockIPSet(policyName, ns, direction, ipBlockSetIndex, ipBlockRule)
	setInfo := policies.NewSetInfo(ipBlockIPSet.Metadata.Name, ipsets.CIDRBlocks, false, policies.SrcMatch)
	return ipBlockIPSet, setInfo
}

func (t *translator) podLabelType(label string) ipsets.SetType {
	// TODO(jungukcho): this is unnecessary function which has extra computation
	// will be removed after optimizing parseSelector function
	labels := strings.Split(label, ":")
	if len(labels) == 1 {
		return ipsets.KeyLabelOfPod
	} else if len(labels) == 2 {
		return ipsets.KeyValueLabelOfPod
	} else if len(labels) == 3 {
		return ipsets.NestedLabelOfPod
	}

	// (TODO): check whether this is possible
	return ipsets.UnknownType
}

// podSelectorRule return srcList for ACL by using ops and labelsForSpec
func (t *translator) podSelectorRule(matchType policies.MatchType, ops, ipSetForACL []string) []policies.SetInfo {
	setInfos := []policies.SetInfo{}
	for i := 0; i < len(ipSetForACL); i++ {
		included := ops[i] == ""
		podLabelType := t.podLabelType(ipSetForACL[i])
		setInfo := policies.NewSetInfo(ipSetForACL[i], podLabelType, included, matchType)
		setInfos = append(setInfos, setInfo)
	}
	return setInfos
}

func (t *translator) podSelectorIPSets(ipSetForSingleVal []string, ipSetNameForMultiVal map[string][]string) []*ipsets.TranslatedIPSet {
	podSelectorIPSets := []*ipsets.TranslatedIPSet{}
	for _, hashSetName := range ipSetForSingleVal {
		podLabelType := t.podLabelType(hashSetName)
		ipset := ipsets.NewTranslatedIPSet(hashSetName, podLabelType, []string{})
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	for listSetName, hashIPSetList := range ipSetNameForMultiVal {
		ipset := ipsets.NewTranslatedIPSet(listSetName, ipsets.NestedLabelOfPod, hashIPSetList)
		podSelectorIPSets = append(podSelectorIPSets, ipset)
	}

	return podSelectorIPSets
}

func (t *translator) targetPodSelectorInfo(selector *metav1.LabelSelector) ([]string, []string, []string, map[string][]string) {
	// TODO(jungukcho) : need to revise parseSelector function to reduce computations and enhance readability
	// 1. use better variables to indicate included instead of "".
	// 2. Classify type of set in parseSelector to avoid multiple computations
	singleValueLabelsWithOps, multiValuesLabelsWithOps := parseSelector(selector)
	ops, ipSetForSingleVal := GetOperatorsAndLabels(singleValueLabelsWithOps)

	ipSetForAcl := make([]string, len(ipSetForSingleVal))
	copy(ipSetForAcl, ipSetForSingleVal)

	listSetMembers := []string{}
	ipSetNameForMultiVal := make(map[string][]string)

	for multiValueLabelKeyWithOps, multiValueLabelList := range multiValuesLabelsWithOps {
		op, multiValueLabelKey := GetOperatorAndLabel(multiValueLabelKeyWithOps)
		ops = append(ops, op)

		ipSetNameForMultiValueLabel := getSetNameForMultiValueSelector(multiValueLabelKey, multiValueLabelList)
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

func (t *translator) allPodsSelectorInNs(ns string, matchType policies.MatchType) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	// TODO(jungukcho): important this is common component - double-check whether it has duplicated one or not
	ipset := ipsets.NewTranslatedIPSet(ns, ipsets.Namespace, []string{})
	podSelectorIPSets := []*ipsets.TranslatedIPSet{ipset}

	setInfo := policies.NewSetInfo(ns, ipsets.Namespace, false, matchType)
	dstList := []policies.SetInfo{setInfo}
	return podSelectorIPSets, dstList
}

func (t *translator) targetPodSelector(ns string, matchType policies.MatchType, selector *metav1.LabelSelector) ([]*ipsets.TranslatedIPSet, []policies.SetInfo) {
	// (TODO): some data in singleValueLabels and multiValuesLabels are duplicated
	ops, ipSetForACL, ipSetForSingleVal, ipSetNameForMultiVal := t.targetPodSelectorInfo(selector)
	// select all pods in a namespace
	if len(ops) == 1 && len(ipSetForSingleVal) == 1 && ops[0] == "" && ipSetForSingleVal[0] == "" {
		podSelectorIPSets, dstList := t.allPodsSelectorInNs(ns, matchType)
		return podSelectorIPSets, dstList
	}

	// TODO(jungukcho): need to order hashset and listset if ipSetNameForMultiVal exists.
	// refer to last test set in TestPodSelectorIPSets
	podSelectorIPSets := t.podSelectorIPSets(ipSetForSingleVal, ipSetNameForMultiVal)
	setInfos := t.podSelectorRule(matchType, ops, ipSetForACL)

	return podSelectorIPSets, setInfos
}

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
	dropACL := policies.NewACLPolicy(npmNetpol.NameSpace, npmNetpol.Name, policies.Dropped, direction)
	npmNetpol.ACLs = append(npmNetpol.ACLs, dropACL)
}

// ruleExists returns type of rules from networkingv1.NetworkPolicyIngressRule or networkingv1.NetworkPolicyEgressRule
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

func (t *translator) portRule(npmNetpol *policies.NPMNetworkPolicy, acl *policies.ACLPolicy, portRule *networkingv1.NetworkPolicyPort, portType netpolPortType) {
	if portType == namedPort {
		namedPortIPSet, namedPortRuleDstList, protocol := t.namedPortRule(portRule)
		acl.DstList = append(acl.DstList, namedPortRuleDstList)
		acl.Protocol = policies.Protocol(protocol)
		npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, namedPortIPSet)
	} else if portType == numericPort {
		portInfo, protocol := t.numericPortRule(portRule)
		acl.DstPorts = portInfo
		acl.Protocol = policies.Protocol(protocol)
	}
}

func (t *translator) peerAndPortRule(npmNetpol *policies.NPMNetworkPolicy, ports []networkingv1.NetworkPolicyPort, setInfo []policies.SetInfo) {
	if ports == nil || len(ports) == 0 {
		acl := policies.NewACLPolicy(npmNetpol.NameSpace, npmNetpol.Name, policies.Allowed, policies.Ingress)
		acl.SrcList = setInfo
		npmNetpol.ACLs = append(npmNetpol.ACLs, acl)
		return
	}

	for _, portRule := range ports {
		portType, err := t.portType(portRule)
		if err != nil {
			// TODO(jungukcho): handle error
			klog.Infof("Invalid NetworkPolicyPort %s", err)
			continue
		}

		acl := policies.NewACLPolicy(npmNetpol.NameSpace, npmNetpol.Name, policies.Allowed, policies.Ingress)
		acl.SrcList = setInfo
		t.portRule(npmNetpol, acl, &portRule, portType)
		npmNetpol.ACLs = append(npmNetpol.ACLs, acl)
	}
}

func (t *translator) translateIngress(npmNetpol *policies.NPMNetworkPolicy, targetSelector metav1.LabelSelector, rules []networkingv1.NetworkPolicyIngressRule) error {
	// TODO(jungukcho) : Double-check addedCidrEntry.
	var addedCidrEntry bool // all cidr entry will be added in one set per from/to rule
	npmNetpol.PodSelectorIPSets, npmNetpol.PodSelectorList = t.targetPodSelector(npmNetpol.NameSpace, policies.DstMatch, &targetSelector)

	for i, rule := range rules {
		allowExternal, portRuleExists, fromRuleExists := t.ruleExists(rule.Ports, rule.From)

		// #0. TODO(jungukcho): cannot come up when this condition is met.
		if !portRuleExists && !fromRuleExists && !allowExternal {
			acl := policies.NewACLPolicy(npmNetpol.NameSpace, npmNetpol.Name, policies.Allowed, policies.Ingress)
			ruleIPSets, setInfo := t.allowAllTraffic()
			npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, ruleIPSets)
			acl.SrcList = append(acl.SrcList, setInfo)
			npmNetpol.ACLs = append(npmNetpol.ACLs, acl)
			continue
		}

		// #1. Only Ports fields exist in rule
		if portRuleExists && !fromRuleExists && !allowExternal {
			for _, portRule := range rule.Ports {
				portType, err := t.portType(portRule)
				if err != nil {
					klog.Infof("Invalid NetworkPolicyPort %s", err)
					continue
				}

				portACL := policies.NewACLPolicy(npmNetpol.NameSpace, npmNetpol.Name, policies.Allowed, policies.Ingress)
				t.portRule(npmNetpol, portACL, &portRule, portType)
				npmNetpol.ACLs = append(npmNetpol.ACLs, portACL)
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
					ipBlockIPSet, ipBlockSetInfo := t.ipBlockRule(npmNetpol.Name, npmNetpol.NameSpace, policies.Ingress, i, fromRule.IPBlock)
					npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, ipBlockIPSet)
					if j != 0 && addedCidrEntry {
						continue
					}
					t.peerAndPortRule(npmNetpol, rule.Ports, []policies.SetInfo{ipBlockSetInfo})
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
					nsSelectorIPSets, nsSrcList := t.nameSpaceSelector(&nsSelector, policies.SrcMatch)
					npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, nsSelectorIPSets...)
					t.peerAndPortRule(npmNetpol, rule.Ports, nsSrcList)
				}
				continue
			}

			// #2.3 handle podSelector and port if exist
			if fromRule.PodSelector != nil && fromRule.NamespaceSelector == nil {
				// TODO check old code if we need any ns- prefix for pod selectors
				podSelectorIPSets, podSelectorSrcList := t.targetPodSelector(npmNetpol.NameSpace, policies.SrcMatch, fromRule.PodSelector)
				npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, podSelectorIPSets...)
				t.peerAndPortRule(npmNetpol, rule.Ports, podSelectorSrcList)
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
			podSelectorIPSets, podSelectorSrcList := t.targetPodSelector(npmNetpol.NameSpace, policies.SrcMatch, fromRule.PodSelector)
			npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, podSelectorIPSets...)

			for _, nsSelector := range FlattenNameSpaceSelector(fromRule.NamespaceSelector) {
				nsSelectorIPSets, nsSrcList := t.nameSpaceSelector(&nsSelector, policies.SrcMatch)
				npmNetpol.RuleIPSets = append(npmNetpol.RuleIPSets, nsSelectorIPSets...)
				nsSrcList = append(nsSrcList, podSelectorSrcList...)
				t.peerAndPortRule(npmNetpol, rule.Ports, nsSrcList)
			}
		}

		// TODO(jungukcho): move this code in entry point of this function?
		if allowExternal {
			allowExternalACL := policies.NewACLPolicy(npmNetpol.NameSpace, npmNetpol.Name, policies.Allowed, policies.Ingress)
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
			klog.Infof("Cannot translate ingress rules due to %s", err)
			return nil, errIngressTranslation
		}
	}

	for _, ptype := range npObj.Spec.PolicyTypes {
		if ptype == networkingv1.PolicyTypeIngress {
			if err := t.translateIngress(npmNetpol, npObj.Spec.PodSelector, npObj.Spec.Ingress); err != nil {
				klog.Infof("Cannot translate ingress rules due to %s", err)
				return nil, errIngressTranslation
			}
		}
	}

	if hasIngress := t.existIngress(npObj); hasIngress {
		t.defaultDropACL(npmNetpol, policies.Ingress)
	}

	return npmNetpol, nil
}
