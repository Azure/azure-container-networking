package policies

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

type NPMNetworkPolicy struct {
	// Namespace is only used by Linux to construct an iptables comment
	Namespace string
	// PolicyKey is a unique combination of "namespace/name" of network policy
	PolicyKey string
	// ACLPolicyID is only used in Windows. See aclPolicyID() in policy_windows.go for more info
	ACLPolicyID string
	// TODO get rid of PodSelectorIPSets in favor of PodSelectorList (exact same except need to add members field to SetInfo)
	// PodSelectorIPSets holds the IPSets for the Pod Selector
	PodSelectorIPSets []*ipsets.TranslatedIPSet
	// ChildPodSelectorIPSets holds the IPSets that are members of any ipset in PodSelectorIPSets
	ChildPodSelectorIPSets []*ipsets.TranslatedIPSet
	// TODO change to slice of pointers
	// PodSelectorList holds the ipsets from PodSelectorIPSets and info about them to avoid duplication in SrcList and DstList fields in ACLs
	PodSelectorList []SetInfo
	// RuleIPSets holds all IPSets generated from policy's rules
	// and not from pod selector IPSets, including children of a NestedLabelOfPod ipset
	RuleIPSets []*ipsets.TranslatedIPSet
	ACLs       []*ACLPolicy
	// podIP is key and endpoint ID as value
	// Will be populated by dataplane and policy manager
	PodEndpoints map[string]string
}

func NewNPMNetworkPolicy(netPolName, netPolNamespace string) *NPMNetworkPolicy {
	return &NPMNetworkPolicy{
		Namespace:   netPolNamespace,
		PolicyKey:   fmt.Sprintf("%s/%s", netPolNamespace, netPolName),
		ACLPolicyID: aclPolicyID(netPolNamespace, netPolName),
	}
}

func (netPol *NPMNetworkPolicy) HasCIDRRules() bool {
	for _, set := range netPol.RuleIPSets {
		if set.Metadata.Type == ipsets.CIDRBlocks {
			return true
		}
	}
	return false
}

func (netPol *NPMNetworkPolicy) AllPodSelectorIPSets() []*ipsets.TranslatedIPSet {
	return append(netPol.PodSelectorIPSets, netPol.ChildPodSelectorIPSets...)
}

func (netPol *NPMNetworkPolicy) numACLRulesProducedInKernel() int {
	numRules := 0
	hasIngress := false
	hasEgress := false
	for _, aclPolicy := range netPol.ACLs {
		if aclPolicy.hasIngress() {
			hasIngress = true
			numRules++
		}
		if aclPolicy.hasEgress() {
			hasEgress = true
			numRules++
		}
	}

	// both Windows and Linux have an extra ACL rule for ingress and an extra rule for egress
	if hasIngress {
		numRules++
	}
	if hasEgress {
		numRules++
	}
	return numRules
}

func (netPol *NPMNetworkPolicy) PrettyString() string {
	if netPol == nil {
		klog.Infof("NPMNetworkPolicy is nil when trying to print string")
		return "nil NPMNetworkPolicy"
	}
	itemStrings := make([]string, 0, len(netPol.ACLs))
	for _, item := range netPol.ACLs {
		itemStrings = append(itemStrings, item.PrettyString())
	}
	aclArrayString := strings.Join(itemStrings, "\n--\n")

	podSelectorIPSetString := translatedIPSetsToString(netPol.PodSelectorIPSets)
	podSelectorListString := infoArrayToString(netPol.PodSelectorList)
	format := `Namespace/Name: %s
PodSelectorIPSets: %s
PodSelectorList: %s
ACLs:
%s`
	return fmt.Sprintf(format, netPol.PolicyKey, podSelectorIPSetString, podSelectorListString, aclArrayString)
}

