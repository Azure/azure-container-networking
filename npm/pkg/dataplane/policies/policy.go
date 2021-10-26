package policies

import (
	"strconv"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	networkingv1 "k8s.io/api/networking/v1"
)

type NPMNetworkPolicy struct {
	Name string
	// PodSelectorIPSets holds all the IPSets generated from Pod Selector
	PodSelectorIPSets []*ipsets.TranslatedIPSet
	// RuleIPSets holds all IPSets generated from policy's rules
	// and not from pod selector IPSets
	//
	RuleIPSets []*ipsets.TranslatedIPSet
	ACLs       []*ACLPolicy
	// podIP is key and endpoint ID as value
	// Will be populated by dataplane and policy manager
	PodEndpoints map[string]string
	RawNP        *networkingv1.NetworkPolicy
}

// ACLPolicy equivalent to a single iptable rule in linux
// or a single HNS rule in windows
type ACLPolicy struct {
	// PolicyID is the rules name with a given network policy
	PolicyID string
	// Comment is the string attached to rule to identity its representation
	Comment string
	// SrcList source IPSets condition setinfos
	SrcList []SetInfo
	// DstList destination IPSets condition setinfos
	DstList []SetInfo
	// Target defines a target in iptables for linux. i,e, Mark, Accept, Drop
	// in windows, this is either ALLOW or DENY
	Target Verdict
	// Direction defines the flow of traffic
	Direction Direction
	// SrcPorts holds the source port information
	SrcPorts []Ports
	// DstPorts holds the destination port information
	DstPorts []Ports
	// Protocol is the value of traffic protocol
	Protocol Protocol
}

// SetInfo helps capture additional details in a matchSet
// example match set in linux:
//             ! azure-npm-123 src,src
// "!" this indicates a negative match of an IPset for src,src
// Included flag captures the negative or positive match
// MatchType captures match flags
type SetInfo struct {
	IPSet     *ipsets.IPSetMetadata
	Included  bool
	MatchType MatchType
}

// Ports represents a range of ports.
// To specify one port, set Port and EndPort to the same value.
// uint16 is used since there are 2^16 - 1 TCP/UDP ports (0 is invalid)
// and 2^16 SCTP ports. ICMP is connectionless and doesn't use ports.
type Ports struct {
	Port    int32
	EndPort int32
}

type Verdict string

type Direction string

type Protocol string

type MatchType int8

const (
	// Ingress when packet is entering a container
	Ingress Direction = "IN"
	// Egress when packet is leaving a container
	Egress Direction = "OUT"
	// Both applies to both directions
	Both Direction = "BOTH"

	// Allowed is accept in linux
	Allowed Verdict = "ALLOW"
	// Dropped is denying a flow
	Dropped Verdict = "DROP"

	// TCP Protocol
	TCP Protocol = "tcp"
	// UDP Protocol
	UDP Protocol = "udp"
	// SCTP Protocol
	SCTP Protocol = "sctp"
	// ICMP Protocol
	ICMP Protocol = "icmp"
	// AnyProtocol can be used for all other protocols
	AnyProtocol Protocol = "all"
)

// Possible MatchTypes.
// MatchTypes with 2 locations (e.g. SrcDst) are for ip and port respectively.
const (
	SrcMatch    MatchType = 0
	DstMatch    MatchType = 1
	SrcSrcMatch MatchType = 2
	DstDstMatch MatchType = 3
	SrcDstMatch MatchType = 4
	DstSrcMatch MatchType = 5
)

func (policy *ACLPolicy) hasKnownDirection() bool {
	return policy.Direction == Ingress ||
		policy.Direction == Egress ||
		policy.Direction == Both
}

func (policy *ACLPolicy) hasIngress() bool {
	return policy.Direction == Ingress || policy.Direction == Both
}

func (policy *ACLPolicy) hasEgress() bool {
	return policy.Direction == Egress || policy.Direction == Both
}

func (policy *ACLPolicy) hasKnownProtocol() bool {
	return policy.Protocol != "" && (policy.Protocol == TCP ||
		policy.Protocol == UDP ||
		policy.Protocol == SCTP ||
		policy.Protocol == ICMP ||
		policy.Protocol == AnyProtocol)
}

func (policy *ACLPolicy) hasKnownTarget() bool {
	return policy.Target == Allowed || policy.Target == Dropped
}

func (portRange *Ports) isValidRange() bool {
	return portRange.Port <= portRange.EndPort
}

var matchTypeStrings = make(map[MatchType]string)

func initMatchTypeStrings() {
	if len(matchTypeStrings) == 0 {
		matchTypeStrings[SrcMatch] = util.IptablesSrcFlag
		matchTypeStrings[DstMatch] = util.IptablesDstFlag
		matchTypeStrings[SrcSrcMatch] = util.IptablesSrcFlag + "," + util.IptablesSrcFlag
		matchTypeStrings[DstDstMatch] = util.IptablesDstFlag + "," + util.IptablesDstFlag
		matchTypeStrings[SrcDstMatch] = util.IptablesSrcFlag + "," + util.IptablesDstFlag
		matchTypeStrings[DstSrcMatch] = util.IptablesDstFlag + "," + util.IptablesSrcFlag
	}
}

// match type is only used in Linux
func (setInfo *SetInfo) hasKnownMatchType() bool {
	initMatchTypeStrings()
	_, exists := matchTypeStrings[setInfo.MatchType]
	return exists
}

func (matchType MatchType) toIPTablesString() string {
	initMatchTypeStrings()
	return matchTypeStrings[matchType]
}

func (portRange *Ports) toIPTablesString() string {
	start := strconv.Itoa(int(portRange.Port))
	if portRange.Port == portRange.EndPort {
		return start
	}
	end := strconv.Itoa(int(portRange.EndPort))
	return start + ":" + end
}

func (policy *ACLPolicy) satisifiesPortAndProtocolConstraints() bool {
	return policy.Protocol != AnyProtocol ||
		(len(policy.SrcPorts) == 0 && len(policy.DstPorts) == 0)
}

func (networkPolicy *NPMNetworkPolicy) hasSamePodSelector(otherNetworkPolicy *NPMNetworkPolicy) bool {
	if len(networkPolicy.PodSelectorIPSets) != len(otherNetworkPolicy.PodSelectorIPSets) {
		return false
	}
	for k, ipset := range networkPolicy.PodSelectorIPSets {
		otherIPSet := otherNetworkPolicy.PodSelectorIPSets[k]
		if !ipset.Equals(otherIPSet) {
			return false
		}
	}
	return true
}
