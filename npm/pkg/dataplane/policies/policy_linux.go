package policies

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
)

type UniqueDirection bool

const (
	forIngress UniqueDirection = true
	forEgress  UniqueDirection = false

	// 5-6 elements depending on Included boolean
	maxLengthForMatchSetSpecs = 6
)

// the NPMNetworkPolicy ACLPolicyID field is unnused in Linux
func aclPolicyID(_, _ string) string {
	return ""
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

func (networkPolicy *NPMNetworkPolicy) egressChainName() string {
	return networkPolicy.chainName(util.IptablesAzureEgressPolicyChainPrefix)
}

func (networkPolicy *NPMNetworkPolicy) ingressChainName() string {
	return networkPolicy.chainName(util.IptablesAzureIngressPolicyChainPrefix)
}

func (networkPolicy *NPMNetworkPolicy) chainName(prefix string) string {
	policyHash := util.Hash(networkPolicy.PolicyKey)
	return joinWithDash(prefix, policyHash)
}

func (networkPolicy *NPMNetworkPolicy) commentForJumpToIngress() string {
	return networkPolicy.commentForJump(forIngress)
}

func (networkPolicy *NPMNetworkPolicy) commentForJumpToEgress() string {
	return networkPolicy.commentForJump(forEgress)
}

func (networkPolicy *NPMNetworkPolicy) commentForJump(direction UniqueDirection) string {
	prefix := "EGRESS"
	if direction == forIngress {
		prefix = "INGRESS"
	}

	toFrom := "FROM"
	if direction == forIngress {
		toFrom = "TO"
	}

	podSelectorComment := "all"
	if len(networkPolicy.PodSelectorList) > 0 {
		podSelectorComment = commentForInfos(networkPolicy.PodSelectorList)
	}
	return fmt.Sprintf("%s-POLICY-%s-%s-%s-IN-ns-%s", prefix, networkPolicy.PolicyKey, toFrom, podSelectorComment, networkPolicy.Namespace)
}

func commentForInfos(infos []SetInfo) string {
	infoComments := make([]string, 0, len(infos))
	for _, info := range infos {
		infoComments = append(infoComments, info.comment())
	}
	return strings.Join(infoComments, "-AND-")
}

func (info SetInfo) comment() string {
	name := info.IPSet.GetPrefixName()
	if info.Included {
		return name
	}
	return "!" + name
}

func (info SetInfo) matchSetSpecs(matchString string) []string {
	specs := make([]string, 0, maxLengthForMatchSetSpecs)
	specs = append(specs, util.IptablesModuleFlag, util.IptablesSetModuleFlag)
	if !info.Included {
		specs = append(specs, util.IptablesNotFlag)
	}
	hashedSetName := info.IPSet.GetHashedName()
	specs = append(specs, util.IptablesMatchSetFlag, hashedSetName, matchString)
	return specs
}

func (aclPolicy *ACLPolicy) comment() string {
	// cleanPeerList contains peers that aren't namedports
	var cleanPeerList []SetInfo
	// TODO remove the if check and initialize the list like in egress once we replace SrcList and DstList with PeerList
	if aclPolicy.Direction == Ingress {
		cleanPeerList = aclPolicy.SrcList
	} else {
		cleanPeerList = make([]SetInfo, 0, len(aclPolicy.DstList))
	}

	var namedPortPeer SetInfo
	foundNamedPortPeer := false
	for _, info := range aclPolicy.DstList {
		if info.IPSet.Type == ipsets.NamedPorts {
			if foundNamedPortPeer {
				metrics.SendErrorLogAndMetric(util.IptmID, "while creating ACL comment, unexpectedly found more than one namedPort peer for ACL:\n%s", aclPolicy.PrettyString())
			}
			namedPortPeer = info
			foundNamedPortPeer = true
		} else if aclPolicy.Direction == Egress {
			// TODO remove the if check once we replace SrcList and DstList with PeerList
			cleanPeerList = append(cleanPeerList, info)
		}
	}

	builder := strings.Builder{}
	if aclPolicy.Target == Allowed {
		builder.WriteString("ALLOW")
	} else {
		builder.WriteString("DROP")
	}

	if len(cleanPeerList) == 0 {
		builder.WriteString("-ALL")
	} else {
		if aclPolicy.Direction == Ingress {
			builder.WriteString("-FROM-")
		} else {
			builder.WriteString("-TO-")
		}
		builder.WriteString(commentForInfos(cleanPeerList))
	}

	builder.WriteString(aclPolicy.Protocol.comment())
	builder.WriteString(aclPolicy.DstPorts.comment())
	if foundNamedPortPeer {
		builder.WriteString("-TO-" + namedPortPeer.comment())
	}
	return builder.String()
}

func (proto Protocol) comment() string {
	if proto == UnspecifiedProtocol {
		return ""
	}
	return fmt.Sprintf("-ON-%s", string(proto))
}

func (portRange *Ports) comment() string {
	if portRange.Port == 0 {
		return ""
	}
	if portRange.Port >= portRange.EndPort {
		return fmt.Sprintf("-TO-PORT-%d", portRange.Port)
	}
	return fmt.Sprintf("-TO-PORT-%d:%d", portRange.Port, portRange.EndPort)
}

func (portRange *Ports) toIPTablesString() string {
	start := strconv.Itoa(int(portRange.Port))
	if portRange.Port == portRange.EndPort {
		return start
	}
	end := strconv.Itoa(int(portRange.EndPort))
	return start + ":" + end
}

/*
	Notes on commenting

	v1 overall:
		"[value]" means include if needed e.g. if a port is specified
		- prefix:
			- no to/from rules and not allowing external:
				- allowed: "ALLOW-ALL"
				- denied: "DROP-ALL"
			- otherwise: drop the "ALL"
		- suffix:
			- no to/from rules or port rules and not allowing external:
				- ingress: "-FROM-all-namespaces"
				- egress: "-TO-all-namespaces"
			- otherwise: "" (no suffix)
		- append these to each other to form the whole comment:
			prefix
			[-cidrIPSetName]
			[-AND-nsSelectorComment]
			[-AND]
			[-podSelectorComment]
			[-protocolComment]
			[-portComment]
			-TO  (or "-FROM" if egress)
			-targetSelectorComment
			suffix

	v2 overall for ACLs:
		- prefix:
			- allowed:
				- no IPSets in SrcList/DstList (i.e. allow external): "ALLOW-ALL"
				- otherwise:
					- ingress: "ALLOW-FROM"
					- egress: "ALLOW-TO"
			- denied: replace "ALLOW" with "DROP"
		- similar idea (think there are at most two non-namedPort ipsets e.g. ns selector and pod selector):
			prefix
			[-ipset1Name]
			[-AND]
			[-ipset2Name]
			[-ON-protocolComment]
			[-TO-namedPortIPSetName]
			[-TO-portComment]
		NOTE: can have none or only one of namedPort and port (range)

	v2 for jumps to ingress/egress chains:
		- prefix:
			- ingress: "INGRESS"
			- egress: "EGRESS"
		- form:
			INGRESS     (or "EGRESS-" if egress)
			-POLICY
			-policyKey
			-TO         (or "-FROM" if egress)
			[-podSelectorComment]   (or "all" if there are no pod selectors)
			-IN-ns
			-namespaceName

	strings for protocol, ports, selectors:
		protocol: just "name"

		port (range):
			- v1:
				- for single port: PORT-x
				- for namedport: PORT-name
			- v2:
				- for single port: PORT-x
				- with endport: PORT-x:y
				- in v2, namedports are specified as IPSets

		selector:
			"[!]" means optionally include "!" if the label is not included
			- namespace selectors (only for v1):
				- if there are no match expressions or labels:
					all-namespaces
				- otherwise:
					ns-[!]label1-AND-ns-[!]label2...
			- other w/out ns:
				[!]label1-AND-[!]label2...
			- other w/ ns:
				[!]label1-AND-[!]label2...-AND-ns-[!]labelM-IN-ns-name
*/
