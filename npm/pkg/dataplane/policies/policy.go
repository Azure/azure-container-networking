package policies

import (
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	networkingv1 "k8s.io/api/networking/v1"
)

type NPMNetworkPolicy struct {
	Name string
	// PodSelectorIPSets holds all the IPSets generated from Pod Selector
	PodSelectorIPSets map[string]*ipsets.TranslatedIPSet
	// RuleIPSets holds all IPSets generated from policy's rules
	// and not from pod selector IPSets
	RuleIPSets map[string]*ipsets.TranslatedIPSet
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
	IPSet     *ipsets.IPSet
	Included  bool
	MatchType string // match type can be “src”, “src,dst” or “dst,dst” etc
}

type Ports struct {
	Port    int64
	EndPort int64
}

type Verdict string

type Direction string

type Protocol string

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
	AnyProtocol Protocol = "any"
)
