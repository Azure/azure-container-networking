package policies

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/klog"
)

type NPMNetworkPolicy struct {
	Name      string
	NameSpace string
	// TODO remove Name and Namespace field
	// PolicyKey is a unique combination of "namespace/name" of network policy
	PolicyKey string
	// PodSelectorIPSets holds all the IPSets generated from Pod Selector
	PodSelectorIPSets []*ipsets.TranslatedIPSet
	// PodSelectorList holds target pod information to avoid duplicatoin in SrcList and DstList fields in ACLs
	PodSelectorList []SetInfo
	// RuleIPSets holds all IPSets generated from policy's rules
	// and not from pod selector IPSets
	RuleIPSets []*ipsets.TranslatedIPSet
	ACLs       []*ACLPolicy
	// podIP is key and endpoint ID as value
	// Will be populated by dataplane and policy manager
	PodEndpoints map[string]string
}

func NewNPMNetworkPolicy(netPolName, netPolNamespace string) *NPMNetworkPolicy {
	return &NPMNetworkPolicy{
		Name:      netPolName,
		NameSpace: netPolNamespace,
		PolicyKey: fmt.Sprintf("%s/%s", netPolNamespace, netPolName),
	}
}

// ACLPolicy equivalent to a single iptable rule in linux
// or a single HNS rule in windows
type ACLPolicy struct {
	// PolicyID is the rules name with a given network policy
	// PolicyID will be same for all ACLs in a Network Policy
	// it will be "azure-acl-NetPolNS-netPolName"
	PolicyID string
	// Comment is the string attached to rule to identity its representation
	Comment string
	// TODO(jungukcho): now I think we do not need to manage SrcList and DstList
	// We may have just one PeerList to hold since it will depend on direction except for namedPort.
	// They are exclusive and each SetInfo even have its own direction.
	// PeerList []SetInfo
	// SrcList source IPSets condition setinfos
	SrcList []SetInfo
	// DstList destination IPSets condition setinfos
	DstList []SetInfo
	// Target defines a target in iptables for linux. i,e, Mark, Accept, Drop
	// in windows, this is either ALLOW or DENY
	Target Verdict
	// Direction defines the flow of traffic
	Direction Direction
	// DstPorts always holds the destination port information.
	// The valid value for port must be between 1 and 65535, inclusive
	// and the endPort must be equal or greater than port.
	DstPorts Ports
	// Protocol is the value of traffic protocol
	Protocol Protocol
}

const policyIDPrefix = "azure-acl"

// FIXME this impacts windows DP if it isn't equivalent to netPol.PolicyKey
// aclPolicyID returns azure-acl-<network policy namespace>-<network policy name> format
// to differentiate ACLs among different network policies,
// but aclPolicy in the same network policy has the same aclPolicyID.
func aclPolicyID(policyNS, policyName string) string {
	return fmt.Sprintf("%s-%s-%s", policyIDPrefix, policyNS, policyName)
}

// TODO make this a method of NPMNetworkPolicy, and just use netPol.PolicyKey as the PolicyID
func NewACLPolicy(policyNS, policyName string, target Verdict, direction Direction) *ACLPolicy {
	acl := &ACLPolicy{
		PolicyID:  aclPolicyID(policyNS, policyName),
		Target:    target,
		Direction: direction,
	}
	return acl
}

// AddSetInfo is to add setInfo to SrcList or DstList based on direction
// except for a setInfo for namedPort since namedPort is always for destination.
// TODO(jungukcho): cannot come up with Both Direction.
func (aclPolicy *ACLPolicy) AddSetInfo(peerList []SetInfo) {
	for _, peer := range peerList {
		// in case peer is a setInfo for namedPort, the peer is always added to DstList in aclPolicy
		// regardless of direction since namePort is always for destination.
		if peer.MatchType == DstDstMatch {
			aclPolicy.DstList = append(aclPolicy.DstList, peer)
			continue
		}

		// add peer into SrcList or DstList based on Direction
		if aclPolicy.Direction == Ingress {
			aclPolicy.SrcList = append(aclPolicy.SrcList, peer)
		} else if aclPolicy.Direction == Egress {
			aclPolicy.DstList = append(aclPolicy.DstList, peer)
		}
	}
}

func (aclPolicy *ACLPolicy) hasKnownDirection() bool {
	return aclPolicy.Direction == Ingress ||
		aclPolicy.Direction == Egress ||
		aclPolicy.Direction == Both
}

func (aclPolicy *ACLPolicy) hasIngress() bool {
	return aclPolicy.Direction == Ingress || aclPolicy.Direction == Both
}

func (aclPolicy *ACLPolicy) hasEgress() bool {
	return aclPolicy.Direction == Egress || aclPolicy.Direction == Both
}

func (aclPolicy *ACLPolicy) hasKnownProtocol() bool {
	return aclPolicy.Protocol == TCP ||
		aclPolicy.Protocol == UDP ||
		aclPolicy.Protocol == SCTP ||
		aclPolicy.Protocol == UnspecifiedProtocol
}

func (aclPolicy *ACLPolicy) hasKnownTarget() bool {
	return aclPolicy.Target == Allowed || aclPolicy.Target == Dropped
}

func (aclPolicy *ACLPolicy) satisifiesPortAndProtocolConstraints() bool {
	// namedports handle protocol constraints
	return (aclPolicy.hasNamedPort() && aclPolicy.Protocol == UnspecifiedProtocol) ||
		aclPolicy.Protocol != UnspecifiedProtocol ||
		aclPolicy.DstPorts.isUnspecified()
}

func (aclPolicy *ACLPolicy) hasNamedPort() bool {
	for _, peer := range aclPolicy.DstList {
		if peer.IPSet.Type == ipsets.NamedPorts {
			return true
		}
	}
	return false
}