// ACLPolicy equivalent to a single iptable rule in linux
// or a single HNS rule in windows
type ACLPolicy struct {
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
	// SrcDirectIPs holds direct IPs for source matching (used for /32 CIDRs on Windows with npm lite enabled)
	SrcDirectIPs []string
	// DstDirectIPs holds direct IPs for destination matching (used for /32 CIDRs on Windows with npm lite enabled)
	DstDirectIPs []string
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

// NormalizePolicy helps fill in missed fields in aclPolicy
func NormalizePolicy(networkPolicy *NPMNetworkPolicy) {
	for _, aclPolicy := range networkPolicy.ACLs {
		if aclPolicy.Protocol == "" {
			aclPolicy.Protocol = UnspecifiedProtocol
		}

		if aclPolicy.DstPorts.EndPort == 0 {
			aclPolicy.DstPorts.EndPort = aclPolicy.DstPorts.Port
		}
	}
}

func ValidatePolicy(networkPolicy *NPMNetworkPolicy) error {
	for _, aclPolicy := range networkPolicy.ACLs {
		if !aclPolicy.hasKnownTarget() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy for NetPol %s has unknown target [%s]", networkPolicy.PolicyKey, aclPolicy.Target))
		}
		if !aclPolicy.hasKnownDirection() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy for NetPol %s has unknown direction [%s]", networkPolicy.PolicyKey, aclPolicy.Direction))
		}
		if !aclPolicy.hasKnownProtocol() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy for NetPol %s has unknown protocol [%s]", networkPolicy.PolicyKey, aclPolicy.Protocol))
		}
		if util.IsWindowsDP() && aclPolicy.Protocol == SCTP {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy for NetPol %s has unsupported SCTP protocol on Windows", networkPolicy.PolicyKey))
		}

		if !aclPolicy.satisifiesPortAndProtocolConstraints() {
			return npmerrors.SimpleError(fmt.Sprintf(
				"ACL policy for NetPol %s has dst port(s) (Port or Port and EndPort), so must have protocol tcp, udp, udplite, sctp, or dccp but has protocol %s",
				networkPolicy.PolicyKey,
				string(aclPolicy.Protocol),
			))
		}

		if !aclPolicy.DstPorts.isValidRange() {
			return npmerrors.SimpleError(fmt.Sprintf("ACL policy for NetPol %s has invalid port range in DstPorts (start: %d, end: %d)",
				networkPolicy.PolicyKey, aclPolicy.DstPorts.Port, aclPolicy.DstPorts.EndPort))
		}

		for _, setInfo := range aclPolicy.SrcList {
			if !setInfo.hasKnownMatchType() {
				return npmerrors.SimpleError(fmt.Sprintf("ACL policy for NetPol %s has set %s in SrcList with unknown Match Type", networkPolicy.PolicyKey, setInfo.IPSet.Name))
			}
		}
		for _, setInfo := range aclPolicy.DstList {
			if !setInfo.hasKnownMatchType() {
				return npmerrors.SimpleError(fmt.Sprintf("ACL policy for NetPol %s has set %s in DstList with unknown Match Type", networkPolicy.PolicyKey, setInfo.IPSet.Name))
			}
		}
	}
	return nil
}

func NewACLPolicy(target Verdict, direction Direction) *ACLPolicy {
	acl := &ACLPolicy{
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

func (aclPolicy *ACLPolicy) PrettyString() string {
	format := `Target:%s  Direction:%s  Protocol:%s  Ports:%+v
SrcList: %s
DstList: %s`
	return fmt.Sprintf(format, aclPolicy.Target, aclPolicy.Direction, aclPolicy.Protocol, aclPolicy.DstPorts, infoArrayToString(aclPolicy.SrcList), infoArrayToString(aclPolicy.DstList))
}

func infoArrayToString(items []SetInfo) string {
	itemStrings := make([]string, 0, len(items))
	for _, item := range items {
		itemStrings = append(itemStrings, fmt.Sprintf("{%s}", item.PrettyString()))
	}
	return fmt.Sprintf("[%s]", strings.Join(itemStrings, ","))
}

func translatedIPSetsToString(items []*ipsets.TranslatedIPSet) string {
	itemStrings := make([]string, 0, len(items))
	for _, item := range items {
		ipset := ipsets.NewIPSet(item.Metadata)
		itemStrings = append(itemStrings, fmt.Sprintf("{%s}", ipset.PrettyString()))
	}
	return fmt.Sprintf("[%s]", strings.Join(itemStrings, ","))
}

// SetInfo helps capture additional details in a matchSet.
// Included flag captures the negative or positive match.
// Included is true when match set does not have "!".
// Included is false when match set have "!".
// MatchType captures match direction flags.
// For example match set in linux:
//
//	! azure-npm-123 src
//
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

func (info SetInfo) PrettyString() string {
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