func (netPol *NPMNetworkPolicy) String() string {
	if netPol == nil {
		klog.Infof("NPMNetworkPolicy is nil when trying to print string")
		return "nil NPMNetworkPolicy"
	}
	itemStrings := make([]string, 0, len(netPol.ACLs))
	for _, item := range netPol.ACLs {
		itemStrings = append(itemStrings, item.String())
	}
	aclArrayString := strings.Join(itemStrings, "\n--\n")

	podSelectorIPSetString := translatedIPSetsToString(netPol.PodSelectorIPSets)
	podSelectorListString := infoArrayToString(netPol.PodSelectorList)
	format := `Name:%s  Namespace:%s
PodSelectorIPSets: %s
PodSelectorList: %s
ACLs:
%s`
	return fmt.Sprintf(format, netPol.Name, netPol.NameSpace, podSelectorIPSetString, podSelectorListString, aclArrayString)
}

func (aclPolicy *ACLPolicy) String() string {
	format := `Target:%s  Direction:%s  Protocol:%s  Ports:%+v
SrcList: %s
DstList: %s`
	return fmt.Sprintf(format, aclPolicy.Target, aclPolicy.Direction, aclPolicy.Protocol, aclPolicy.DstPorts, infoArrayToString(aclPolicy.SrcList), infoArrayToString(aclPolicy.DstList))
}

func infoArrayToString(items []SetInfo) string {
	itemStrings := make([]string, 0, len(items))
	for _, item := range items {
		itemStrings = append(itemStrings, fmt.Sprintf("{%s}", item.String()))
	}
	return fmt.Sprintf("[%s]", strings.Join(itemStrings, ","))
}

func translatedIPSetsToString(items []*ipsets.TranslatedIPSet) string {
	itemStrings := make([]string, 0, len(items))
	for _, item := range items {
		ipset := ipsets.NewIPSet(item.Metadata)
		itemStrings = append(itemStrings, fmt.Sprintf("{%s}", ipset.String()))
	}
	return fmt.Sprintf("[%s]", strings.Join(itemStrings, ","))
}

// SetInfo helps capture additional details in a matchSet.
// Included flag captures the negative or positive match.
// Included is true when match set does not have "!".
// Included is false when match set have "!".
// MatchType captures match direction flags.
// For example match set in linux:
//             ! azure-npm-123 src
// "!" this indicates a negative match (Included is false) of an azure-npm-123
// MatchType is "src"
type SetInfo struct {
	IPSet     *ipsets.IPSetMetadata
	Included  bool
	MatchType MatchType
}

// Ports represents a range of ports.
// To specify one port, set Port and EndPort to the same value.
// uint16 is used since there are 2^16 - 1 TCP/UDP ports (0 is invalid)
// and 2^16 SCTP ports.
// NewSetInfo creates SetInfo.
func NewSetInfo(name string, setType ipsets.SetType, included bool, matchType MatchType) SetInfo {
	return SetInfo{
		IPSet:     ipsets.NewIPSetMetadata(name, setType),
		Included:  included,
		MatchType: matchType,
	}
}

func (info SetInfo) String() string {
	return fmt.Sprintf("Name:%s  HashedName:%s  MatchType:%v  Included:%v", info.IPSet.GetPrefixName(), info.IPSet.GetHashedName(), info.MatchType, info.Included)
}

type Ports struct {
	Port    int32
	EndPort int32
}

func (portRange *Ports) isValidRange() bool {
	return portRange.Port <= portRange.EndPort
}

func (portRange *Ports) isUnspecified() bool {
	return portRange.Port == 0
}

type Direction string

const (
	// Ingress when packet is entering a container
	Ingress Direction = "IN"
	// Egress when packet is leaving a container
	Egress Direction = "OUT"
	// Both applies to both directions
	Both Direction = "BOTH"
)

type Verdict string

const (
	// Allowed is accept in linux
	Allowed Verdict = "ALLOW"
	// Dropped is denying a flow
	Dropped Verdict = "DROP"
)

// Protocol can be TCP, UDP, SCTP, or unspecified since they are currently supported in networkpolicy.
// Protocol value is case-sensitive (Capital now).
// TODO: Need to remove this dependency on case-sensitivity.
// NPM is not fully tested with SCTP.
type Protocol string

const (

	// TCP Protocol
	TCP Protocol = "TCP"
	// UDP Protocol
	UDP Protocol = "UDP"
	// SCTP Protocol
	SCTP Protocol = "SCTP"
	// UnspecifiedProtocol leaves protocol unspecified. For a named port, this represents its protocol. Otherwise, this represents any protocol.
	UnspecifiedProtocol Protocol = "unspecified"
)

type MatchType int8

// Possible MatchTypes.
const (
	SrcMatch MatchType = 0
	DstMatch MatchType = 1
	// MatchTypes with 2 locations (e.g. DstDst) are for ip and port respectively.
	DstDstMatch MatchType = 2
	// This is used for podSelector under spec. It can be Src or Dst based on existence of ingress or egress rule.
	EitherMatch MatchType = 3
)

var matchTypeStrings = map[MatchType]string{
	SrcMatch:    util.IptablesSrcFlag,
	DstMatch:    util.IptablesDstFlag,
	DstDstMatch: util.IptablesDstFlag + "," + util.IptablesDstFlag,
}

// match type is only used in Linux
func (setInfo *SetInfo) hasKnownMatchType() bool {
	_, exists := matchTypeStrings[setInfo.MatchType]
	return exists
}

func (matchType MatchType) toIPTablesString() string {
	return matchTypeStrings[matchType]
}
